// Package check is brain check: it checks notes against the lifecycle model
// and their vault's policy, the gate a shared vault's CI runs on the notes a
// pull request changes. It reads every configured vault once, so links and
// note refs resolve the way brain resolves them everywhere else, and
// reports each problem as a Finding at the level the policy gives its rule.
package check

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/brain/internal/lifecycle"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// Finding is one problem in one note.
type Finding struct {
	Vault string `json:"vault"`
	Path  string `json:"path"`
	// Line is the 1-based line the problem is on; 0 means the whole note.
	Line    int    `json:"line,omitempty"`
	Rule    string `json:"rule"`
	Level   string `json:"level"`
	Message string `json:"message"`
}

// Skipped is a path brain check was given and did not check.
type Skipped struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Report is the result of one check: ok when no finding is an error.
type Report struct {
	OK       bool      `json:"ok"`
	Checked  int       `json:"checked"`
	Errors   int       `json:"errors"`
	Warnings int       `json:"warnings"`
	Findings []Finding `json:"findings"`
	Skipped  []Skipped `json:"skipped,omitempty"`
}

// Options selects what Run checks.
type Options struct {
	// Set is every configured vault: links and note refs resolve across it.
	Set *vault.Set
	// Targets are the notes to check. Empty checks every note in the tiers
	// of the Scope vaults.
	Targets []vault.Ref
	// Scope is the vaults checked when Targets is empty; nil means Set.
	Scope *vault.Set
	// Now dates review_by; zero means time.Now.
	Now time.Time
}

// Run checks the notes opts names. A link from a checked note to a
// retracted or withdrawn note is an error (a new citation of a revoked
// note); a link to any other note that is not validated, or one past its
// review_by, is a warning. So is a link from an unchecked note to a checked
// one that is not validated: the citations a change of status leaves
// behind.
func Run(opts Options) (Report, error) {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	sn, err := opts.Set.Snapshot()
	if err != nil {
		return Report{}, err
	}
	c := &checker{sn: sn, now: opts.Now, policies: map[*vault.Vault]Policy{}}
	targets, err := c.targets(opts)
	if err != nil {
		return Report{}, err
	}
	c.indexStems()
	checked := make(map[*vault.NoteView]bool, len(targets))
	for _, n := range targets {
		checked[n] = true
	}
	for _, n := range targets {
		c.note(n)
	}
	for _, n := range targets {
		c.citedElsewhere(n, checked)
	}
	return c.report(len(targets)), nil
}

type checker struct {
	sn       *vault.Snapshot
	now      time.Time
	policies map[*vault.Vault]Policy
	stems    map[*vault.Vault]map[string][]*vault.NoteView
	findings []Finding
}

// targets resolves the notes to check.
func (c *checker) targets(opts Options) ([]*vault.NoteView, error) {
	if len(opts.Targets) > 0 {
		out := make([]*vault.NoteView, 0, len(opts.Targets))
		for _, ref := range opts.Targets {
			n, err := c.sn.Note(ref)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", ref, err)
			}
			out = append(out, n)
		}
		return out, nil
	}
	scope := opts.Scope
	if scope == nil {
		scope = opts.Set
	}
	in := map[*vault.Vault]bool{}
	for _, v := range scope.Vaults() {
		in[v] = true
	}
	var out []*vault.NoteView
	for _, n := range c.sn.Notes() {
		if in[n.Ref.Vault] && n.Tier() != "" {
			out = append(out, n)
		}
	}
	return out, nil
}

// indexStems groups every tier note of each vault by lowercased stem.
func (c *checker) indexStems() {
	c.stems = map[*vault.Vault]map[string][]*vault.NoteView{}
	for _, n := range c.sn.Notes() {
		if n.Tier() == "" {
			continue
		}
		byStem := c.stems[n.Ref.Vault]
		if byStem == nil {
			byStem = map[string][]*vault.NoteView{}
			c.stems[n.Ref.Vault] = byStem
		}
		key := strings.ToLower(n.Stem)
		byStem[key] = append(byStem[key], n)
	}
}

