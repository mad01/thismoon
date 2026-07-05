package discover

import (
	"os"
	"path/filepath"
	"testing"
)

const tmanPlistXML = `<?xml version="1.0" encoding="UTF-8"?>
<!-- Managed by t-man - DO NOT EDIT MANUALLY -->
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>KeepAlive</key><true/><key>Label</key><string>speak-web</string><key>ProgramArguments</key><array><string>/Users/u/code/bin/speak</string><string>serve</string><string>--port</string><string>7425</string></array><key>RunAtLoad</key><true/><key>TManMetadata</key><dict><key>Hash</key><string>abc</string><key>ManagedBy</key><string>t-man</string><key>Version</key><string>80d09e8</string></dict></dict></plist>`

const foreignPlistXML = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>Label</key><string>com.vendor.thing</string><key>ProgramArguments</key><array><string>/usr/bin/true</string></array></dict></plist>`

const noPortPlistXML = `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict><key>Label</key><string>dark-notify</string><key>ProgramArguments</key><array><string>/opt/homebrew/bin/dark-notify</string><string>-c</string><string>/x.sh</string></array><key>TManMetadata</key><dict><key>ManagedBy</key><string>t-man</string></dict></dict></plist>`

const routesTOML = `suffix = "this"

[[route]]
name = "speak"
port = 7425

[[route]]
name = "p"
cname = "speak"
`

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScan(t *testing.T) {
	agents := t.TempDir()
	daemons := t.TempDir()
	writeFile(t, agents, "speak-web.plist", tmanPlistXML)
	writeFile(t, agents, "com.vendor.thing.plist", foreignPlistXML)
	writeFile(t, agents, "dark-notify.plist", noPortPlistXML)
	writeFile(t, daemons, "d-man.plist", noPortPlistXML) // reuse shape; label dark-notify

	routes := filepath.Join(t.TempDir(), "routes.toml")
	if err := os.WriteFile(routes, []byte(routesTOML), 0o644); err != nil {
		t.Fatal(err)
	}

	services, err := Scan(agents, daemons, routes)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 3 {
		t.Fatalf(
			"got %d services, want 3 (foreign plist must be skipped): %+v",
			len(services),
			services,
		)
	}

	byLabel := map[string][]Service{}
	for _, s := range services {
		byLabel[s.Label] = append(byLabel[s.Label], s)
	}

	web := byLabel["speak-web"][0]
	if web.Port != 7425 || !web.HTTP() {
		t.Errorf("speak-web port = %d, want 7425", web.Port)
	}
	if web.Link != "http://speak.this/" {
		t.Errorf("speak-web link = %q, want http://speak.this/", web.Link)
	}
	if web.Binary != "/Users/u/code/bin/speak" {
		t.Errorf("speak-web binary = %q", web.Binary)
	}

	if len(byLabel["dark-notify"]) != 2 {
		t.Fatalf("expected dark-notify from both dirs")
	}
	for _, dn := range byLabel["dark-notify"] {
		if dn.HTTP() || dn.Link != "" {
			t.Errorf("dark-notify should be a PID-checked service without link: %+v", dn)
		}
	}
}

func TestScanMissingDirsAndRoutes(t *testing.T) {
	services, err := Scan(
		filepath.Join(t.TempDir(), "nope"),
		filepath.Join(t.TempDir(), "nope2"),
		"/nonexistent/routes.toml",
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 0 {
		t.Fatalf("got %d services, want 0", len(services))
	}
}

func TestPortFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"separate flag", []string{"/bin/x", "serve", "--port", "7423"}, 7423},
		{"equals form", []string{"/bin/x", "serve", "--port=8765"}, 8765},
		{"no port", []string{"/bin/x", "-c", "/x.sh"}, 0},
		{"port flag last without value", []string{"/bin/x", "--port"}, 0},
		{"non-numeric", []string{"/bin/x", "--port", "abc"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := portFromArgs(tt.args); got != tt.want {
				t.Errorf("portFromArgs(%v) = %d, want %d", tt.args, got, tt.want)
			}
		})
	}
}
