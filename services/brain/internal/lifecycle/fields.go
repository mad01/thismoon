package lifecycle

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/services/brain/internal/markdown"
)

// Fields is the lifecycle view of one note's frontmatter. Read fills it
// leniently: a field of the wrong shape reads as empty, and Validate says
// what is wrong with it.
type Fields struct {
	Status          string
	ExpiresAt       string
	ReviewBy        string
	StatusAt        string
	StatusReason    string
	SupersededBy    string
	RetractedReason string
	RetractedAt     string
	Volatility      string
	CapturedBy      string
	Steward         string
	Classification  string
	ValidFrom       string
	InvalidAt       string
	DistinctFrom    []string
	Sources         []Source
}

// Source is one entry of the sources list. A bare string entry is its URL.
type Source struct {
	URL         string
	Note        string
	ContentHash string
	Archive     string
	// Ref pins code a claim rests on: repo:path@commit, optionally
	// followed by #symbol.
	Ref string
}

// Problem is one lifecycle field with the wrong shape or value.
type Problem struct {
	Key     string
	Message string
}

func (p Problem) String() string { return p.Key + ": " + p.Message }

// stringKeys maps each scalar lifecycle key to its Fields slot.
var stringKeys = []struct {
	key  string
	slot func(*Fields) *string
}{
	{KeyExpiresAt, func(f *Fields) *string { return &f.ExpiresAt }},
	{KeyReviewBy, func(f *Fields) *string { return &f.ReviewBy }},
	{KeyStatusAt, func(f *Fields) *string { return &f.StatusAt }},
	{KeyStatusReason, func(f *Fields) *string { return &f.StatusReason }},
	{KeySupersededBy, func(f *Fields) *string { return &f.SupersededBy }},
	{KeyRetractedReason, func(f *Fields) *string { return &f.RetractedReason }},
	{KeyRetractedAt, func(f *Fields) *string { return &f.RetractedAt }},
	{KeyVolatility, func(f *Fields) *string { return &f.Volatility }},
	{KeyCapturedBy, func(f *Fields) *string { return &f.CapturedBy }},
	{KeySteward, func(f *Fields) *string { return &f.Steward }},
	{KeyClassification, func(f *Fields) *string { return &f.Classification }},
	{KeyValidFrom, func(f *Fields) *string { return &f.ValidFrom }},
	{KeyInvalidAt, func(f *Fields) *string { return &f.InvalidAt }},
}

// dateKeys are the lifecycle keys that hold a date.
var dateKeys = []string{
	KeyExpiresAt, KeyReviewBy, KeyStatusAt, KeyRetractedAt, KeyValidFrom, KeyInvalidAt,
}

// enumKeys are the lifecycle keys that take one of a fixed set of values.
var enumKeys = map[string][]string{
	KeyVolatility:      Volatilities,
	KeyRetractedReason: RetractReasons,
	KeyClassification:  Classifications,
}

// codeRef matches a sources[].ref: repo:path@commit with an optional
// #symbol. The commit is a hex object name of 7 to 64 characters.
var codeRef = regexp.MustCompile(`^[^\s:@#]+:[^\s@#]+@[0-9a-fA-F]{7,64}(#\S+)?$`)

// contentHash matches a sources[].content_hash: hex, optionally prefixed
// with its algorithm, such as sha256:.
var contentHash = regexp.MustCompile(`^([a-z0-9-]+:)?[0-9a-fA-F]{16,128}$`)

// Read returns the lifecycle view of fm, leaving fields of the wrong shape
// empty.
func Read(fm markdown.Frontmatter) Fields {
	f, _ := parse(fm)
	return f
}

// Validate returns what is wrong with the lifecycle fields of fm: a wrong
// shape, a value outside its set, a date that does not parse, a malformed
// source, a stored expired status. A missing field is not a problem here;
// Required reports the fields a status needs.
func Validate(fm markdown.Frontmatter) []Problem {
	f, problems := parse(fm)
	if Normalize(f.Status) == Expired {
		problems = append(problems, Problem{
			KeyStatus, "expired is computed from review_by and never stored; set review_by instead",
		})
	}
	for _, key := range dateKeys {
		if v := stringValue(f, key); v != "" {
			if _, ok := ParseDate(v); !ok {
				problems = append(
					problems,
					Problem{key, fmt.Sprintf("%q is not a YYYY-MM-DD date", v)},
				)
			}
		}
	}
	for key, allowed := range enumKeys {
		if v := stringValue(f, key); v != "" && !slices.Contains(allowed, v) {
			problems = append(problems, Problem{
				key, fmt.Sprintf("%q is not one of %s", v, strings.Join(allowed, ", ")),
			})
		}
	}
	if c := f.CapturedBy; c != "" && c != "user" &&
		(!strings.HasPrefix(c, "agent:") || len(c) == len("agent:")) {
		problems = append(problems, Problem{
			KeyCapturedBy, fmt.Sprintf("%q is neither user nor agent:<session>", c),
		})
	}
	if from, ok := ParseDate(f.ValidFrom); ok {
		if until, ok := ParseDate(f.InvalidAt); ok && until.Before(from) {
			problems = append(problems, Problem{KeyInvalidAt, "is before valid_from"})
		}
	}
	for i, s := range f.Sources {
		if s.Ref != "" && !codeRef.MatchString(s.Ref) {
			problems = append(problems, Problem{
				fmt.Sprintf("%s[%d].ref", KeySources, i),
				fmt.Sprintf("%q is not repo:path@commit or repo:path@commit#symbol", s.Ref),
			})
		}
		if s.ContentHash != "" && !contentHash.MatchString(s.ContentHash) {
			problems = append(problems, Problem{
				fmt.Sprintf("%s[%d].content_hash", KeySources, i),
				fmt.Sprintf("%q is not a hex digest such as sha256:<hex>", s.ContentHash),
			})
		}
	}
	slices.SortStableFunc(problems, func(a, b Problem) int { return strings.Compare(a.Key, b.Key) })
	return problems
}

