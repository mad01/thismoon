package stale

import (
	"bufio"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// CodeownersFiles are where a vault's CODEOWNERS may live, in the order
// GitHub looks: the first that exists is the one used.
var CodeownersFiles = []string{".github/CODEOWNERS", "CODEOWNERS"}

// Owners is a parsed CODEOWNERS file: the last rule matching a path names
// its owners, as on GitHub.
type Owners struct {
	rules []ownerRule
}

type ownerRule struct {
	match  *regexp.Regexp
	owners []string
}

// LoadOwners reads the vault's CODEOWNERS. A vault without one has no
// rules, so every path is unowned by CODEOWNERS.
func LoadOwners(root string) (Owners, error) {
	for _, name := range CodeownersFiles {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return Owners{}, err
		}
		return ParseOwners(data), nil
	}
	return Owners{}, nil
}

// ParseOwners parses CODEOWNERS text. Blank lines and # comments are
// skipped; a line is a gitignore-style pattern followed by its owners. A
// pattern with no owners matches and leaves the path unowned.
func ParseOwners(data []byte) Owners {
	var o Owners
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := sc.Text()
		if i := strings.Index(line, " #"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		o.rules = append(o.rules, ownerRule{match: patternRegexp(fields[0]), owners: fields[1:]})
	}
	return o
}

// Of returns the owners of the vault-relative path rel, nil when no rule
// names any.
func (o Owners) Of(rel string) []string {
	for i := len(o.rules) - 1; i >= 0; i-- {
		if o.rules[i].match.MatchString(rel) {
			return o.rules[i].owners
		}
	}
	return nil
}

// patternRegexp compiles a CODEOWNERS pattern. A pattern with a slash
// before its end is anchored at the root, one without matches at any
// depth; one ending in a slash matches only what is under that directory,
// and any pattern matching a directory matches everything under it. * and
// ? stay within one path segment, ** crosses segments.
func patternRegexp(pattern string) *regexp.Regexp {
	dirOnly := strings.HasSuffix(pattern, "/")
	p := strings.Trim(pattern, "/")
	anchored := strings.HasPrefix(pattern, "/") || strings.Contains(p, "/")
	var b strings.Builder
	b.WriteString("^")
	if !anchored {
		b.WriteString("(?:.*/)?")
	}
	for i := 0; i < len(p); i++ {
		switch {
		case strings.HasPrefix(p[i:], "**/"):
			b.WriteString("(?:.*/)?")
			i += 2
		case strings.HasPrefix(p[i:], "**"):
			b.WriteString(".*")
			i++
		case p[i] == '*':
			b.WriteString("[^/]*")
		case p[i] == '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(p[i : i+1]))
		}
	}
	if dirOnly {
		b.WriteString("/.*$")
	} else {
		b.WriteString("(?:/.*)?$")
	}
	return regexp.MustCompile(b.String())
}