// policy returns the vault's policy, loading it once. A policy that cannot
// be used is a finding of its own, and the vault is checked with the
// lenient default.
func (c *checker) policy(v *vault.Vault) Policy {
	if p, ok := c.policies[v]; ok {
		return p
	}
	p, err := LoadPolicy(v.Root)
	if err != nil {
		c.findings = append(c.findings, Finding{
			Vault: v.Name, Path: PolicyFile, Rule: RulePolicy, Level: Error,
			Message: fmt.Sprintf("%v; the vault is checked with the lenient default", err),
		})
		p = Policy{}
	}
	c.policies[v] = p
	return p
}

// add records a finding in note n at the level n's vault gives rule.
func (c *checker) add(n *vault.NoteView, line int, rule, format string, args ...any) {
	level := c.policy(n.Ref.Vault).Level(rule)
	if level == Off {
		return
	}
	c.findings = append(c.findings, Finding{
		Vault: n.Ref.Vault.Name, Path: n.Ref.Path, Line: line, Rule: rule, Level: level,
		Message: fmt.Sprintf(format, args...),
	})
}

// note runs every rule on one checked note.
func (c *checker) note(n *vault.NoteView) {
	c.secrets(n)
	c.links(n)
	switch {
	case errors.Is(n.ParseErr, vault.ErrInvalidPath):
		c.add(n, 0, RuleSymlink, "%v", n.ParseErr)
		return
	case n.ParseErr != nil:
		c.add(n, 1, RuleSchema, "frontmatter does not parse: %v", n.ParseErr)
		return
	}
	c.schema(n)
	c.fields(n)
	c.duplicates(n)
}

func (c *checker) schema(n *vault.NoteView) {
	fm := n.Frontmatter
	for _, p := range lifecycle.Validate(fm) {
		c.add(n, 0, RuleSchema, "%s", p)
	}
	status := lifecycle.Normalize(fm.Status)
	if status != "" && status != lifecycle.Expired && !lifecycle.Known(status) {
		c.add(n, 0, RuleStatusUnknown, "status %q is not one of %s",
			fm.Status, strings.Join(lifecycle.Statuses, ", "))
	}
	for _, p := range lifecycle.Required(fm, n.Body) {
		c.add(n, 0, RuleStatusFields, "%s", p)
	}
	f := lifecycle.Read(fm)
	if status == lifecycle.Superseded && f.SupersededBy != "" {
		if _, ok := c.sn.Resolve(f.SupersededBy); !ok {
			c.add(n, 0, RuleStatusFields, "superseded_by %q names no single note", f.SupersededBy)
		}
	}
	known := slices.Contains(lifecycle.Classifications, f.Classification)
	if known && !c.policy(n.Ref.Vault).Allows(f.Classification) {
		c.add(
			n,
			0,
			RuleClassification,
			"classification %q is not allowed in vault %s (allowed: %s)",
			f.Classification,
			n.Ref.Vault.Name,
			c.policy(n.Ref.Vault).allowed(),
		)
	}
}

// fields reports the optional fields a note of its status should carry,
// and a source note without a source.
func (c *checker) fields(n *vault.NoteView) {
	fm, f := n.Frontmatter, lifecycle.Read(n.Frontmatter)
	status := n.Status()
	if status == lifecycle.Withdrawn {
		return
	}
	missing := func(key, why string) { c.add(n, 0, RuleMissingField, "%s is missing%s", key, why) }
	if fm.ID == "" {
		missing(lifecycle.KeyID, " (add one with: brain migrate --ids)")
	}
	if f.Classification == "" {
		missing(lifecycle.KeyClassification, "")
	}
	switch status {
	case lifecycle.Validated:
		if f.ReviewBy == "" {
			missing(lifecycle.KeyReviewBy, " on a validated note")
		}
	case lifecycle.Raw:
		if f.ExpiresAt == "" {
			missing(lifecycle.KeyExpiresAt, " on a raw note")
		}
	}
	if strings.EqualFold(fm.Type, "source") && fm.Source == "" && len(f.Sources) == 0 {
		c.add(n, 0, RuleProvenance, "a note of type source names no source or sources")
	}
}