// Required returns the fields fm's status needs and lacks. A superseded
// note names its replacement, a retracted one its reason and date, and a
// withdrawn one keeps nothing but id, status and status_at, with no body.
func Required(fm markdown.Frontmatter, body []byte) []Problem {
	f := Read(fm)
	var problems []Problem
	need := func(key, value string) {
		if strings.TrimSpace(value) == "" {
			problems = append(
				problems,
				Problem{key, "is required when status is " + Normalize(fm.Status)},
			)
		}
	}
	switch Normalize(fm.Status) {
	case Superseded:
		need(KeySupersededBy, f.SupersededBy)
	case Retracted:
		need(KeyRetractedReason, f.RetractedReason)
		need(KeyRetractedAt, f.RetractedAt)
	case Withdrawn:
		for _, key := range fm.Keys() {
			if key != KeyID && key != KeyStatus && key != KeyStatusAt {
				problems = append(
					problems,
					Problem{key, "a withdrawn note keeps only id, status and status_at"},
				)
			}
		}
		if strings.TrimSpace(string(body)) != "" {
			problems = append(problems, Problem{"body", "a withdrawn note has no body"})
		}
	}
	return problems
}

// parse reads every lifecycle field of fm, collecting shape problems.
func parse(fm markdown.Frontmatter) (Fields, []Problem) {
	f := Fields{Status: fm.Status}
	var problems []Problem
	for _, k := range stringKeys {
		v, err := scalar(extra(fm, k.key))
		if err != nil {
			problems = append(problems, Problem{k.key, err.Error()})
			continue
		}
		*k.slot(&f) = v
	}
	var err error
	if f.DistinctFrom, err = stringList(extra(fm, KeyDistinctFrom)); err != nil {
		problems = append(problems, Problem{KeyDistinctFrom, err.Error()})
	}
	var sourceProblems []Problem
	f.Sources, sourceProblems = sources(extra(fm, KeySources))
	return f, append(problems, sourceProblems...)
}

func stringValue(f Fields, key string) string {
	for _, k := range stringKeys {
		if k.key == key {
			return *k.slot(&f)
		}
	}
	return ""
}

// extra returns the value node of a key brain does not type, or nil.
func extra(fm markdown.Frontmatter, key string) *yaml.Node {
	for _, e := range fm.Extra {
		if e.Key == key {
			return e.Value
		}
	}
	return nil
}

// scalar returns the text of a scalar node; nil reads as "".
func scalar(n *yaml.Node) (string, error) {
	if n == nil {
		return "", nil
	}
	if n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	if n.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("takes a single value, got %s", kindName(n))
	}
	if n.ShortTag() == "!!null" {
		return "", nil
	}
	return strings.TrimSpace(n.Value), nil
}

// stringList reads a list of scalars; a single scalar reads as one item.
func stringList(n *yaml.Node) ([]string, error) {
	if n == nil {
		return nil, nil
	}
	if n.Kind != yaml.SequenceNode {
		v, err := scalar(n)
		if err != nil || v == "" {
			return nil, err
		}
		return []string{v}, nil
	}
	out := make([]string, 0, len(n.Content))
	for _, item := range n.Content {
		v, err := scalar(item)
		if err != nil {
			return nil, fmt.Errorf("takes a list of values: %w", err)
		}
		if v != "" {
			out = append(out, v)
		}
	}
	return out, nil
}

// sources reads the sources list: each entry a URL or a mapping of
// url, note, content_hash, archive and ref.
func sources(n *yaml.Node) ([]Source, []Problem) {
	if n == nil {
		return nil, nil
	}
	if n.Kind != yaml.SequenceNode {
		return nil, []Problem{{KeySources, "takes a list, got " + kindName(n)}}
	}
	var (
		out      []Source
		problems []Problem
	)
	for i, item := range n.Content {
		key := fmt.Sprintf("%s[%d]", KeySources, i)
		switch item.Kind {
		case yaml.ScalarNode:
			out = append(out, Source{URL: strings.TrimSpace(item.Value)})
		case yaml.MappingNode:
			s, err := sourceEntry(item)
			if err != nil {
				problems = append(problems, Problem{key, err.Error()})
			}
			out = append(out, s)
		default:
			problems = append(
				problems,
				Problem{key, "takes a URL or a mapping, got " + kindName(item)},
			)
		}
	}
	return out, problems
}

func sourceEntry(n *yaml.Node) (Source, error) {
	var s Source
	slots := map[string]*string{
		"url": &s.URL, "note": &s.Note, "content_hash": &s.ContentHash,
		"archive": &s.Archive, "ref": &s.Ref,
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		slot, ok := slots[n.Content[i].Value]
		if !ok {
			continue
		}
		v, err := scalar(n.Content[i+1])
		if err != nil {
			return s, fmt.Errorf("%s %w", n.Content[i].Value, err)
		}
		*slot = v
	}
	return s, nil
}

func kindName(n *yaml.Node) string {
	switch n.Kind {
	case yaml.SequenceNode:
		return "a list"
	case yaml.MappingNode:
		return "a mapping"
	}
	return "a scalar"
}
