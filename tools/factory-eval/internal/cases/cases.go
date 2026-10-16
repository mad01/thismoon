// Package cases loads and validates the eval's cases file: the pinned
// commit, the model tiers and their dollar caps, the shared outline, the
// judge rubric, the arm preambles, and the docs-sweep cases. Every choice
// in the file carries a provenance label, and the file carries the user's
// sign-off, which a paid run requires.
package cases

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/tools/factory-eval/internal/outline"
)

// Provenance labels: where a value in the cases file came from.
const (
	// Measured values were computed from the repo at the pinned commit.
	Measured = "measured"
	// Spec values come from the ticket, the factory spec, or the shared
	// docs outline.
	Spec = "spec"
	// Judgement values are choices made for this harness.
	Judgement = "judgement"
)

var (
	shaPattern      = regexp.MustCompile(`^[0-9a-f]{40}$`)
	validEfforts    = []string{"low", "medium", "high", "xhigh", "max"}
	validSeverities = []string{"suggestion", "warning", "error"}
)

// RootGlossary is the glossary a case's judge call gets when the case
// names none of its own.
const RootGlossary = "CONTEXT.md"

// File is the whole cases file.
type File struct {
	Version         int       `yaml:"version"`
	Repo            string    `yaml:"repo"`
	Commit          string    `yaml:"commit"`
	SignOff         SignOff   `yaml:"sign_off"`
	Runs            int       `yaml:"runs"`
	TimeoutMinutes  int       `yaml:"timeout_minutes"`
	BatchCeilingUSD float64   `yaml:"batch_ceiling_usd"`
	Tiers           []Tier    `yaml:"tiers"`
	Outline         Outline   `yaml:"outline"`
	Humanizer       Humanizer `yaml:"humanizer"`
	Judge           Judge     `yaml:"judge"`
	Arms            Arms      `yaml:"arms"`
	Cases           []Case    `yaml:"cases"`

	// digest is ContentDigest of the bytes the file was parsed from.
	digest string
}

// Tier is one model tier both arms run at, with the dollar cap each arm
// run gets at that tier.
type Tier struct {
	Name       string  `yaml:"name"`
	Model      string  `yaml:"model"`
	Effort     string  `yaml:"effort"`
	Worker     string  `yaml:"worker"`
	BudgetUSD  float64 `yaml:"budget_usd"`
	Primary    bool    `yaml:"primary"`
	Provenance string  `yaml:"provenance"`
}

// Outline is the shared docs outline for the two files every case edits.
type Outline struct {
	Provenance string       `yaml:"provenance"`
	ClaudeMD   outline.Spec `yaml:"claude_md"`
	Readme     outline.Spec `yaml:"readme"`
}

// Humanizer configures the deterministic prose check. MinSeverity is one
// of humanizer's levels: suggestion, warning or error.
type Humanizer struct {
	MinSeverity string   `yaml:"min_severity"`
	IgnoreRules []string `yaml:"ignore_rules"`
	Provenance  string   `yaml:"provenance"`
}

// Judge configures the one LLM judge call per case. GlossaryNotes go to
// the judge beside the case's glossary files.
type Judge struct {
	Model         string   `yaml:"model"`
	BudgetUSD     float64  `yaml:"budget_usd"`
	Rubric        string   `yaml:"rubric"`
	DoNotFail     []string `yaml:"do_not_fail"`
	GlossaryNotes string   `yaml:"glossary_notes"`
	Provenance    string   `yaml:"provenance"`
}

// Arms holds the preamble each arm's session gets before the case tasks.
type Arms struct {
	Single  Arm `yaml:"single"`
	Factory Arm `yaml:"factory"`
}

// Arm is one arm's preamble, a text/template over the run's paths and tier.
type Arm struct {
	Preamble   string `yaml:"preamble"`
	Provenance string `yaml:"provenance"`
}

