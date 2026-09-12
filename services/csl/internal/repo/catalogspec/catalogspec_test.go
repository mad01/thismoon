package catalogspec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    Component
		wantOK  bool
		wantErr bool
	}{
		{
			name: "backstage component",
			in: "apiVersion: backstage.io/v1alpha1\nkind: Component\n" +
				"metadata:\n  name: petstore\nspec:\n  type: service\n" +
				"  owner: group:default/platform\n  system: shop\n",
			want:   Component{Name: "petstore", Owner: "group:default/platform", System: "shop"},
			wantOK: true,
		},
		{
			name: "catalog service component",
			in: "apiVersion: catalog.mad01/v1alpha1\nkind: Component\n" +
				"metadata:\n  name: csl\nspec:\n  owner: mad01\n  system: thismoon\n",
			want:   Component{Name: "csl", Owner: "mad01", System: "thismoon"},
			wantOK: true,
		},
		{
			name:   "missing apiVersion is the catalog default",
			in:     "kind: Component\nmetadata:\n  name: csl\nspec:\n  owner: mad01\n",
			want:   Component{Name: "csl", Owner: "mad01"},
			wantOK: true,
		},
		{
			name: "any version under an accepted group",
			in: "apiVersion: backstage.io/v1beta1\nkind: Component\n" +
				"metadata:\n  name: petstore\nspec:\n  owner: team-a\n",
			want:   Component{Name: "petstore", Owner: "team-a"},
			wantOK: true,
		},
		{
			name:   "kind compares case-insensitively",
			in:     "kind: component\nmetadata:\n  name: csl\nspec:\n  owner: mad01\n",
			want:   Component{Name: "csl", Owner: "mad01"},
			wantOK: true,
		},
		{
			name: "first component after a system document",
			in: "apiVersion: backstage.io/v1alpha1\nkind: System\nmetadata:\n  name: shop\n" +
				"spec:\n  owner: team-a\n---\n" +
				"apiVersion: backstage.io/v1alpha1\nkind: Component\nmetadata:\n  name: cart\n" +
				"spec:\n  owner: team-a\n  system: shop\n---\n" +
				"apiVersion: backstage.io/v1alpha1\nkind: Component\nmetadata:\n  name: second\n" +
				"spec:\n  owner: team-b\n",
			want:   Component{Name: "cart", Owner: "team-a", System: "shop"},
			wantOK: true,
		},
		{
			name: "system only is not a component",
			in:   "apiVersion: backstage.io/v1alpha1\nkind: System\nmetadata:\n  name: shop\n",
		},
		{
			name: "unknown apiVersion group is skipped",
			in: "apiVersion: example.com/v1\nkind: Component\nmetadata:\n  name: x\n" +
				"spec:\n  owner: y\n",
		},
		{
			name: "component without a name is skipped",
			in:   "apiVersion: backstage.io/v1alpha1\nkind: Component\nspec:\n  owner: y\n",
		},
		{
			name: "empty file",
			in:   "",
		},
		{
			name: "comment-only document then component",
			in: "# just a comment\n---\napiVersion: backstage.io/v1alpha1\nkind: Component\n" +
				"metadata:\n  name: x\nspec:\n  owner: y\n",
			want:   Component{Name: "x", Owner: "y"},
			wantOK: true,
		},
		{
			name:    "malformed yaml is an error",
			in:      "kind: Component\nmetadata: [unclosed\n",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok, err := Parse([]byte(tc.in))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Parse() = %+v, %v, nil; want an error", got, ok)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() error: %v", err)
			}
			if ok != tc.wantOK {
				t.Fatalf("Parse() ok = %v, want %v (got %+v)", ok, tc.wantOK, got)
			}
			if got != tc.want {
				t.Errorf("Parse() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestRead(t *testing.T) {
	component := func(name string) string {
		return "apiVersion: backstage.io/v1alpha1\nkind: Component\nmetadata:\n  name: " + name +
			"\nspec:\n  owner: team\n"
	}

	t.Run("no descriptor", func(t *testing.T) {
		dir := t.TempDir()
		c, ok, err := Read(dir)
		if err != nil || ok {
			t.Fatalf("Read() = %+v, %v, %v; want zero, false, nil", c, ok, err)
		}
	})

	t.Run("service-info.yaml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "service-info.yaml")
		writeFile(t, path, component("svc"))
		c, ok, err := Read(dir)
		if err != nil || !ok {
			t.Fatalf("Read() = %+v, %v, %v; want a component", c, ok, err)
		}
		if c.Name != "svc" || c.File != path {
			t.Errorf("Read() = %+v, want name svc from %s", c, path)
		}
	})

	t.Run("catalog-info.yaml wins over service-info.yaml", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "catalog-info.yaml"), component("backstage-name"))
		writeFile(t, filepath.Join(dir, "service-info.yaml"), component("catalog-name"))
		c, ok, err := Read(dir)
		if err != nil || !ok {
			t.Fatalf("Read() = %+v, %v, %v; want a component", c, ok, err)
		}
		if c.Name != "backstage-name" {
			t.Errorf("Read().Name = %q, want the catalog-info.yaml entity", c.Name)
		}
	})

	t.Run("descriptor without a component", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "catalog-info.yaml"),
			"apiVersion: backstage.io/v1alpha1\nkind: System\nmetadata:\n  name: shop\n")
		c, ok, err := Read(dir)
		if err != nil || ok {
			t.Fatalf("Read() = %+v, %v, %v; want zero, false, nil", c, ok, err)
		}
	})

	t.Run("pointer file names the descriptor to read", func(t *testing.T) {
		dir := t.TempDir()
		target := filepath.Join(dir, "services", "csl", "service-info.yaml")
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, target, component("pointed"))
		// The root descriptor is present too, and loses: the pointer is the
		// explicit instruction.
		writeFile(t, filepath.Join(dir, "catalog-info.yaml"), component("root"))
		writeFile(
			t,
			filepath.Join(dir, PointerFile),
			"descriptor: services/csl/service-info.yaml\n",
		)
		c, ok, err := Read(dir)
		if err != nil || !ok {
			t.Fatalf("Read() = %+v, %v, %v; want the pointed component", c, ok, err)
		}
		if c.Name != "pointed" || c.File != target {
			t.Errorf("Read() = %+v, want name pointed from %s", c, target)
		}
	})

	t.Run("pointer file errors", func(t *testing.T) {
		tests := []struct {
			name    string
			content string
			wantErr string
		}{
			{"no descriptor key", "other: x\n", "missing the descriptor key"},
			{"absolute path", "descriptor: /etc/passwd\n", "inside the repo"},
			{"escapes the repo", "descriptor: ../elsewhere/catalog-info.yaml\n", "inside the repo"},
			{"missing target", "descriptor: nope/catalog-info.yaml\n", "nope/catalog-info.yaml"},
			{"not a mapping", "- a\n- b\n", "yaml"},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, PointerFile), tc.content)
				_, _, err := Read(dir)
				if err == nil {
					t.Fatal("Read() error = nil, want one")
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error %q does not mention %q", err, tc.wantErr)
				}
			})
		}
	})

	t.Run("pointer through a symlink out of the repo is refused", func(t *testing.T) {
		outside := t.TempDir()
		writeFile(t, filepath.Join(outside, "secret.yaml"), component("leaked"))
		dir := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(dir, "vendor")); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(dir, PointerFile), "descriptor: vendor/secret.yaml\n")
		_, _, err := Read(dir)
		if err == nil || !strings.Contains(err.Error(), "outside the repo") {
			t.Fatalf("Read() error = %v, want a refusal naming the escape", err)
		}
	})

	t.Run("pointer through a symlink inside the repo is fine", func(t *testing.T) {
		dir := t.TempDir()
		real := filepath.Join(dir, "services", "api", "catalog-info.yaml")
		if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, real, component("api"))
		if err := os.Symlink(filepath.Join(dir, "services"), filepath.Join(dir, "svc")); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(dir, PointerFile), "descriptor: svc/api/catalog-info.yaml\n")
		c, ok, err := Read(dir)
		if err != nil || !ok || c.Name != "api" {
			t.Fatalf("Read() = %+v, %v, %v; want api", c, ok, err)
		}
		if c.File != filepath.Join(dir, "svc", "api", "catalog-info.yaml") {
			t.Errorf("File = %q, want the lexical path under the repo", c.File)
		}
	})

	t.Run("oversized descriptor is refused", func(t *testing.T) {
		dir := t.TempDir()
		big := make([]byte, maxDescriptorBytes+1)
		for i := range big {
			big[i] = '#'
		}
		writeFile(t, filepath.Join(dir, "catalog-info.yaml"), string(big))
		_, _, err := Read(dir)
		if err == nil || !strings.Contains(err.Error(), "larger than a descriptor") {
			t.Fatalf("Read() error = %v, want a size refusal", err)
		}
	})

	t.Run("malformed descriptor names the file", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "catalog-info.yaml")
		writeFile(t, path, "kind: Component\nmetadata: [unclosed\n")
		_, _, err := Read(dir)
		if err == nil {
			t.Fatal("Read() error = nil, want a parse error")
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("error %q does not name %s", err, path)
		}
	})
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
