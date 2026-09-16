package symbols

import (
	"bytes"
	"testing"
)

// want is the contract for one extracted symbol: the exact source text its
// byte range must slice to, its kind, and its parent.
type want struct {
	name, kind, parent, parentKind string
}

// checkSymbols asserts Extract yields exactly wants, in order, with every
// range slicing to the expected name, and that the output holds the zoekt
// invariants (sorted, non-overlapping, inside the content).
func checkSymbols(t *testing.T, path, src string, wants []want) {
	t.Helper()
	syms, err := Extract(path, []byte(src))
	if err != nil {
		t.Fatalf("Extract(%s): %v", path, err)
	}
	var got []want
	for _, s := range syms {
		if int(s.End) > len(src) || s.Start >= s.End {
			t.Fatalf("symbol range [%d,%d) outside content of %d bytes", s.Start, s.End, len(src))
		}
		got = append(got, want{src[s.Start:s.End], s.Kind, s.Parent, s.ParentKind})
	}
	if len(got) != len(wants) {
		t.Fatalf(
			"Extract(%s) = %d symbols, want %d:\n got %+v\nwant %+v",
			path,
			len(got),
			len(wants),
			got,
			wants,
		)
	}
	for i := range wants {
		if got[i] != wants[i] {
			t.Errorf("symbol %d = %+v, want %+v", i, got[i], wants[i])
		}
	}
	for i := 1; i < len(syms); i++ {
		if syms[i].Start < syms[i-1].End {
			t.Errorf(
				"symbols %d and %d overlap or are unsorted: %+v %+v",
				i-1,
				i,
				syms[i-1],
				syms[i],
			)
		}
	}
}

func TestExtractGo(t *testing.T) {
	src := `package demo

import "fmt"

const Answer = 42

const (
	A, B = 1, 2
)

var counter int

type Point struct {
	X, Y int
	Name string
	embedded.Thing
}

type Shape interface {
	Area() float64
}

type Alias = Point

type ID string

func Add(a, b int) int { return a + b }

func (p Point) String() string { return fmt.Sprint(p.X) }

func (p *Point) Move(dx int) { var local = 1; p.X += dx + local }

func (s *Set[T]) Add(v T) {}
`
	checkSymbols(t, "a.go", src, []want{
		{"Answer", "const", "", ""},
		{"A", "const", "", ""},
		{"B", "const", "", ""},
		{"counter", "var", "", ""},
		{"Point", "struct", "", ""},
		{"X", "field", "Point", "struct"},
		{"Y", "field", "Point", "struct"},
		{"Name", "field", "Point", "struct"},
		{"Shape", "interface", "", ""},
		{"Area", "methodSpec", "Shape", "interface"},
		{"Alias", "typealias", "", ""},
		{"ID", "type", "", ""},
		{"Add", "function", "", ""},
		{"String", "method", "Point", "type"},
		{"Move", "method", "Point", "type"},
		{"Add", "method", "Set", "type"},
	})
}

func TestExtractTypeScript(t *testing.T) {
	src := `export function greet(name: string): string { return "hi"; }

export class Greeter {
  name: string;
  constructor(name: string) { this.name = name; }
  greet(): string { return this.name; }
}

export interface Shape { area(): number; size: number }

export type ID = string;

export enum Color { Red, Green = 2 }

export const arrow = (x: number) => x;
let count = 0;
const { a, b } = pair;

export namespace NS { export function inner() {} }
`
	checkSymbols(t, "a.ts", src, []want{
		{"greet", "function", "", ""},
		{"Greeter", "class", "", ""},
		{"name", "field", "Greeter", "class"},
		{"constructor", "method", "Greeter", "class"},
		{"greet", "method", "Greeter", "class"},
		{"Shape", "interface", "", ""},
		{"area", "method", "Shape", "interface"},
		{"size", "field", "Shape", "interface"},
		{"ID", "typealias", "", ""},
		{"Color", "enum", "", ""},
		{"Red", "enumerator", "Color", "enum"},
		{"Green", "enumerator", "Color", "enum"},
		{"arrow", "function", "", ""},
		{"count", "var", "", ""},
		{"NS", "namespace", "", ""},
		{"inner", "function", "NS", "namespace"},
	})
}

func TestExtractPython(t *testing.T) {
	src := `def top(a, b):
    def nested():
        pass
    return a

class Greeter:
    def __init__(self, name):
        self.name = name
    @staticmethod
    def make():
        return Greeter("x")

@decorator
def decorated():
    pass
`
	checkSymbols(t, "a.py", src, []want{
		{"top", "function", "", ""},
		{"nested", "function", "top", "function"},
		{"Greeter", "class", "", ""},
		{"__init__", "method", "Greeter", "class"},
		{"make", "method", "Greeter", "class"},
		{"decorated", "function", "", ""},
	})
}