// Case is one component docs sweep: one branch, two files, one task.
// Glossary lists the repo-relative glossary files the judge grades the
// case against; empty means the root CONTEXT.md alone.
type Case struct {
	Component    string         `yaml:"component"`
	Branch       string         `yaml:"branch"`
	AllowedPaths []string       `yaml:"allowed_paths"`
	Glossary     []string       `yaml:"glossary"`
	Task         string         `yaml:"task"`
	Done         string         `yaml:"done"`
	Deviation    Deviation      `yaml:"deviation"`
	Provenance   CaseProvenance `yaml:"provenance"`
}

// Name is the component's directory name, which the outline title rules
// match against.
func (c Case) Name() string { return path.Base(c.Component) }

// GlossaryFiles is the case's glossary list, the root CONTEXT.md when the
// case names none.
func (c Case) GlossaryFiles() []string {
	if len(c.Glossary) == 0 {
		return []string{RootGlossary}
	}
	return c.Glossary
}

// Deviation is the measured distance of a case's two files from the
// outline and the humanizer pack at the pinned commit.
type Deviation struct {
	ClaudeMD FileDeviation `yaml:"claude_md"`
	Readme   FileDeviation `yaml:"readme"`
	Total    int           `yaml:"total"`
}

// FileDeviation is one file's outline problems and humanizer finding count.
type FileDeviation struct {
	Outline   []string `yaml:"outline"`
	Humanizer int      `yaml:"humanizer_warnings"`
}

// CaseProvenance labels each part of a case.
type CaseProvenance struct {
	Selection    string `yaml:"selection"`
	AllowedPaths string `yaml:"allowed_paths"`
	Task         string `yaml:"task"`
	Done         string `yaml:"done"`
}

// Load reads and validates the cases file at p.
func Load(p string) (*File, error) {
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, fmt.Errorf("cases: read: %w", err)
	}
	return Parse(raw)
}

// Parse decodes and validates a cases file and computes its content
// digest. Unknown keys are errors, so a typo in a field name fails loudly
// instead of dropping the value.
func Parse(raw []byte) (*File, error) {
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	var f File
	if err := dec.Decode(&f); err != nil {
		return nil, fmt.Errorf("cases: decode: %w", err)
	}
	if err := f.validate(); err != nil {
		return nil, err
	}
	digest, err := ContentDigest(raw)
	if err != nil {
		return nil, err
	}
	f.digest = digest
	return &f, nil
}

// PrimaryTier returns the tier whose decision is the headline pass.
func (f *File) PrimaryTier() Tier {
	for _, t := range f.Tiers {
		if t.Primary {
			return t
		}
	}
	return f.Tiers[0]
}

func (f *File) validate() error {
	var errs []error
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("cases: "+format, args...))
	}
	if f.Version != 1 {
		add("version %d, want 1", f.Version)
	}
	if f.Repo == "" {
		add("repo is empty")
	}
	if !shaPattern.MatchString(f.Commit) {
		add("commit %q is not a full 40-character sha", f.Commit)
	}
	if f.Runs < 1 {
		add("runs %d, want at least 1", f.Runs)
	}
	if f.TimeoutMinutes < 1 {
		add("timeout_minutes %d, want at least 1", f.TimeoutMinutes)
	}
	if f.BatchCeilingUSD <= 0 {
		add("batch_ceiling_usd must be positive")
	}
	errs = append(errs, f.validateTiers()...)
	errs = append(errs, f.validateSettings()...)
	errs = append(errs, f.validateCases()...)
	return errors.Join(errs...)
}

func (f *File) validateTiers() []error {
	var errs []error
	if len(f.Tiers) == 0 {
		errs = append(errs, errors.New("cases: no tiers"))
	}
	primaries := 0
	names := map[string]bool{}
	for _, t := range f.Tiers {
		if t.Name == "" || names[t.Name] {
			errs = append(errs, fmt.Errorf("cases: tier name %q is empty or repeated", t.Name))
		}
		names[t.Name] = true
		if t.Model == "" || t.Worker == "" || t.BudgetUSD <= 0 {
			errs = append(
				errs,
				fmt.Errorf("cases: tier %q needs model, worker, budget_usd", t.Name),
			)
		}
		if !slices.Contains(validEfforts, t.Effort) {
			errs = append(errs, fmt.Errorf("cases: tier %q effort %q unknown", t.Name, t.Effort))
		}
		if t.Primary {
			primaries++
		}
		errs = append(errs, checkLabel("tier "+t.Name, t.Provenance))
	}
	if primaries > 1 {
		errs = append(errs, errors.New("cases: more than one primary tier"))
	}
	return errs
}

