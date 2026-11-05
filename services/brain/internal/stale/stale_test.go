package stale

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/drift"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

var staleDay = time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC)

const (
	idOld = "01JC0000000000000000000001"
	idNew = "01JC0000000000000000000002"
)

// staleVault is one vault whose notes cover every reason and the cases
// that make none.
var staleVault = map[string]string{
	"CODEOWNERS": "notes/infra/ @team/infra\n",
	"notes/infra/dns.md": "---\ntitle: DNS\nstatus: validated\nreview_by: 2026-09-01\n---\n" +
		"See [[retracted]] (a mention, not a dependency).\n",
	"notes/infra/fresh.md": "---\ntitle: Fresh\nstatus: validated\nreview_by: 2027-09-01\n---\n",
	"notes/claim.md": "---\ntitle: Claim\nsteward: '@kim'\nstatus: disputed\n" +
		"status_reason: one machine only\nstatus_at: 2026-10-20\n---\n",
	"notes/built-on.md": "---\ntitle: Built On\nreview_by: 2027-01-01\nsources:\n" +
		"  - ref: id:" + idOld + "\n---\n",
	"notes/retracted.md": "---\ntitle: Retracted\nid: " + idOld + "\nstatus: retracted\n" +
		"retracted_reason: error\nretracted_at: 2026-10-01\nstatus_at: 2026-10-01\n---\n",
	"notes/second-hand.md": "---\ntitle: Second Hand\nsources:\n  - ref: notes/built-on.md\n---\n",
	"inbox/old.md":         "---\ntitle: Old\nstatus: raw\nexpires_at: 2026-10-01\n---\n",
	"inbox/live.md":        "---\ntitle: Live\nstatus: raw\nexpires_at: 2026-12-01\n---\n",
	"notes/drifty.md":      "---\ntitle: Drifty\nid: " + idNew + "\n---\n",
}

func buildQueue(t *testing.T, limit int, driftOf func(*vault.NoteView) []drift.Finding) Queue {
	t.Helper()
	root := t.TempDir()
	for rel, content := range staleVault {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	set := vault.NewSet(&vault.Vault{Root: root, Name: "main"})
	sn, err := set.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	var notes []*vault.NoteView
	for _, n := range sn.Notes() {
		if n.Tier() != "" {
			notes = append(notes, n)
		}
	}
	q, err := Build(
		Options{Snapshot: sn, Notes: notes, Now: staleDay, Limit: limit, Drift: driftOf},
	)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

// summary renders the queue as "domain(total): path kind,kind; ..." lines.
func summary(q Queue) []string {
	var out []string
	for _, g := range q.Groups {
		var notes []string
		for _, e := range g.Notes {
			var kinds []string
			for _, r := range e.Reasons {
				kinds = append(kinds, r.Kind)
			}
			notes = append(notes, e.Path+" "+strings.Join(kinds, ","))
		}
		out = append(out, g.Domain+"("+g.From+"): "+strings.Join(notes, "; "))
	}
	return out
}

func TestBuildGroupsTheQueueByOwner(t *testing.T) {
	q := buildQueue(t, 0, nil)
	want := []string{
		"@kim(steward): notes/claim.md disputed",
		"@team/infra(codeowners): notes/infra/dns.md expired",
		"unowned(none): inbox/old.md expired; notes/built-on.md dependent",
	}
	if got := summary(q); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("queue =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if q.Total != 4 || q.Drift {
		t.Errorf("total %d drift %v, want 4 and false", q.Total, q.Drift)
	}
	dep := q.Groups[2].Notes[1].Reasons[0]
	if !strings.Contains(dep.Detail, "main:notes/retracted.md") || dep.Since != "2026-10-01" {
		t.Errorf("dependent reason = %+v", dep)
	}
}

func TestBuildWithDriftAndALimit(t *testing.T) {
	driftOf := func(n *vault.NoteView) []drift.Finding {
		if n.Ref.Path != "notes/drifty.md" {
			return nil
		}
		return []drift.Finding{
			{Kind: drift.KindChanged, Source: "acme/app:a.go@abcdef1"},
			{Kind: drift.KindUnresolved, Source: "acme/gone:b.go@abcdef1"},
		}
	}
	q := buildQueue(t, 1, driftOf)
	unowned := q.Groups[len(q.Groups)-1]
	if unowned.Domain != Unowned || unowned.Total != 3 || len(unowned.Notes) != 1 {
		t.Fatalf("unowned group = %+v, want 3 notes, 1 shown", unowned)
	}
	if !q.Drift || q.Total != 5 {
		t.Errorf("drift %v total %d, want true and 5", q.Drift, q.Total)
	}
	var drifty *Entry
	q2 := buildQueue(t, 0, driftOf)
	for _, e := range q2.Groups[len(q2.Groups)-1].Notes {
		if e.Path == "notes/drifty.md" {
			drifty = &e
		}
	}
	if drifty == nil || drifty.Reasons[0].Detail != "changed acme/app:a.go@abcdef1" {
		t.Errorf("drifty entry = %+v, want one drift reason without the unresolved source", drifty)
	}
}
