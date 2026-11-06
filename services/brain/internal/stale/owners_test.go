package stale

import (
	"strings"
	"testing"
)

func TestOwners(t *testing.T) {
	o := ParseOwners([]byte(`# vault owners
*                @team/all
notes/           @team/notes
/notes/infra/    @team/infra @alex
*.secret.md
MOCs/*.md        @team/maps  # maps directly under MOCs
**/runbooks/**   @team/ops
`))
	tests := map[string]string{
		"inbox/x.md":                "@team/all",
		"notes/x.md":                "@team/notes",
		"notes/infra/dns.md":        "@team/infra @alex",
		"notes/infra/deep/dns.md":   "@team/infra @alex",
		"notes/a.secret.md":         "",
		"MOCs/topics.md":            "@team/maps",
		"MOCs/sub/topics.md":        "@team/all",
		"outputs/runbooks/a/b.md":   "@team/ops",
		"other/notes/not-anchor.md": "@team/notes",
	}
	for path, want := range tests {
		if got := strings.Join(o.Of(path), " "); got != want {
			t.Errorf("Of(%q) = %q, want %q", path, got, want)
		}
	}
	if got := (Owners{}).Of("notes/x.md"); got != nil {
		t.Errorf("no CODEOWNERS: Of = %v, want nil", got)
	}
}
