// Package stale is brain stale: the queue of notes that need a human look,
// computed on every run and never stored. A note is in it when its
// review_by has passed (for a raw note, its expires_at), when it is
// disputed, when drift found its pinned sources changed (only when the
// caller runs drift), or when a sources[] entry rests on a note that was
// superseded, retracted or withdrawn. Only sources make a dependent:
// wikilinks are mentions, and a dependent's own dependents are not queued.
// The queue is grouped by domain: the CODEOWNERS owners of the note's
// path, else its steward, else unowned.
package stale

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/drift"
	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// Reason kinds.
const (
	ReasonExpired   = "expired"
	ReasonDisputed  = "disputed"
	ReasonDrift     = "drift"
	ReasonDependent = "dependent"
)

// Where a group's domain comes from.
const (
	FromCodeowners = "codeowners"
	FromSteward    = "steward"
	FromNone       = "none"
)

// Unowned is the domain of a note neither CODEOWNERS nor a steward claims.
const Unowned = "unowned"

// DefaultLimit is how many notes each group shows by default.
const DefaultLimit = 10

// Reason is one cause for a note to be in the queue.
type Reason struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
	// Since is the day the reason began, when it has one.
	Since string `json:"since,omitempty"`
}

// Entry is one note in the queue.
type Entry struct {
	Vault   string   `json:"vault"`
	Path    string   `json:"path"`
	Title   string   `json:"title,omitempty"`
	Status  string   `json:"status,omitempty"`
	Reasons []Reason `json:"reasons"`
}

// Group is the queued notes of one domain. Total counts them all; Notes
// holds the first Limit, oldest reason first.
type Group struct {
	Domain string  `json:"domain"`
	From   string  `json:"from"`
	Total  int     `json:"total"`
	Notes  []Entry `json:"notes"`
}

// Queue is the stale queue.
type Queue struct {
	Total int `json:"total"`
	// Drift says whether drift ran for this queue.
	Drift  bool    `json:"drift"`
	Groups []Group `json:"groups"`
}

// Options selects what Build queues.
type Options struct {
	// Snapshot is every configured vault, so sources refs resolve.
	Snapshot *vault.Snapshot
	// Notes are the notes considered.
	Notes []*vault.NoteView
	// Now dates review_by and expires_at; zero means time.Now.
	Now time.Time
	// Limit caps the notes each group shows; zero means DefaultLimit.
	Limit int
	// Drift, when set, returns a note's drift findings; nil skips drift.
	Drift func(*vault.NoteView) []drift.Finding
}

// Build computes the queue.
func Build(opts Options) (Queue, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	if opts.Limit <= 0 {
		opts.Limit = DefaultLimit
	}
	b := builder{opts: opts, owners: map[*vault.Vault]Owners{}, order: map[string]int{}}
	groups := map[string]*Group{}
	var keys []string
	for i, n := range opts.Notes {
		if _, ok := b.order[n.Ref.Vault.Name]; !ok {
			b.order[n.Ref.Vault.Name] = i
		}
		reasons := b.reasons(n)
		if len(reasons) == 0 {
			continue
		}
		domain, from, err := b.domain(n)
		if err != nil {
			return Queue{}, err
		}
		g, ok := groups[domain]
		if !ok {
			g = &Group{Domain: domain, From: from}
			groups[domain] = g
			keys = append(keys, domain)
		}
		g.Notes = append(g.Notes, Entry{
			Vault: n.Ref.Vault.Name, Path: n.Ref.Path, Title: n.Frontmatter.Title,
			Status: n.Status(), Reasons: reasons,
		})
	}
	q := Queue{Drift: opts.Drift != nil, Groups: make([]Group, 0, len(keys))}
	for _, k := range keys {
		g := groups[k]
		b.sortEntries(g.Notes)
		g.Total = len(g.Notes)
		q.Total += g.Total
		if len(g.Notes) > opts.Limit {
			g.Notes = g.Notes[:opts.Limit]
		}
		q.Groups = append(q.Groups, *g)
	}
	sortGroups(q.Groups)
	return q, nil
}

type builder struct {
	opts   Options
	owners map[*vault.Vault]Owners
	order  map[string]int
}

