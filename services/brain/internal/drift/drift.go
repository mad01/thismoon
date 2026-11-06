// Package drift is brain drift: it checks the sources a note pins against
// what they are now. A code ref (repo:path@commit#symbol) is checked in the
// local checkout of its repo, found under the configured code roots: the
// path must exist at HEAD, its content is compared with the pinned commit,
// and the symbol must still be in it. An external source is fetched, only
// when the caller asks for network checks, and compared with its
// content_hash. Each problem is a Finding of one Kind.
package drift

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// Kinds of finding.
const (
	// KindMoved is a pinned path that no longer exists at HEAD.
	KindMoved = "moved"
	// KindChanged is a pinned path whose content at HEAD differs from the
	// pinned commit.
	KindChanged = "changed"
	// KindSymbolMissing is a pinned symbol no longer in the file at HEAD.
	KindSymbolMissing = "symbol-missing"
	// KindGone is an external source that answers 404 or 410.
	KindGone = "gone"
	// KindHashChanged is an external source whose content no longer
	// matches its content_hash.
	KindHashChanged = "hash-changed"
	// KindBadHash is a content_hash that is not a full digest of an
	// algorithm drift computes, so no fetch could ever match or miss it.
	KindBadHash = "bad-hash"
	// KindUnresolved is a source drift could not check: its repo is not
	// checked out, its commit is not there, the fetch failed. It is not
	// drift.
	KindUnresolved = "unresolved"
	// KindTooLarge is a pinned file over MaxBlobBytes at HEAD, whose symbol
	// drift did not read the file to look for. It is not drift.
	KindTooLarge = "too-large"
)

// Finding is one problem with one source of a note.
type Finding struct {
	Kind string `json:"kind"`
	// Source is the code ref or the URL the finding is about.
	Source  string `json:"source"`
	Message string `json:"message"`
	// MovedTo is where git saw a moved path go, when it detected a rename.
	MovedTo string `json:"moved_to,omitempty"`
	// Archive is the source's archive URL, set on a gone finding.
	Archive string `json:"archive,omitempty"`
}

// Drifted reports whether the finding is drift: anything but a source
// drift could not check (unresolved, too-large).
func (f Finding) Drifted() bool { return f.Kind != KindUnresolved && f.Kind != KindTooLarge }

// NoteReport is the findings of one note.
type NoteReport struct {
	Vault    string    `json:"vault"`
	Path     string    `json:"path"`
	Findings []Finding `json:"findings"`
}

// Report is the result of one drift run: ok when no note drifted. Notes
// lists only the notes with findings.
type Report struct {
	OK      bool `json:"ok"`
	Online  bool `json:"online"`
	Checked int  `json:"checked"`
	Drifted int  `json:"drifted"`
	// Offline counts the external sources left unchecked without online.
	Offline int          `json:"offline_skipped,omitempty"`
	Notes   []NoteReport `json:"notes"`
}

// Options configures a run.
type Options struct {
	// CodeRoots are the directories checkouts are found under.
	CodeRoots []string
	// Online allows the network checks of external sources.
	Online bool
	// AllowLocal lets online fetches reach loopback, link-local and private
	// addresses, which are refused by default so that a shared vault's
	// sources cannot point drift at services on this machine or network.
	AllowLocal bool
	// Budget bounds all the fetches of one Checker together; zero means
	// DefaultBudget. A source not fetched in time is unresolved.
	Budget time.Duration
	// Client fetches external sources; nil uses one with FetchTimeout that
	// enforces AllowLocal. A caller's own client is used as given.
	Client *http.Client
}

// FetchTimeout bounds one fetch of an external source.
const FetchTimeout = 20 * time.Second

// DefaultBudget bounds all the fetches of one online run together.
const DefaultBudget = 2 * time.Minute

// Checker checks sources, each distinct one once.
type Checker struct {
	opts     Options
	git      *gitChecker
	fetched  map[string][]Finding
	offline  int
	deadline time.Time // when the online budget runs out
}

// New returns a Checker for opts. Its online budget starts now.
func New(opts Options) *Checker {
	if opts.Client == nil {
		opts.Client = newClient(opts.AllowLocal)
	}
	if opts.Budget <= 0 {
		opts.Budget = DefaultBudget
	}
	return &Checker{
		opts:     opts,
		git:      newGitChecker(opts.CodeRoots),
		fetched:  map[string][]Finding{},
		deadline: time.Now().Add(opts.Budget),
	}
}

// Sources returns the findings for one note's sources. Entries that are
// neither a code ref nor an http(s) URL (a note ref, a bare citation) are
// not drift's to check.
func (c *Checker) Sources(ctx context.Context, sources []lifecycle.Source) []Finding {
	var out []Finding
	for _, s := range sources {
		if ref, ok := lifecycle.ParseCodeRef(s.Ref); ok {
			out = append(out, c.git.check(ctx, ref)...)
		}
		if isWebURL(s.URL) {
			out = append(out, c.external(ctx, s)...)
		}
	}
	return out
}

// external checks one URL source online, or counts it as skipped. A
// malformed content_hash is a finding either way, since no fetch is needed
// to see it.
func (c *Checker) external(ctx context.Context, s lifecycle.Source) []Finding {
	if s.ContentHash != "" {
		if _, err := parseContentHash(s.ContentHash); err != nil {
			return []Finding{{Kind: KindBadHash, Source: s.URL, Message: err.Error()}}
		}
	}
	if !c.opts.Online {
		c.offline++
		return nil
	}
	key := s.URL + "\x00" + s.ContentHash + "\x00" + s.Archive
	if got, ok := c.fetched[key]; ok {
		return got
	}
	got := c.fetchInBudget(ctx, s)
	c.fetched[key] = got
	return got
}

// fetchInBudget fetches one source within what is left of the online
// budget. A source the budget does not reach, or one cut off by it, is
// unresolved with a message that says so.
func (c *Checker) fetchInBudget(ctx context.Context, s lifecycle.Source) []Finding {
	spent := []Finding{{
		Kind:    KindUnresolved,
		Source:  s.URL,
		Message: fmt.Sprintf("not fetched: the online budget of %s ran out", c.opts.Budget),
	}}
	if !time.Now().Before(c.deadline) {
		return spent
	}
	fctx, cancel := context.WithDeadline(ctx, c.deadline)
	defer cancel()
	got := fetch(fctx, c.opts.Client, s)
	if errors.Is(fctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil && unresolved(got) {
		return spent
	}
	return got
}

func unresolved(findings []Finding) bool {
	return len(findings) == 1 && findings[0].Kind == KindUnresolved
}

// Run checks the sources of every note and reports the notes with findings,
// in the order given.
func Run(ctx context.Context, notes []*vault.NoteView, opts Options) Report {
	c := New(opts)
	r := Report{Online: opts.Online, Checked: len(notes), Notes: []NoteReport{}}
	for _, n := range notes {
		findings := c.Sources(ctx, lifecycle.Read(n.Frontmatter).Sources)
		if len(findings) == 0 {
			continue
		}
		r.Notes = append(r.Notes, NoteReport{
			Vault: n.Ref.Vault.Name, Path: n.Ref.Path, Findings: findings,
		})
		if anyDrifted(findings) {
			r.Drifted++
		}
	}
	r.Offline = c.offline
	r.OK = r.Drifted == 0
	return r
}

func anyDrifted(findings []Finding) bool {
	for _, f := range findings {
		if f.Drifted() {
			return true
		}
	}
	return false
}
