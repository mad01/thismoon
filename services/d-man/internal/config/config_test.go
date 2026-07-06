package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "routes.toml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	path := writeConfig(t, `
[[route]]
name = "csl"
port = 7424
`)
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Suffix != DefaultSuffix {
		t.Errorf("suffix = %q, want %q", c.Suffix, DefaultSuffix)
	}
	if got := c.Routes[0].Target; got != DefaultTarget {
		t.Errorf("target = %q, want %q", got, DefaultTarget)
	}
	if got := c.Routes[0].Host(c.Suffix); got != "csl.this" {
		t.Errorf("host = %q, want csl.this", got)
	}
	if got := c.Routes[0].Backend(); got != "127.0.0.1:7424" {
		t.Errorf("backend = %q, want 127.0.0.1:7424", got)
	}
}

func TestRouteMapAndHosts(t *testing.T) {
	path := writeConfig(t, `
suffix = "lab"
[[route]]
name = "csl"
port = 7424
[[route]]
name = "p"
port = 7423
`)
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rm := c.RouteMap()
	if rm["csl.lab"] != "127.0.0.1:7424" || rm["p.lab"] != "127.0.0.1:7423" {
		t.Errorf("RouteMap = %v", rm)
	}
	hosts := c.Hosts()
	if len(hosts) != 2 {
		t.Errorf("Hosts = %v", hosts)
	}
}

func TestSitesExcludesCnames(t *testing.T) {
	path := writeConfig(t, `
[[route]]
name = "csl"
port = 7424
[[route]]
name = "present"
port = 7423
[[route]]
name = "p"
cname = "present"
`)
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	sites := c.Sites()
	if len(sites) != 2 {
		t.Fatalf("Sites len = %d, want 2 (cname excluded): %v", len(sites), sites)
	}
	if sites[0].Name != "csl" || sites[0].Host != "csl.this" || sites[0].URL != "http://csl.this/" {
		t.Errorf("Sites[0] = %+v", sites[0])
	}
	for _, s := range sites {
		if s.Name == "p" {
			t.Errorf("Sites must exclude cname alias %q", s.Name)
		}
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]string{
		"empty name":     "[[route]]\nport = 80\n",
		"bad name":       "[[route]]\nname = \"a b\"\nport = 80\n",
		"bad suffix":     "suffix = \"this!\"\n[[route]]\nname = \"x\"\nport = 80\n",
		"no port/cname":  "[[route]]\nname = \"x\"\n",
		"port too high":  "[[route]]\nname = \"x\"\nport = 70000\n",
		"duplicate":      "[[route]]\nname = \"x\"\nport = 80\n[[route]]\nname = \"x\"\nport = 81\n",
		"bad target":     "[[route]]\nname = \"x\"\nport = 80\ntarget = \"not a host\"\n",
		"port and cname": "[[route]]\nname=\"a\"\nport=80\n[[route]]\nname=\"b\"\nport=81\ncname=\"a\"\n",
		"cname missing":  "[[route]]\nname = \"b\"\ncname = \"nope\"\n",
		"cname cycle":    "[[route]]\nname=\"a\"\ncname=\"b\"\n[[route]]\nname=\"b\"\ncname=\"a\"\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Load(writeConfig(t, body)); err == nil {
				t.Errorf("expected error for %s", name)
			}
		})
	}
}

func TestCnameResolves(t *testing.T) {
	path := writeConfig(t, `
[[route]]
name = "present"
port = 7423
[[route]]
name = "p"
cname = "present"
`)
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	rm := c.RouteMap()
	if rm["p.this"] != "127.0.0.1:7423" {
		t.Errorf("cname p.this -> %q, want 127.0.0.1:7423", rm["p.this"])
	}
	if rm["present.this"] != "127.0.0.1:7423" {
		t.Errorf("present.this -> %q", rm["present.this"])
	}
	// /etc/hosts must include the alias too.
	if len(c.Hosts()) != 2 {
		t.Errorf("Hosts = %v, want 2", c.Hosts())
	}
}

func TestCnameChain(t *testing.T) {
	path := writeConfig(t, `
[[route]]
name = "present"
port = 7423
[[route]]
name = "p"
cname = "present"
[[route]]
name = "pp"
cname = "p"
`)
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := c.RouteMap()["pp.this"]; got != "127.0.0.1:7423" {
		t.Errorf("chained cname pp.this -> %q, want 127.0.0.1:7423", got)
	}
}

func TestValidateAcceptsIPv6Target(t *testing.T) {
	path := writeConfig(t, "[[route]]\nname = \"x\"\nport = 80\ntarget = \"::1\"\n")
	if _, err := Load(path); err != nil {
		t.Errorf("Load: %v", err)
	}
}
