package tlsca

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestEnsureCAPersistsAndReloads(t *testing.T) {
	dir := t.TempDir()

	ca1, err := EnsureCA(dir)
	if err != nil {
		t.Fatalf("EnsureCA (create): %v", err)
	}
	if _, err := os.Stat(CertPath(dir)); err != nil {
		t.Errorf("cert file not written: %v", err)
	}
	// Key file must be private (0600).
	info, err := os.Stat(KeyPath(dir))
	if err != nil {
		t.Fatalf("key file not written: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("key perm = %o, want 600", perm)
	}

	// A second EnsureCA loads the same CA rather than regenerating.
	ca2, err := EnsureCA(dir)
	if err != nil {
		t.Fatalf("EnsureCA (reload): %v", err)
	}
	if ca1.cert.SerialNumber.Cmp(ca2.cert.SerialNumber) != 0 {
		t.Errorf("reloaded CA has different serial; expected the persisted one")
	}
	if !ca1.cert.IsCA {
		t.Errorf("CA cert IsCA = false")
	}
	if ca1.cert.Subject.CommonName != CommonName {
		t.Errorf("CA CN = %q, want %q", ca1.cert.Subject.CommonName, CommonName)
	}
}

// configured stands in for one routes file: two route hosts (one written with
// capitals, as a route name may be) and two block-list hosts, one an IP
// literal, which the block list accepts.
var configured = []string{"csl.this", "Upper.this", "reddit.com", "127.0.0.1"}

func TestConfiguredNamesGetVerifiedLeaf(t *testing.T) {
	ca := newTestCA(t)
	ca.SetHosts(configured)
	roots := x509.NewCertPool()
	roots.AddCert(ca.cert)

	// Over a real handshake: route hosts and block-list hosts, in any case.
	for _, name := range []string{"csl.this", "upper.this", "UPPER.this", "reddit.com", "Reddit.COM"} {
		t.Run(name, func(t *testing.T) {
			leaf, err := handshake(t, ca, name)
			if err != nil {
				t.Fatalf("handshake for %q: %v", name, err)
			}
			if _, err := leaf.Verify(x509.VerifyOptions{Roots: roots, DNSName: name}); err != nil {
				t.Errorf("leaf for %q does not verify against CA: %v", name, err)
			}
		})
	}

	// Clients never put an IP literal in SNI, so the IP leaf is checked directly.
	cert, err := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: "127.0.0.1"})
	if err != nil || cert == nil {
		t.Fatalf("GetCertificate(127.0.0.1) = %v, %v; want a leaf", cert, err)
	}
	opts := x509.VerifyOptions{Roots: roots, DNSName: "127.0.0.1"} // matched against IPAddresses
	if _, err := cert.Leaf.Verify(opts); err != nil {
		t.Errorf("IP leaf does not verify against CA: %v", err)
	}
}

func TestGetCertificateCaches(t *testing.T) {
	ca := newTestCA(t)
	ca.SetHosts(configured)
	a, err := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: "reddit.com"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: "Reddit.com"})
	if err != nil {
		t.Fatal(err)
	}
	if a == nil || a != b {
		t.Errorf("repeated GetCertificate minted a fresh leaf; expected a cached one")
	}
}

func TestUnknownNameIsRefused(t *testing.T) {
	ca := newTestCA(t)
	ca.SetHosts(configured)
	// No suffix or wildcard matching: a subdomain of a configured host is unknown.
	for _, name := range []string{"unknown.example", "www.reddit.com", "this"} {
		t.Run(name, func(t *testing.T) {
			_, err := handshake(t, ca, name)
			if err == nil || !strings.Contains(err.Error(), "unrecognized name") {
				t.Fatalf("handshake for %q: err = %v, want an unrecognized name alert", name, err)
			}
			cert, err := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: name})
			if cert != nil || err != nil {
				t.Errorf("GetCertificate(%q) = %v, %v; want nil, nil", name, cert, err)
			}
		})
	}
	if n := cacheSize(ca); n != 0 {
		t.Errorf("cache holds %d leaves after refused handshakes, want 0", n)
	}
}

