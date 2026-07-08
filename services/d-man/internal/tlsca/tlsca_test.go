package tlsca

import (
	"crypto/tls"
	"crypto/x509"
	"os"
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

func TestGetCertificateVerifiesAgainstCA(t *testing.T) {
	ca, err := EnsureCA(t.TempDir())
	if err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(ca.cert)

	for _, name := range []string{"reddit.com", "www.example.com", "127.0.0.1"} {
		t.Run(name, func(t *testing.T) {
			cert, err := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: name})
			if err != nil {
				t.Fatalf("GetCertificate(%q): %v", name, err)
			}
			leaf, err := x509.ParseCertificate(cert.Certificate[0])
			if err != nil {
				t.Fatalf("parse leaf: %v", err)
			}
			opts := x509.VerifyOptions{Roots: roots}
			opts.DNSName = name // for an IP SNI, Verify matches it against IPAddresses
			if _, err := leaf.Verify(opts); err != nil {
				t.Errorf("leaf for %q does not verify against CA: %v", name, err)
			}
		})
	}
}

func TestGetCertificateCaches(t *testing.T) {
	ca, err := EnsureCA(t.TempDir())
	if err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}
	a, err := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: "reddit.com"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: "reddit.com"})
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("repeated GetCertificate minted a fresh leaf; expected a cached one")
	}
}

func TestGetCertificateEmptySNI(t *testing.T) {
	ca, err := EnsureCA(t.TempDir())
	if err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}
	cert, err := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: ""})
	if err != nil {
		t.Fatalf("GetCertificate(empty SNI): %v", err)
	}
	if cert.Leaf.Subject.CommonName != "localhost" {
		t.Errorf("empty-SNI leaf CN = %q, want localhost", cert.Leaf.Subject.CommonName)
	}
}