func (f *File) validateSettings() []error {
	errs := []error{
		checkLabel("outline", f.Outline.Provenance),
		checkLabel("humanizer", f.Humanizer.Provenance),
		checkLabel("judge", f.Judge.Provenance),
		checkLabel("arms.single", f.Arms.Single.Provenance),
		checkLabel("arms.factory", f.Arms.Factory.Provenance),
	}
	if !slices.Contains(validSeverities, f.Humanizer.MinSeverity) {
		errs = append(errs, fmt.Errorf("cases: humanizer min_severity %q, want one of %s",
			f.Humanizer.MinSeverity, strings.Join(validSeverities, ", ")))
	}
	if len(f.Outline.ClaudeMD.Sections) == 0 || len(f.Outline.Readme.Sections) == 0 {
		errs = append(errs, errors.New("cases: outline needs claude_md and readme sections"))
	}
	if f.Judge.Model == "" || f.Judge.BudgetUSD <= 0 || strings.TrimSpace(f.Judge.Rubric) == "" {
		errs = append(errs, errors.New("cases: judge needs model, budget_usd, rubric"))
	}
	if strings.TrimSpace(f.Arms.Single.Preamble) == "" ||
		strings.TrimSpace(f.Arms.Factory.Preamble) == "" {
		errs = append(errs, errors.New("cases: both arms need a preamble"))
	}
	return errs
}

func (f *File) validateCases() []error {
	var errs []error
	if len(f.Cases) == 0 {
		errs = append(errs, errors.New("cases: no cases"))
	}
	branches := map[string]bool{}
	for _, c := range f.Cases {
		errs = append(errs, c.validate()...)
		if branches[c.Branch] {
			errs = append(errs, fmt.Errorf("cases: branch %q repeated", c.Branch))
		}
		branches[c.Branch] = true
	}
	return errs
}

func (c Case) validate() []error {
	var errs []error
	if c.Component == "" || c.Branch == "" {
		errs = append(errs, fmt.Errorf("cases: case %q needs component and branch", c.Component))
	}
	if strings.TrimSpace(c.Task) == "" || strings.TrimSpace(c.Done) == "" {
		errs = append(errs, fmt.Errorf("cases: case %q needs task and done", c.Component))
	}
	if len(c.AllowedPaths) == 0 {
		errs = append(errs, fmt.Errorf("cases: case %q has no allowed_paths", c.Component))
	}
	for _, p := range c.AllowedPaths {
		if !strings.HasPrefix(p, c.Component+"/") {
			errs = append(errs, fmt.Errorf("cases: case %q allows %q outside it", c.Component, p))
		}
	}
	for _, g := range c.Glossary {
		if path.IsAbs(g) || path.Clean(g) != g || strings.HasPrefix(g, "../") {
			errs = append(errs, fmt.Errorf("cases: case %q glossary %q is not a clean repo path",
				c.Component, g))
		}
	}
	labels := [][2]string{
		{"selection", c.Provenance.Selection},
		{"allowed_paths", c.Provenance.AllowedPaths},
		{"task", c.Provenance.Task},
		{"done", c.Provenance.Done},
	}
	for _, l := range labels {
		errs = append(errs, checkLabel(c.Component+" "+l[0], l[1]))
	}
	return errs
}

func checkLabel(what, label string) error {
	switch label {
	case Measured, Spec, Judgement:
		return nil
	default:
		return fmt.Errorf("cases: %s provenance %q, want measured, spec, or judgement", what, label)
	}
}