func TestCacheBoundedByConfiguredNames(t *testing.T) {
	ca := newTestCA(t)
	ca.SetHosts(configured)
	for i := range 200 {
		_, _ = ca.GetCertificate(&tls.ClientHelloInfo{ServerName: fmt.Sprintf("n%d.example", i)})
	}
	for range 2 {
		for _, name := range append([]string{""}, configured...) {
			if _, err := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: name}); err != nil {
				t.Fatalf("GetCertificate(%q): %v", name, err)
			}
		}
	}
	// Every configured name plus the one no-server-name leaf, and nothing else.
	if n, want := cacheSize(ca), len(configured)+1; n != want {
		t.Errorf("cache holds %d leaves, want %d", n, want)
	}
}

func TestSetHostsDropsRemovedNames(t *testing.T) {
	ca := newTestCA(t)
	ca.SetHosts([]string{"keep.this", "gone.this"})
	kept, _ := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: "keep.this"})
	if c, _ := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: "gone.this"}); c == nil {
		t.Fatal("no leaf for configured gone.this")
	}

	ca.SetHosts([]string{"keep.this"}) // a routes reload that removed gone.this

	if n := cacheSize(ca); n != 1 {
		t.Errorf("cache holds %d leaves after the reload, want 1", n)
	}
	if c, _ := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: "keep.this"}); c != kept {
		t.Errorf("a name still configured lost its cached leaf on reload")
	}
	if c, err := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: "gone.this"}); c != nil ||
		err != nil {
		t.Errorf("GetCertificate(gone.this) after removal = %v, %v; want nil, nil", c, err)
	}
}

func TestGetCertificateConcurrent(t *testing.T) {
	ca := newTestCA(t)
	ca.SetHosts(configured)
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Go(func() {
			for j := range 50 {
				for _, name := range []string{configured[j%len(configured)], fmt.Sprintf("x%d-%d.example", i, j), ""} {
					if _, err := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: name}); err != nil {
						t.Errorf("GetCertificate(%q): %v", name, err)
					}
				}
			}
		})
	}
	wg.Go(func() { // routes reloads racing the handshakes
		for j := range 50 {
			if j%2 == 0 {
				ca.SetHosts(configured[:1])
			} else {
				ca.SetHosts(configured)
			}
		}
	})
	wg.Wait()
	if n, limit := cacheSize(ca), len(configured)+1; n > limit {
		t.Errorf("cache holds %d leaves, want at most %d", n, limit)
	}
}

func newTestCA(t *testing.T) *CA {
	t.Helper()
	ca, err := EnsureCA(t.TempDir())
	if err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}
	return ca
}

func cacheSize(ca *CA) int {
	ca.mu.Lock()
	defer ca.mu.Unlock()
	return len(ca.leaves)
}

// handshake runs one real TLS handshake against ca on an ephemeral loopback
// port and returns the leaf the client was served. sni "" sends no server name
// (the client dials an IP literal, which never goes in SNI). The client skips
// verification; callers that care verify the leaf against the CA themselves.
func handshake(t *testing.T, ca *CA, sni string) (*x509.Certificate, error) {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{GetCertificate: ca.GetCertificate})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		_ = conn.(*tls.Conn).Handshake()
		_ = conn.Close()
	}()
	conn, err := tls.Dial("tcp", ln.Addr().String(),
		&tls.Config{ServerName: sni, InsecureSkipVerify: true})
	if err != nil {
		<-done
		return nil, err
	}
	leaf := conn.ConnectionState().PeerCertificates[0]
	_ = conn.Close()
	<-done
	return leaf, nil
}

// A client with no server name (it dialed 127.0.0.1) gets the one localhost
// leaf whatever the routes say, as it did before names were limited; an
// explicit "localhost" gets the same leaf.
func TestGetCertificateEmptySNI(t *testing.T) {
	ca := newTestCA(t) // no SetHosts: the fallback needs no configured name
	leaf, err := handshake(t, ca, "")
	if err != nil {
		t.Fatalf("handshake with no server name: %v", err)
	}
	if leaf.Subject.CommonName != "localhost" {
		t.Errorf("empty-SNI leaf CN = %q, want localhost", leaf.Subject.CommonName)
	}
	empty, _ := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: ""})
	named, _ := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: "localhost"})
	if empty == nil || empty != named {
		t.Errorf("empty and explicit localhost got different leaves: %p, %p", empty, named)
	}
	if n := cacheSize(ca); n != 1 {
		t.Errorf("cache holds %d leaves, want 1", n)
	}
}
