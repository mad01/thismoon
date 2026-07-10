package semantic

import (
	"strings"
	"testing"
)

func TestLangForPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"a/b/main.go", "go"},
		{"x.ts", "typescript"},
		{"x.tsx", "typescript"},
		{"y.py", "python"},
		{"src/Main.java", "java"},
		{"README.md", ""},
		{"noext", ""},
	}
	for _, tt := range tests {
		if got := LangForPath(tt.path); got != tt.want {
			t.Errorf("LangForPath(%q) = %q, want %q", tt.path, got, tt.want)
		}
	}
}

const goSrc = `package demo

import "fmt"

func Add(a, b int) int {
	return a + b
}

type Point struct {
	X int
	Y int
}

func (p Point) String() string {
	return fmt.Sprintf("%d,%d", p.X, p.Y)
}
`

const tsSrc = `import { z } from "lib";

export function greet(name: string): string {
  return "hi " + name;
}

class Widget {
  render() {
    return null;
  }
}
`

const javaSrc = `package com.example;

import java.util.List;

public class Greeter {
    private final String name;

    public Greeter(String name) {
        this.name = name;
    }

    public String greet(List<String> extras) {
        return "Hello, " + name + extras;
    }

    public static class Inner {
        int answer() {
            return 42;
        }
    }
}

interface Shape {
    double area();
}

enum Color {
    RED,
    GREEN
}
`

func chunkByKind(chunks []Chunk) map[string][]Chunk {
	m := make(map[string][]Chunk)
	for _, c := range chunks {
		m[c.Kind] = append(m[c.Kind], c)
	}
	return m
}

func TestChunkFileGo(t *testing.T) {
	chunks, err := ChunkFile("demo/repo", "demo.go", "go", []byte(goSrc))
	if err != nil {
		t.Fatalf("ChunkFile: %v", err)
	}
	byKind := chunkByKind(chunks)

	wantKinds := map[string]int{
		"function_declaration": 1,
		"type_declaration":     1,
		"method_declaration":   1,
	}
	for kind, n := range wantKinds {
		if len(byKind[kind]) != n {
			t.Errorf("kind %q: got %d chunks, want %d (all: %+v)", kind, len(byKind[kind]), n, kindSummary(chunks))
		}
	}

	// Add spans lines 5-7 (1-based inclusive).
	fn := byKind["function_declaration"][0]
	if fn.StartLine != 5 || fn.EndLine != 7 {
		t.Errorf("Add: got lines %d-%d, want 5-7", fn.StartLine, fn.EndLine)
	}
	if !strings.Contains(fn.Text, "func Add(a, b int) int") {
		t.Errorf("Add text missing body: %q", fn.Text)
	}

	// type Point spans lines 9-12.
	td := byKind["type_declaration"][0]
	if td.StartLine != 9 || td.EndLine != 12 {
		t.Errorf("Point: got lines %d-%d, want 9-12", td.StartLine, td.EndLine)
	}

	// Breadcrumb assertions on EmbedText.
	if !strings.Contains(fn.EmbedText, "// File: demo.go") {
		t.Errorf("EmbedText missing file breadcrumb: %q", fn.EmbedText)
	}
	if !strings.Contains(fn.EmbedText, "// Lang: go") || !strings.Contains(fn.EmbedText, "Kind: function_declaration") {
		t.Errorf("EmbedText missing lang/kind breadcrumb: %q", fn.EmbedText)
	}
	if !strings.Contains(fn.EmbedText, fn.Text) {
		t.Errorf("EmbedText must contain Text body")
	}
}

func TestChunkFileTypeScript(t *testing.T) {
	chunks, err := ChunkFile("demo/repo", "demo.ts", "typescript", []byte(tsSrc))
	if err != nil {
		t.Fatalf("ChunkFile: %v", err)
	}
	if len(chunks) == 0 {
		t.Fatal("expected chunks, got none")
	}

	var haveFn, haveClass bool
	for _, c := range chunks {
		if strings.Contains(c.Text, "function greet") {
			haveFn = true
			if c.StartLine != 3 || c.EndLine != 5 {
				t.Errorf("greet: got lines %d-%d, want 3-5", c.StartLine, c.EndLine)
			}
		}
		if c.Kind == "class_declaration" && strings.Contains(c.Text, "class Widget") {
			haveClass = true
		}
	}
	if !haveFn {
		t.Errorf("missing exported function chunk; kinds=%v", kindSummary(chunks))
	}
	if !haveClass {
		t.Errorf("missing class chunk; kinds=%v", kindSummary(chunks))
	}
}

