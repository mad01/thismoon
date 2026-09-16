package outline

import (
	"iter"
	"path/filepath"
	"unicode"
	"unicode/utf8"
)

// minRefName is the shortest name that earns a reference count. Shorter
// names (i, ok, id) collide with too many unrelated tokens to say anything.
const minRefName = 3

// refCounter counts, per identifier, the files holding it as a whole token
// and the files defining it, at two scopes: the whole repo, and the
// directory being processed. Files are processed directory by directory,
// so the directory maps are cleared as each one ends. One map per scope
// serves every token: each entry records a file count and the index of
// the last file that counted it, so a file repeating a token bumps the
// count once and a lookup never allocates.
type refCounter struct {
	files int
	repo  scopeCounts
	dir   scopeCounts
}

// scopeCounts holds one scope's token counts and, per kind group, its
// definer counts: a Repo struct and twenty Repo fields are different
// things, so the fields share mentions among themselves, not with the
// struct.
type scopeCounts struct {
	tokens   map[string]*tokenStat
	definers [kindGroups]map[string]*tokenStat
}

// tokenStat is one name's file count and the last file that counted it.
type tokenStat struct {
	files int
	last  int
}

func newRefCounter() *refCounter {
	return &refCounter{repo: newScopeCounts(), dir: newScopeCounts()}
}

func newScopeCounts() scopeCounts {
	sc := scopeCounts{tokens: make(map[string]*tokenStat)}
	for i := range sc.definers {
		sc.definers[i] = make(map[string]*tokenStat)
	}
	return sc
}

// beginDir starts a new directory: its counts start from zero.
func (c *refCounter) beginDir() {
	clear(c.dir.tokens)
	for _, m := range c.dir.definers {
		clear(m)
	}
}

// addFile tokenizes one file and counts each distinct identifier once, in
// both scopes.
func (c *refCounter) addFile(content []byte) {
	c.files++
	for tok := range identifiers(content) {
		c.repo.bump(c.repo.tokens, tok, c.files)
		c.dir.bump(c.dir.tokens, tok, c.files)
	}
}

// addDefiner records that the current file defines name with kind, in
// both scopes.
func (c *refCounter) addDefiner(name, kind string) {
	g := kindWeight(kind)
	c.repo.bump(c.repo.definers[g], []byte(name), c.files)
	c.dir.bump(c.dir.definers[g], []byte(name), c.files)
}

// bump counts key for file once.
func (scopeCounts) bump(m map[string]*tokenStat, key []byte, file int) {
	st, ok := m[string(key)]
	if !ok {
		m[string(key)] = &tokenStat{files: 1, last: file}
		return
	}
	if st.last != file {
		st.files++
		st.last = file
	}
}

// refs returns the reference count of name at the scope the definition
// lives in: the current directory for a package-private name (Go names
// starting lowercase, visible only inside their package), the whole repo
// otherwise. Call it for a directory's definitions before the next
// beginDir.
//
// The count is the files holding name as a whole token, minus the files
// defining it with a kind of the same group, shared evenly among those
// definers: a name defined once gets every other file that mentions it,
// while a `New` defined in twelve packages splits its mentions twelve ways
// and an `init` defined in every file of a package gets none. A name that
// is not one identifier (a heading with spaces) or is shorter than
// minRefName gets 0.
func (c *refCounter) refs(e Entry) int {
	if !isIdentifier(e.Name) {
		return 0
	}
	scope := c.repo
	if isPackagePrivate(e) {
		scope = c.dir
	}
	tokens, ok := scope.tokens[e.Name]
	if !ok {
		return 0
	}
	definers := 1
	if d, ok := scope.definers[kindWeight(e.Kind)][e.Name]; ok {
		definers = max(d.files, 1)
	}
	return max((tokens.files-definers)/definers, 0)
}

// isPackagePrivate reports a Go name that starts with a lowercase letter or
// an underscore: unexported, so no file outside its directory can refer to
// it.
func isPackagePrivate(e Entry) bool {
	if filepath.Ext(e.File) != ".go" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(e.Name)
	return r == '_' || unicode.IsLower(r)
}

// identifiers yields every whole identifier token in src: a maximal run of
// letters, digits, and underscores that does not start with a digit and is
// at least minRefName bytes long. Bytes above ASCII count as letters, so a
// Unicode name stays one token.
func identifiers(src []byte) iter.Seq[[]byte] {
	return func(yield func([]byte) bool) {
		for i := 0; i < len(src); {
			if !isIdentByte(src[i]) {
				i++
				continue
			}
			start := i
			for i < len(src) && isIdentByte(src[i]) {
				i++
			}
			tok := src[start:i]
			if len(tok) < minRefName || isDigit(tok[0]) {
				continue
			}
			if !yield(tok) {
				return
			}
		}
	}
}

// isIdentifier reports whether name would come out of identifiers as one
// token.
func isIdentifier(name string) bool {
	if len(name) < minRefName || isDigit(name[0]) {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !isIdentByte(name[i]) {
			return false
		}
	}
	return true
}

func isIdentByte(b byte) bool {
	return b == '_' || isDigit(b) ||
		(b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b >= 0x80
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
