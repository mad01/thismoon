package drift

import (
	"strings"
	"testing"
)

func TestDiff(t *testing.T) {
	rendered := `{
  "filesystem": {"allowRead": ["~/code/bin", "~/.local/share/events"], "denyRead": ["~"]},
  "network": {"allowLocalBinding": true, "allowedDomains": []}
}`
	tests := []struct {
		name      string
		committed string
		wantDiff  []string // substrings; none means no drift
	}{
		{
			name: "key order, layout and list order are not drift",
			committed: `{"network": {"allowedDomains": [], "allowLocalBinding": true},
"filesystem": {"denyRead": ["~"], "allowRead": ["~/.local/share/events", "~/code/bin", "~/code/bin"]}}`,
		},
		{
			name: "a dropped allow is drift",
			committed: `{"network": {"allowedDomains": [], "allowLocalBinding": true},
"filesystem": {"denyRead": ["~"], "allowRead": ["~/code/bin"]}}`,
			wantDiff: []string{`+      "~/.local/share/events",`},
		},
		{
			name: "a changed flag is drift",
			committed: `{"network": {"allowedDomains": [], "allowLocalBinding": false},
"filesystem": {"denyRead": ["~"], "allowRead": ["~/code/bin", "~/.local/share/events"]}}`,
			wantDiff: []string{
				`-    "allowLocalBinding": false,`,
				`+    "allowLocalBinding": true,`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff, err := Diff(
				"committed.json",
				[]byte(tt.committed),
				"service-info.yaml",
				[]byte(rendered),
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(tt.wantDiff) == 0 {
				if diff != "" {
					t.Fatalf("unexpected drift:\n%s", diff)
				}
				return
			}
			for _, w := range tt.wantDiff {
				if !strings.Contains(diff, w) {
					t.Errorf("diff lacks %q:\n%s", w, diff)
				}
			}
		})
	}
}

func TestCanonicalRejectsNonJSON(t *testing.T) {
	if _, err := Canonical([]byte("{")); err == nil {
		t.Fatal("Canonical accepted broken JSON")
	}
}
