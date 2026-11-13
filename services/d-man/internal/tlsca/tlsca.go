// Package tlsca is a local certificate authority for d-man's TLS listener. It
// mints leaf certificates on demand for the names d-man answers (its route
// hosts and block-list hosts, handed over with SetHosts on every routes load),
// all signed by one long-lived CA whose cert is installed into the system
// trust store (see `d-man ca install`). That is what lets a blocked HTTPS host
// (reddit.com and friends, which force https via HSTS) land on the local block
// page with a valid lock instead of a cert warning. Any other name is refused,
// so the trusted CA never signs for a name d-man does not serve and the leaf
// cache never outgrows the routes file.
package tlsca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CommonName is the CA certificate's subject. `d-man ca uninstall` finds the
// cert in the keychain by this name, so the two must agree.
const CommonName = "d-man Local CA"

const (
	caCertFile = "ca.pem"
	caKeyFile  = "ca-key.pem"

	caValidity   = 10 * 365 * 24 * time.Hour // ~10 years
	leafValidity = 365 * 24 * time.Hour      // ~1 year
	backdate     = time.Hour                 // tolerate small clock skew

	// fallbackName is the leaf served to a client that sends no server name
	// (it dialed an IP literal). It is always allowed, and is the only name
	// outside SetHosts, so the cache holds at most one leaf beyond the config.
	fallbackName = "localhost"
)

// CA holds the CA certificate and key and caches minted leaves per server name.
type CA struct {
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	certPEM []byte

	mu     sync.Mutex
	hosts  map[string]bool             // names a leaf may be minted for (SetHosts)
	leaves map[string]*tls.Certificate // every key is in hosts or is fallbackName
}

// CertPath and KeyPath are where EnsureCA reads or writes the CA material.
func CertPath(dir string) string { return filepath.Join(dir, caCertFile) }
func KeyPath(dir string) string  { return filepath.Join(dir, caKeyFile) }

// CertPEM returns the CA certificate in PEM form (for keychain install and for
// clients that need it as a trust root, e.g. curl --cacert).
func (ca *CA) CertPEM() []byte { return ca.certPEM }

// EnsureCA loads the CA from dir, generating and persisting a fresh one if it
// is not already present. The private key file is written mode 0600.
func EnsureCA(dir string) (*CA, error) {
	certPath, keyPath := CertPath(dir), KeyPath(dir)
	if fileExists(certPath) && fileExists(keyPath) {
		return loadCA(certPath, keyPath)
	}
	return generateCA(dir, certPath, keyPath)
}

// SetHosts replaces the names the CA mints leaves for: the hosts d-man answers
// (route hosts and block-list hosts). Names match case-insensitively. A cached
// leaf for a name that is no longer in the set is dropped, so a routes reload
// that removes a host also removes its certificate.
func (ca *CA) SetHosts(names []string) {
	hosts := make(map[string]bool, len(names))
	for _, n := range names {
		if n = normalizeName(n); n != "" {
			hosts[n] = true
		}
	}
	ca.mu.Lock()
	defer ca.mu.Unlock()
	ca.hosts = hosts
	for name := range ca.leaves {
		if !ca.allowed(name) {
			delete(ca.leaves, name)
		}
	}
}

// GetCertificate is a tls.Config.GetCertificate callback: it returns a leaf
// certificate for the client's server name, minting and caching one on first
// use. A client that presents no server name gets the "localhost" leaf. A name
// outside SetHosts gets nil and no error: crypto/tls, with no static
// Certificates to fall back on, then aborts the handshake with a fatal
// unrecognized_name alert, and nothing is minted or cached.
func (ca *CA) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	name := normalizeName(hello.ServerName)
	if name == "" {
		name = fallbackName
	}
	ca.mu.Lock()
	defer ca.mu.Unlock()
	if c, ok := ca.leaves[name]; ok {
		return c, nil
	}
	if !ca.allowed(name) {
		return nil, nil
	}
	leaf, err := ca.mintLeaf(name)
	if err != nil {
		return nil, err
	}
	ca.leaves[name] = leaf
	return leaf, nil
}

// allowed reports whether a leaf may be minted for the normalized name. The
// caller holds ca.mu.
func (ca *CA) allowed(name string) bool {
	return name == fallbackName || ca.hosts[name]
}

// normalizeName folds a server name the way the proxy folds a Host header:
// trimmed, trailing dot removed, lowercased. DNS names are case-insensitive, so
// "Reddit.com" and "reddit.com" share one leaf.
func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
}

func (ca *CA) mintLeaf(name string) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    now.Add(-backdate),
		NotAfter:     now.Add(leafValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(name); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{name}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		return nil, err
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
		Leaf:        leaf,
	}, nil
}

func generateCA(dir, certPath, keyPath string) (*CA, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create ca dir %s: %w", dir, err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: CommonName},
		NotBefore:             now.Add(-backdate),
		NotAfter:              now.Add(caValidity),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return nil, fmt.Errorf("write ca cert: %w", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		return nil, fmt.Errorf("write ca key: %w", err)
	}
	return newCA(cert, key, certPEM), nil
}

func loadCA(certPath, keyPath string) (*CA, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	cb, _ := pem.Decode(certPEM)
	if cb == nil {
		return nil, fmt.Errorf("ca cert %s: not PEM", certPath)
	}
	cert, err := x509.ParseCertificate(cb.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse ca cert: %w", err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	kb, _ := pem.Decode(keyPEM)
	if kb == nil {
		return nil, fmt.Errorf("ca key %s: not PEM", keyPath)
	}
	key, err := x509.ParseECPrivateKey(kb.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse ca key: %w", err)
	}
	return newCA(cert, key, certPEM), nil
}

func newCA(cert *x509.Certificate, key *ecdsa.PrivateKey, certPEM []byte) *CA {
	return &CA{
		cert:    cert,
		key:     key,
		certPEM: certPEM,
		hosts:   make(map[string]bool),
		leaves:  make(map[string]*tls.Certificate),
	}
}

func randomSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
