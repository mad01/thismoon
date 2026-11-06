package vault

import "testing"

func TestNormalizeTitle(t *testing.T) {
	tests := map[string]string{
		"Kubernetes: Upgrades":   "kubernetes upgrades",
		"  kubernetes-upgrades ": "kubernetes upgrades",
		"C++":                    "c++",
		"C#":                     "c#",
		"Größe & Maß":            "größe maß",
		"---":                    "",
	}
	for in, want := range tests {
		if got := NormalizeTitle(in); got != want {
			t.Errorf("NormalizeTitle(%q) = %q, want %q", in, got, want)
		}
	}
	if NormalizeTitle("C++") == NormalizeTitle("C#") {
		t.Error("C++ and C# normalize alike")
	}
}

func TestSameTitleSkipsRevokedAndOutsideTiers(t *testing.T) {
	v := newTestVault(t, map[string]string{
		"notes/a.md": "---\ntitle: Topic\n---\n",
		"notes/b.md": "---\ntitle: Other\naliases: [topic]\n---\n",
		"notes/c.md": "---\ntitle: Topic\nstatus: superseded\nsuperseded_by: id:x\n---\n",
		"README.md":  "---\ntitle: Topic\n---\n",
	})
	v.Name = "personal"
	got, err := NewSet(v).SameTitle("TOPIC")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Ref.Path != "notes/a.md" || got[1].Alias != "topic" {
		t.Errorf("SameTitle = %+v, want notes/a.md and the alias on notes/b.md", got)
	}
}
