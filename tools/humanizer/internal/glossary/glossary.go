// Package glossary turns the domain glossaries in CONTEXT.md files into a
// vale substitution rule. A glossary entry is a bold **Term**: line, a
// definition, and an _Avoid_: line listing the banned synonyms; each banned
// synonym becomes a case-insensitive swap to its term. A CONTEXT.md governs
// its own directory and everything below it, so the rule for a file holds
// only the glossaries on that file's path. Parse, InScope, RuleFor, and
// Render are pure; Load and the Repo methods (repo.go) are the file-system
// edge.
package glossary

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Entry is one glossary term with the synonyms its _Avoid_ line bans.
type Entry struct {
	Term  string
	Avoid []string
}

// Skip is one piece of an _Avoid_ line that Parse leaves out of the rule.
type Skip struct {
	Line    int
	Synonym string
	Reason  string
}

// Context is one parsed CONTEXT.md. Path is the file's slash path relative
// to the repo root; the context governs path.Dir(Path) and everything below
// it, so an empty Path (what Parse returns) governs the whole repo.
type Context struct {
	Path    string
	Entries []Entry
	Skips   []Skip
}

// Substitution maps one banned synonym to the term to use instead. Use joins
// the terms with " or " when the same synonym is banned under several terms.
type Substitution struct {
	Avoid string
	Use   string
}

var (
	termLine      = regexp.MustCompile(`^\*\*([^*]+?):?\*\*:?\s*$`)
	avoidLine     = regexp.MustCompile(`^_Avoid_:\s*(.*)$`)
	parenthetical = regexp.MustCompile(`\([^()]*\)`)
)

// synonymTrim is the markup stripped from both ends of a banned synonym.
const synonymTrim = " \t`*_\"."

const (
	// reasonUnclosed explains a skip at an unmatched "(".
	reasonUnclosed = "unclosed parenthesis, so the rest of the line is read as a note"
	// reasonEdge explains a skipped synonym with a non-word first or last character.
	reasonEdge = "starts or ends with a non-word character, so the rule's word boundaries never match it"
)

// Parse reads one CONTEXT.md and returns its entries in file order. An
// _Avoid_ line belongs to the closest **Term** line above it; an _Avoid_ line
// with no term above it is ignored. Pieces of an _Avoid_ line that cannot
// become a working swap are returned as skips, not entries.
func Parse(r io.Reader) (Context, error) {
	sc := bufio.NewScanner(r)
	var (
		ctx  Context
		term string
		line int
	)
	for sc.Scan() {
		line++
		text := strings.TrimSpace(sc.Text())
		if m := termLine.FindStringSubmatch(text); m != nil {
			term = strings.TrimSpace(m[1])
			continue
		}
		m := avoidLine.FindStringSubmatch(text)
		if m == nil || term == "" {
			continue
		}
		avoid, skips := splitSynonyms(m[1])
		for _, s := range skips {
			s.Line = line
			ctx.Skips = append(ctx.Skips, s)
		}
		if len(avoid) > 0 {
			ctx.Entries = append(ctx.Entries, Entry{Term: term, Avoid: avoid})
		}
		term = ""
	}
	if err := sc.Err(); err != nil {
		return Context{}, fmt.Errorf("glossary: read: %w", err)
	}
	return ctx, nil
}

// splitSynonyms strips parenthetical notes (they can hold commas) and splits
// the rest of an _Avoid_ list on commas. An unmatched "(" starts a note that
// runs to the end of the line. A synonym whose first or last character is
// not a word character is skipped: vale wraps every swap key in word
// boundaries, so such a key never matches. The returned skips carry no line.
func splitSynonyms(list string) ([]string, []Skip) {
	for parenthetical.MatchString(list) {
		list = parenthetical.ReplaceAllString(list, "")
	}
	var skips []Skip
	if open := strings.Index(list, "("); open >= 0 {
		skips = append(skips, Skip{Synonym: strings.TrimSpace(list[open:]), Reason: reasonUnclosed})
		list = list[:open]
	}
	var out []string
	for _, part := range strings.Split(list, ",") {
		s := strings.Join(strings.Fields(strings.Trim(part, synonymTrim)), " ")
		switch {
		case s == "": // an empty piece, such as a trailing comma
		case !wordEdged(s):
			skips = append(skips, Skip{Synonym: s, Reason: reasonEdge})
		default:
			out = append(out, s)
		}
	}
	return out, skips
}

// wordEdged reports whether s starts and ends with a letter, digit, or
// underscore, the characters a regexp word boundary sits next to.
func wordEdged(s string) bool {
	first, _ := utf8.DecodeRuneInString(s)
	last, _ := utf8.DecodeLastRuneInString(s)
	return isWordRune(first) && isWordRune(last)
}

func isWordRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// InScope returns the entries of the contexts that govern target, in context
// order. target is a slash path relative to the repo root ("." is the root
// itself); an empty target means the text has no location, so every context
// applies.
func InScope(contexts []Context, target string) []Entry {
	var out []Entry
	for _, c := range contexts {
		if target == "" || governs(path.Dir(c.Path), target) {
			out = append(out, c.Entries...)
		}
	}
	return out
}

// governs reports whether a CONTEXT.md in dir applies to target: dir is the
// repo root, target itself, or an ancestor of target.
func governs(dir, target string) bool {
	return dir == "." || target == dir || strings.HasPrefix(target, dir+"/")
}

// RuleFor renders the substitution rule for the entries in scope at target
// (see InScope), or nil when no swap applies there.
func RuleFor(contexts []Context, target string) []byte {
	entries := InScope(contexts, target)
	subs := Substitutions(entries)
	if len(subs) == 0 {
		return nil
	}
	return Render(subs, MultiWordTerms(entries))
}

// Substitutions flattens entries into swaps sorted by banned synonym, case
// folded. A synonym that is itself a term is dropped, and a synonym banned
// under several terms keeps one swap whose Use lists the terms in order.
func Substitutions(entries []Entry) []Substitution {
	terms := map[string]bool{}
	for _, e := range entries {
		terms[strings.ToLower(e.Term)] = true
	}
	uses := map[string][]string{}
	spelling := map[string]string{}
	for _, e := range entries {
		for _, avoid := range e.Avoid {
			key := strings.ToLower(avoid)
			if terms[key] {
				continue
			}
			if _, seen := spelling[key]; !seen {
				spelling[key] = avoid
			}
			uses[key] = appendUnique(uses[key], e.Term)
		}
	}
	out := make([]Substitution, 0, len(uses))
	for key, terms := range uses {
		out = append(out, Substitution{Avoid: spelling[key], Use: strings.Join(terms, " or ")})
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Avoid) < strings.ToLower(out[j].Avoid)
	})
	return out
}

// MultiWordTerms returns the terms that contain a space, sorted case-folded
// and deduplicated. Render lists them as exceptions, so a banned word inside a
// correct term ("page" in "Ephemeral page") does not fire.
func MultiWordTerms(entries []Entry) []string {
	seen := map[string]bool{}
	var out []string
	for _, e := range entries {
		key := strings.ToLower(e.Term)
		if !strings.Contains(e.Term, " ") || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e.Term)
	}
	sort.Slice(
		out,
		func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) },
	)
	return out
}

func appendUnique(list []string, s string) []string {
	for _, have := range list {
		if have == s {
			return list
		}
	}
	return append(list, s)
}

// header is the humanizer-* metadata block of the generated rule. The rules
// registry parses the same block, so `rules explain STE.Glossary` and the
// generated file cannot disagree.
const header = `# humanizer-id: STE.Glossary
# humanizer-name: Glossary term
# humanizer-category: ste
# humanizer-profile: procedure, description
# humanizer-summary: A synonym that a CONTEXT.md glossary bans, in place of its glossary term.
# humanizer-rationale: Generated at run time from the _Avoid_ lists of the CONTEXT.md files that govern the checked file: a CONTEXT.md applies to its own directory and everything below it, and text with no location gets every glossary in the repo. A glossary scopes a word to one sense, so a hit is a prompt to check the sense, not an error: "delete" can be the glossary's Retract or a plain file deletion. Use the term when you mean the glossary sense.
# humanizer-before: Delete the assertion when its pin goes stale.
# humanizer-after: Retract the assertion when its pin goes stale.
# humanizer-reference: CONTEXT.md files in the repo (bold term line, definition, _Avoid_ line)
extends: substitution
message: "Use '%s' instead of '%s' if you mean the glossary sense."
level: suggestion
ignorecase: true
`

// Render writes subs as a vale substitution rule, with exceptions as phrases
// a match inside is never reported. Swap keys and exceptions are regular
// expressions, so each is quoted. vale prefixes every exception with (?-i),
// so each one also gets (?i) to match in any case. Render(nil, nil) is the
// bare rule with an empty swap map.
func Render(subs []Substitution, exceptions []string) []byte {
	var b bytes.Buffer
	b.WriteString(header)
	if len(exceptions) > 0 {
		b.WriteString("exceptions:\n")
		for _, e := range exceptions {
			fmt.Fprintf(&b, "  - %s\n", yamlQuote(caseInsensitive+regexp.QuoteMeta(e)))
		}
	}
	if len(subs) == 0 {
		b.WriteString("swap: {}\n")
		return b.Bytes()
	}
	b.WriteString("swap:\n")
	for _, s := range subs {
		fmt.Fprintf(&b, "  %s: %s\n", yamlQuote(regexp.QuoteMeta(s.Avoid)), yamlQuote(s.Use))
	}
	return b.Bytes()
}

// caseInsensitive is the inline regexp flag that undoes vale's (?-i) prefix.
const caseInsensitive = "(?i)"

// yamlQuote returns s as a YAML single-quoted scalar.
func yamlQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