func TestChunkFileJava(t *testing.T) {
	chunks, err := ChunkFile("demo/repo", "Greeter.java", "java", []byte(javaSrc))
	if err != nil {
		t.Fatalf("ChunkFile: %v", err)
	}
	byKind := chunkByKind(chunks)

	wantKinds := map[string]int{
		"class_declaration":       2, // Greeter header + Inner header
		"interface_declaration":   1, // Shape header
		"constructor_declaration": 1,
		"method_declaration":      3, // greet, answer, area
		"enum_declaration":        1, // Color, emitted whole
	}
	for kind, n := range wantKinds {
		if len(byKind[kind]) != n {
			t.Errorf("kind %q: got %d chunks, want %d (all: %v)", kind, len(byKind[kind]), n, kindSummary(chunks))
		}
	}

	// The Greeter header covers the signature and fields, and stops before the
	// first member so nothing is embedded twice.
	header := byKind["class_declaration"][0]
	if header.StartLine != 5 || header.EndLine != 6 {
		t.Errorf("Greeter header: got lines %d-%d, want 5-6", header.StartLine, header.EndLine)
	}
	if !strings.Contains(header.Text, "class Greeter") || !strings.Contains(header.Text, "private final String name;") {
		t.Errorf("Greeter header missing signature/fields: %q", header.Text)
	}
	if strings.Contains(header.Text, "this.name") {
		t.Errorf("Greeter header must not contain member bodies: %q", header.Text)
	}

	// greet spans lines 12-14 (1-based inclusive).
	var greet *Chunk
	for i := range chunks {
		if chunks[i].Kind == "method_declaration" && strings.Contains(chunks[i].Text, "greet") {
			greet = &chunks[i]
		}
	}
	if greet == nil {
		t.Fatalf("missing greet method chunk; kinds=%v", kindSummary(chunks))
	}
	if greet.StartLine != 12 || greet.EndLine != 14 {
		t.Errorf("greet: got lines %d-%d, want 12-14", greet.StartLine, greet.EndLine)
	}

	// The nested class Inner yields its own header and method chunks.
	var haveInnerHeader, haveAnswer bool
	for _, c := range chunks {
		if c.Kind == "class_declaration" && strings.Contains(c.Text, "class Inner") {
			haveInnerHeader = true
		}
		if c.Kind == "method_declaration" && strings.Contains(c.Text, "answer") {
			haveAnswer = true
		}
	}
	if !haveInnerHeader || !haveAnswer {
		t.Errorf("missing nested class chunks (header=%v method=%v); kinds=%v",
			haveInnerHeader, haveAnswer, kindSummary(chunks))
	}
}

func TestChunkFileFallbackWindows(t *testing.T) {
	// 95 lines of unknown-extension content => windows of 40 with 10 overlap,
	// i.e. stride 30: starts at 1, 31, 61, 91 => 4 windows.
	var b strings.Builder
	for i := 1; i <= 95; i++ {
		b.WriteString("line content here\n")
	}
	chunks, err := ChunkFile("demo/repo", "data.txt", "", []byte(b.String()))
	if err != nil {
		t.Fatalf("ChunkFile: %v", err)
	}
	if len(chunks) != 4 {
		t.Fatalf("got %d windows, want 4 (%v)", len(chunks), kindSummary(chunks))
	}
	first := chunks[0]
	if first.Kind != "window" {
		t.Errorf("fallback Kind = %q, want \"window\"", first.Kind)
	}
	if first.StartLine != 1 || first.EndLine != 40 {
		t.Errorf("first window lines = %d-%d, want 1-40", first.StartLine, first.EndLine)
	}
	if chunks[1].StartLine != 31 {
		t.Errorf("second window StartLine = %d, want 31", chunks[1].StartLine)
	}
	last := chunks[len(chunks)-1]
	if last.EndLine != 95 {
		t.Errorf("last window EndLine = %d, want 95 (clamped)", last.EndLine)
	}
	if !strings.Contains(first.EmbedText, "// File: data.txt") {
		t.Errorf("fallback EmbedText missing breadcrumb: %q", first.EmbedText)
	}
}

func kindSummary(chunks []Chunk) []string {
	out := make([]string, len(chunks))
	for i, c := range chunks {
		out[i] = c.Kind
	}
	return out
}