// duplicates reports an id or stem another note of the same vault shares.
func (c *checker) duplicates(n *vault.NoteView) {
	if id := n.Frontmatter.ID; id != "" {
		var others []string
		for _, o := range c.sn.WithID(id) {
			if o != n && o.Ref.Vault == n.Ref.Vault {
				others = append(others, o.Ref.Path)
			}
		}
		if len(others) > 0 {
			c.add(n, 0, RuleIDDuplicate, "id %s is also on %s", id, strings.Join(others, ", "))
		}
	}
	if n.Tier() == "" {
		return
	}
	var others []string
	for _, o := range c.stems[n.Ref.Vault][strings.ToLower(n.Stem)] {
		if o != n {
			others = append(others, o.Ref.Path)
		}
	}
	if len(others) > 0 {
		c.add(n, 0, RuleStemDuplicate, "stem %s is also %s; a [[%s]] link resolves to one of them",
			n.Stem, strings.Join(others, ", "), n.Stem)
	}
}

// links checks every wikilink of a checked note, and every note its
// distinct_from and sources refs name.
func (c *checker) links(n *vault.NoteView) {
	for _, cite := range c.sn.Citations(n) {
		switch cite.To {
		case nil:
			c.add(n, cite.Line, RuleLinkDangling, "[[%s]] resolves to no note", cite.Target)
		case n:
			// A note citing itself says nothing about its standing.
		default:
			c.citation(n, cite, false)
		}
	}
	f := lifecycle.Read(n.Frontmatter)
	for _, ref := range f.DistinctFrom {
		if _, ok := c.sn.Resolve(ref); !ok {
			c.add(n, 0, RuleLinkDangling, "distinct_from %q names no single note", ref)
		}
	}
	for _, s := range f.Sources {
		if ref := s.NoteRef(); ref != "" {
			if _, ok := c.sn.Resolve(ref); !ok {
				c.add(n, 0, RuleLinkDangling, "sources ref %q names no single note", ref)
			}
		}
	}
}

// citation reports a link to a note that is not validated. elsewhere marks
// a link from an unchecked note, which is never more than the
// link-unvalidated level: it was there before the change.
func (c *checker) citation(from *vault.NoteView, cite vault.Citation, elsewhere bool) {
	standing := c.standing(cite.To)
	if standing == "" {
		return
	}
	rule := RuleLinkUnvalidated
	if !elsewhere && (standing == lifecycle.Retracted || standing == lifecycle.Withdrawn) {
		rule = RuleLinkRevoked
	}
	c.add(
		from,
		cite.Line,
		rule,
		"links [[%s]] (%s), which is %s",
		cite.Target,
		cite.To.Ref,
		standing,
	)
}

// citedElsewhere reports the links from unchecked notes to a checked note
// that is not validated.
func (c *checker) citedElsewhere(n *vault.NoteView, checked map[*vault.NoteView]bool) {
	if c.standing(n) == "" {
		return
	}
	for _, cite := range c.sn.CitedBy(n) {
		if !checked[cite.From] {
			c.citation(cite.From, cite, true)
		}
	}
}

func (c *checker) standing(n *vault.NoteView) string {
	return lifecycle.Standing(n.Status(), lifecycle.Read(n.Frontmatter), c.now)
}

// secrets scans the note's bytes as they are on disk.
func (c *checker) secrets(n *vault.NoteView) {
	found, err := n.Ref.Vault.Secrets(n.Ref.Path, n.Data)
	if err != nil {
		c.add(n, 0, RuleSecret, "secret scan failed: %v", err)
		return
	}
	for _, f := range found {
		if f.Rule.ID == vault.NulByteRule {
			c.add(n, f.Line, RuleNulByte, "%s", f.Match)
			continue
		}
		c.add(n, f.Line, RuleSecret, "%s", vault.DescribeSecret(f))
	}
}

// report sorts the findings by vault, path, line and rule, and counts them.
func (c *checker) report(checked int) Report {
	order := map[string]int{}
	for i, v := range c.vaultsInOrder() {
		order[v] = i
	}
	sort.SliceStable(c.findings, func(i, j int) bool {
		a, b := c.findings[i], c.findings[j]
		if a.Vault != b.Vault {
			return order[a.Vault] < order[b.Vault]
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Rule < b.Rule
	})
	r := Report{Checked: checked, Findings: c.findings}
	if r.Findings == nil {
		r.Findings = []Finding{}
	}
	for _, f := range r.Findings {
		if f.Level == Error {
			r.Errors++
		} else {
			r.Warnings++
		}
	}
	r.OK = r.Errors == 0
	return r
}

func (c *checker) vaultsInOrder() []string {
	var names []string
	seen := map[string]bool{}
	for _, n := range c.sn.Notes() {
		if name := n.Ref.Vault.Name; !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}
