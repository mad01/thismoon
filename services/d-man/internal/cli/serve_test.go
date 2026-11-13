package cli

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/thismoon/services/d-man/internal/config"
	"github.com/mad01/thismoon/services/d-man/internal/hosts"
	"github.com/mad01/thismoon/services/d-man/internal/tlsca"
)

// TestReloadLimitsCertificatesToConfiguredHosts drives serve's reload path on
// scratch paths with an in-test CA: after each routes load the CA mints leaves
// for exactly the route hosts and block-list hosts of the last good file.
func TestReloadLimitsCertificatesToConfiguredHosts(t *testing.T) {
	dir := t.TempDir()
	// writeRoutes primes the hosts file so no reload writes it or emits an
	// event; this keeps any stray emit off the events service on the machine.
	t.Setenv("EVENTS_BASE_URL", "http://127.0.0.1:1")
	setFlag(t, &flagConfig, filepath.Join(dir, "routes.toml"))
	setFlag(t, &flagHostsFile, filepath.Join(dir, "hosts"))
	setFlag(t, &flagSettingsFile, filepath.Join(dir, "ui-settings.json"))
	ca, err := tlsca.EnsureCA(filepath.Join(dir, "ca"))
	if err != nil {
		t.Fatalf("EnsureCA: %v", err)
	}

	writeRoutes(t, `blocklist = ["Reddit.com"]

[[route]]
name = "csl"
port = 7424

[[route]]
name = "old"
port = 7425

[[route]]
name = "alias"
cname = "csl"
`)
	rh, err := newReloadableHandler(ca)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	assertCerts(t, ca, map[string]bool{
		"csl.this": true, "old.this": true, "alias.this": true, "reddit.com": true,
		"unknown.example": false, "www.reddit.com": false,
	})

	writeRoutes(t, `blocklist = ["reddit.com"]

[[route]]
name = "csl"
port = 7424
`)
	if err := reloadOnce(rh); err != nil {
		t.Fatalf("reload: %v", err)
	}
	assertCerts(t, ca, map[string]bool{
		"csl.this": true, "reddit.com": true, "old.this": false, "alias.this": false,
	})

	// A bad edit fails the reload and keeps the last good names.
	if err := os.WriteFile(flagConfig, []byte("[[route]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := reloadOnce(rh); err == nil {
		t.Fatal("reload of a broken routes file succeeded")
	}
	assertCerts(t, ca, map[string]bool{"csl.this": true, "old.this": false})

	if _, err := os.Stat(flagHostsFile + ".d-man.bak"); err == nil {
		t.Error("a reload rewrote the scratch hosts file; the priming in writeRoutes is off")
	}
}

// writeRoutes writes body as the routes file and primes the scratch hosts file
// with the block a reload of it renders, so the reload finds nothing to sync.
func writeRoutes(t *testing.T, body string) {
	t.Helper()
	cfg, err := config.Parse([]byte(body), "test routes")
	if err != nil {
		t.Fatalf("test routes: %v", err)
	}
	if err := os.WriteFile(flagConfig, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	primed := hosts.Splice(nil, hosts.Render(cfg.Hosts()))
	if err := os.WriteFile(flagHostsFile, primed, 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertCerts(t *testing.T, ca *tlsca.CA, want map[string]bool) {
	t.Helper()
	for name, ok := range want {
		cert, err := ca.GetCertificate(&tls.ClientHelloInfo{ServerName: name})
		if err != nil {
			t.Errorf("GetCertificate(%q): %v", name, err)
		}
		if got := cert != nil; got != ok {
			t.Errorf("certificate for %q = %v, want %v", name, got, ok)
		}
	}
}

// setFlag points a package flag variable at v for the test and restores it.
func setFlag(t *testing.T, p *string, v string) {
	t.Helper()
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}