// reasons returns why n is in the queue, empty when it is not.
func (b *builder) reasons(n *vault.NoteView) []Reason {
	status := n.Status()
	if lifecycle.Revoked(status) || n.ParseErr != nil {
		return nil
	}
	f := lifecycle.Read(n.Frontmatter)
	var out []Reason
	if past(f.ReviewBy, b.opts.Now) {
		out = append(
			out,
			Reason{ReasonExpired, "review_by " + f.ReviewBy + " has passed", f.ReviewBy},
		)
	}
	if status == lifecycle.Raw && past(f.ExpiresAt, b.opts.Now) {
		out = append(
			out,
			Reason{ReasonExpired, "expires_at " + f.ExpiresAt + " has passed", f.ExpiresAt},
		)
	}
	if status == lifecycle.Disputed {
		detail := f.StatusReason
		if detail == "" {
			detail = "no reason recorded"
		}
		out = append(out, Reason{ReasonDisputed, detail, f.StatusAt})
	}
	out = append(out, b.dependent(f.Sources)...)
	if b.opts.Drift != nil {
		out = append(out, driftReasons(b.opts.Drift(n))...)
	}
	return out
}

// dependent returns a reason per sources entry resting on a revoked note.
func (b *builder) dependent(sources []lifecycle.Source) []Reason {
	var out []Reason
	for _, s := range sources {
		ref := s.NoteRef()
		if ref == "" {
			continue
		}
		dep, ok := b.opts.Snapshot.Resolve(ref)
		if !ok || !lifecycle.Revoked(dep.Status()) {
			continue
		}
		df := lifecycle.Read(dep.Frontmatter)
		out = append(out, Reason{
			Kind:   ReasonDependent,
			Detail: fmt.Sprintf("rests on %s (%s), which is %s", ref, dep.Ref, dep.Status()),
			Since:  df.StatusAt,
		})
	}
	return out
}

// driftReasons folds the drift findings of one note into one reason.
func driftReasons(findings []drift.Finding) []Reason {
	var parts []string
	for _, f := range findings {
		if f.Drifted() {
			parts = append(parts, f.Kind+" "+f.Source)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	return []Reason{{Kind: ReasonDrift, Detail: strings.Join(parts, "; ")}}
}

// domain returns the domain of n and where it came from.
func (b *builder) domain(n *vault.NoteView) (string, string, error) {
	o, ok := b.owners[n.Ref.Vault]
	if !ok {
		var err error
		if o, err = LoadOwners(n.Ref.Vault.Root); err != nil {
			return "", "", fmt.Errorf("%s: read CODEOWNERS: %w", n.Ref.Vault.Name, err)
		}
		b.owners[n.Ref.Vault] = o
	}
	if owners := o.Of(n.Ref.Path); len(owners) > 0 {
		return strings.Join(owners, " "), FromCodeowners, nil
	}
	if steward := lifecycle.Read(n.Frontmatter).Steward; steward != "" {
		return steward, FromSteward, nil
	}
	return Unowned, FromNone, nil
}

// sortEntries orders a group's notes by their oldest reason, a note whose
// reasons carry no date last, then by vault and path.
func (b *builder) sortEntries(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		a, c := oldest(entries[i].Reasons), oldest(entries[j].Reasons)
		if (a == "") != (c == "") {
			return c == ""
		}
		if a != c {
			return a < c
		}
		if entries[i].Vault != entries[j].Vault {
			return b.order[entries[i].Vault] < b.order[entries[j].Vault]
		}
		return entries[i].Path < entries[j].Path
	})
}

// sortGroups orders groups by size, then domain, with unowned last.
func sortGroups(groups []Group) {
	sort.SliceStable(groups, func(i, j int) bool {
		a, c := groups[i], groups[j]
		if (a.Domain == Unowned) != (c.Domain == Unowned) {
			return c.Domain == Unowned
		}
		if a.Total != c.Total {
			return a.Total > c.Total
		}
		return a.Domain < c.Domain
	})
}

// oldest returns the earliest Since of reasons, normalized to a date.
func oldest(reasons []Reason) string {
	first := ""
	for _, r := range reasons {
		t, ok := lifecycle.ParseDate(r.Since)
		if !ok {
			continue
		}
		if d := lifecycle.Date(t); first == "" || d < first {
			first = d
		}
	}
	return first
}

// past reports whether the date value lies before now's day.
func past(value string, now time.Time) bool {
	t, ok := lifecycle.ParseDate(value)
	return ok && lifecycle.Date(t) < lifecycle.Date(now)
}
