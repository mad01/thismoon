package outline

import (
	"strings"
	"testing"
)

func TestIdentifiers(t *testing.T) {
	src := []byte("func NewFoo(x int) *Foo { return &Foo{id: 42, ok: a_b} } // Fooé 3rd 0x1F")
	var got []string
	for tok := range identifiers(src) {
		got = append(got, string(tok))
	}
	want := "func NewFoo int Foo return Foo a_b Fooé"
	if s := strings.Join(got, " "); s != want {
		t.Errorf("identifiers = %q, want %q", s, want)
	}
}

// addDefining feeds one file that defines each of names as a struct.
func addDefining(c *refCounter, content string, names ...string) {
	c.addFile([]byte(content))
	for _, n := range names {
		c.addDefiner(n, "struct")
	}
}

// TestRefCounter_CountsDistinctFilesForWholeTokens: three files, one defines
// Foo, two reference it, one only as part of FooBar. Foo has 2 refs; a file
// repeating the name counts once; the defining file is subtracted.
func TestRefCounter_CountsDistinctFilesForWholeTokens(t *testing.T) {
	c := newRefCounter()
	c.beginDir()
	addDefining(c, "type Foo struct{}\nfunc (f Foo) M() {}\n", "Foo", "M")
	c.addFile([]byte("var a Foo\nvar b Foo\n"))
	c.addFile([]byte("x := Foo{}\n"))
	addDefining(c, "type FooBar struct{}\n", "FooBar")

	if c.files != 4 {
		t.Errorf("files = %d, want 4", c.files)
	}
	if got := c.refs(Entry{Name: "Foo", File: "a.ts"}); got != 2 {
		t.Errorf("refs(Foo) = %d, want 2", got)
	}
	if got := c.refs(Entry{Name: "FooBar", File: "d.ts"}); got != 0 {
		t.Errorf("refs(FooBar) = %d, want 0 (defined in one file, referenced nowhere)", got)
	}
	for _, name := range []string{"M", "ok", "Foo Bar", "Foo.M", "42x", "Missing"} {
		if got := c.refs(Entry{Name: name, File: "a.ts"}); got != 0 {
			t.Errorf("refs(%q) = %d, want 0", name, got)
		}
	}
}

// TestRefCounter_SharesAmongDefinersOfOneGroup: a name defined in two
// files splits the four other files that mention it, so each definer gets
// 2; a field of the same name is another group and does not dilute them.
func TestRefCounter_SharesAmongDefinersOfOneGroup(t *testing.T) {
	c := newRefCounter()
	c.beginDir()
	addDefining(c, "type New struct{}\n", "New")
	addDefining(c, "type New struct{}\n", "New")
	c.addFile([]byte("type T struct{ New int }\n"))
	c.addDefiner("New", "field")
	for range 3 {
		c.addFile([]byte("New()\n"))
	}
	if got := c.refs(Entry{Name: "New", Kind: "struct", File: "a.ts"}); got != 2 {
		t.Errorf("refs(New struct) = %d, want (6-2)/2 = 2", got)
	}
	if got := c.refs(Entry{Name: "New", Kind: "field", File: "c.ts"}); got != 5 {
		t.Errorf("refs(New field) = %d, want 6-1 = 5", got)
	}
}

// TestRefCounter_PackagePrivateCountsInsideTheDirectory: a lowercase Go name
// counts only files of its own directory, so a mention elsewhere is noise;
// the same name in an exported form, or in another language, counts
// repo-wide.
func TestRefCounter_PackagePrivateCountsInsideTheDirectory(t *testing.T) {
	c := newRefCounter()
	c.beginDir()
	addDefining(c, "package a\nfunc helper() {}\nfunc Helper() {}\n", "helper", "Helper")
	c.addFile([]byte("package a\nvar _ = helper\nvar _ = Helper\n"))
	private := c.refs(Entry{Name: "helper", File: "a/x.go"})
	c.beginDir()
	c.addFile([]byte("package b\nvar _ = helper\nvar _ = Helper\n"))
	c.addFile([]byte("package b\nvar _ = Helper\n"))

	if private != 1 {
		t.Errorf("refs(helper) = %d, want 1 (only the sibling file counts)", private)
	}
	if got := c.refs(Entry{Name: "Helper", File: "a/x.go"}); got != 3 {
		t.Errorf("refs(Helper) = %d, want 3 (repo-wide)", got)
	}
	if got := c.refs(Entry{Name: "helper", File: "a/x.py"}); got != 2 {
		t.Errorf("refs(helper in Python) = %d, want 2 (repo-wide: the rule is Go's)", got)
	}
}

func TestIsPackagePrivate(t *testing.T) {
	cases := map[Entry]bool{
		{Name: "helper", File: "a.go"}: true,
		{Name: "_x", File: "a.go"}:     true,
		{Name: "Helper", File: "a.go"}: false,
		{Name: "helper", File: "a.py"}: false,
		{Name: "été", File: "a.go"}:    true,
	}
	for e, want := range cases {
		if got := isPackagePrivate(e); got != want {
			t.Errorf("isPackagePrivate(%q in %s) = %v, want %v", e.Name, e.File, got, want)
		}
	}
}
