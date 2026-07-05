package discover

import (
	"os"
	"path/filepath"
	"testing"
)

const npmLockJSON = `{
  "name": "demo",
  "lockfileVersion": 3,
  "packages": {
    "": {
      "name": "demo",
      "devDependencies": { "typescript": "^5.0.0" },
      "dependencies": { "left-pad": "^1.3.0" }
    },
    "node_modules/typescript": { "version": "5.4.2", "dev": true },
    "node_modules/left-pad": { "version": "1.3.0" },
    "node_modules/left-pad/node_modules/nested": { "version": "2.0.1" }
  }
}`

func TestParseNPMLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package-lock.json")
	if err := os.WriteFile(path, []byte(npmLockJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	pkgs, err := parseNPMLock(path)
	if err != nil {
		t.Fatalf("parseNPMLock: %v", err)
	}
	if len(pkgs) != 3 {
		t.Fatalf("got %d packages, want 3: %+v", len(pkgs), pkgs)
	}

	byName := map[string]Package{}
	for _, p := range pkgs {
		if p.Ecosystem != "npm" {
			t.Errorf("%s ecosystem = %q, want npm", p.Name, p.Ecosystem)
		}
		byName[p.Name] = p
	}

	if ts := byName["typescript"]; ts.Version != "5.4.2" || !ts.Direct {
		t.Errorf("typescript = %+v, want version 5.4.2 direct", ts)
	}
	if lp := byName["left-pad"]; lp.Version != "1.3.0" || !lp.Direct {
		t.Errorf("left-pad = %+v, want version 1.3.0 direct", lp)
	}
	// Nested transitive dep: name is the segment after the final node_modules/,
	// and it is not in the root dependency sets, so not direct.
	if n := byName["nested"]; n.Version != "2.0.1" || n.Direct {
		t.Errorf("nested = %+v, want version 2.0.1 indirect", n)
	}
}