func TestExtractJava(t *testing.T) {
	src := `public class Greeter {
    private String name;
    public static final int MAX = 3, MIN = 1;
    public Greeter(String name) { this.name = name; }
    public String greet() { int local = 1; return name; }
    enum Mode { A, B }
}

interface Shape { double area(); int SIDES = 4; }

record Pair(int a, int b) { Pair { } }
`
	checkSymbols(t, "A.java", src, []want{
		{"Greeter", "class", "", ""},
		{"name", "field", "Greeter", "class"},
		{"MAX", "field", "Greeter", "class"},
		{"MIN", "field", "Greeter", "class"},
		{"Greeter", "method", "Greeter", "class"},
		{"greet", "method", "Greeter", "class"},
		{"Mode", "enum", "Greeter", "class"},
		{"A", "enumerator", "Mode", "enum"},
		{"B", "enumerator", "Mode", "enum"},
		{"Shape", "interface", "", ""},
		{"area", "method", "Shape", "interface"},
		{"SIDES", "field", "Shape", "interface"},
		{"Pair", "class", "", ""},
		{"Pair", "method", "Pair", "class"},
	})
}

func TestExtractProtobuf(t *testing.T) {
	src := `syntax = "proto3";

message Match {
  string repo = 1;
  message Inner { int32 x = 1; }
  enum Kind { UNKNOWN = 0; }
}

service SearchDaemon {
  rpc Search(SearchRequest) returns (SearchResponse);
}
`
	checkSymbols(t, "a.proto", src, []want{
		{"Match", "struct", "", ""},
		{"repo", "field", "Match", "struct"},
		{"Inner", "struct", "Match", "struct"},
		{"x", "field", "Inner", "struct"},
		{"Kind", "enum", "Match", "struct"},
		{"UNKNOWN", "enumerator", "Kind", "enum"},
		{"SearchDaemon", "interface", "", ""},
		{"Search", "method", "SearchDaemon", "interface"},
	})
}

func TestExtractMarkdown(t *testing.T) {
	src := "# Title\n\nSome text\n\n## Sub heading\n\nSetext\n======\n"
	checkSymbols(t, "a.md", src, []want{
		{"Title", "section", "", ""},
		{"Sub heading", "section", "", ""},
		{"Setext", "section", "", ""},
	})
}

func TestExtractBash(t *testing.T) {
	src := "#!/bin/bash\nfunction foo() { echo hi; }\nbaz() { echo hi; }\n"
	checkSymbols(t, "a.sh", src, []want{
		{"foo", "function", "", ""},
		{"baz", "function", "", ""},
	})
}

// TestExtractMultibyteOffsets pins that ranges are byte offsets, not rune
// offsets: two 2-byte runes precede the declaration, so a rune-counting
// implementation would land two bytes early.
func TestExtractMultibyteOffsets(t *testing.T) {
	src := "package p\n\n// héllo wörld\nfunc Add() {}\n"
	syms, err := Extract("a.go", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(syms) != 1 {
		t.Fatalf("got %d symbols, want 1: %+v", len(syms), syms)
	}
	wantStart := uint32(bytes.Index([]byte(src), []byte("Add")))
	if wantStart != 33 {
		t.Fatalf("fixture drifted: Add starts at byte %d, want 33", wantStart)
	}
	if syms[0].Start != wantStart || syms[0].End != wantStart+3 {
		t.Errorf(
			"range = [%d,%d), want [%d,%d)",
			syms[0].Start,
			syms[0].End,
			wantStart,
			wantStart+3,
		)
	}
}

// TestExtractSyntaxError: a file that does not parse cleanly still yields the
// declarations tree-sitter recovered, and never an error or a panic.
func TestExtractSyntaxError(t *testing.T) {
	src := "package p\n\nfunc Good() {}\n\nfunc broken( {\n\ntype T struct {\n"
	syms, err := Extract("broken.go", []byte(src))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	var names []string
	for _, s := range syms {
		names = append(names, src[s.Start:s.End])
	}
	if len(names) == 0 || names[0] != "Good" {
		t.Errorf("recovered symbols = %v, want Good first", names)
	}
}

// TestExtractNoRules: linked grammars without rules and unknown files yield
// nil without error, so the indexer adds them as plain documents.
func TestExtractNoRules(t *testing.T) {
	for _, path := range []string{"a.yaml", "a.sql", "a.tf", "Dockerfile", "notes.txt", "noext"} {
		syms, err := Extract(path, []byte("key: value\n"))
		if err != nil || syms != nil {
			t.Errorf("Extract(%s) = %v, %v; want nil, nil", path, syms, err)
		}
	}
}

// TestNormalizeInvariants covers the one place the zoekt builder's checks are
// enforced: unsorted input is sorted, and empty, out-of-range, rune-splitting,
// and overlapping ranges are dropped.
func TestNormalizeInvariants(t *testing.T) {
	content := []byte("ab é cd") // é is bytes 3-4
	got := normalize(content, []Symbol{
		{Start: 5, End: 7, Kind: "b"}, // "cd", out of order
		{Start: 0, End: 2, Kind: "a"}, // "ab"
		{Start: 1, End: 3, Kind: "x"}, // overlaps "ab"
		{Start: 3, End: 4, Kind: "x"}, // splits é
		{Start: 4, End: 4, Kind: "x"}, // empty
		{Start: 6, End: 9, Kind: "x"}, // past the end
	})
	if len(got) != 2 || got[0].Kind != "a" || got[1].Kind != "b" {
		t.Fatalf("normalize = %+v, want [ab cd]", got)
	}
	if normalize(content, nil) != nil {
		t.Error("normalize(nil) should be nil")
	}
}
