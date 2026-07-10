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
		{"main.tf", "hcl"},
		{"vars.hcl", "hcl"},
		{"run.sh", "bash"},
		{"env.bash", "bash"},
		{"Dockerfile", "dockerfile"},
		{"docker/Dockerfile.dev", "dockerfile"},
		{"build.dockerfile", "dockerfile"},
		{"README.md", "markdown"},
		{"api.proto", "protobuf"},
		{"schema.sql", "sql"},
		{"config.yaml", "yaml"},
		{"ci.yml", "yaml"},
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

func TestChunkFileHCL(t *testing.T) {
	src := `variable "region" {
  type = string
}

resource "aws_instance" "web" {
  ami = "abc"
  tags = {
    Name = "web"
  }
}
`
	chunks, err := ChunkFile("demo/repo", "main.tf", "hcl", []byte(src))
	if err != nil {
		t.Fatalf("ChunkFile: %v", err)
	}
	// One chunk per top-level block; the nested tags block stays inside its
	// resource chunk.
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2 (%v)", len(chunks), kindSummary(chunks))
	}
	if chunks[0].Kind != "block" || chunks[0].StartLine != 1 || chunks[0].EndLine != 3 {
		t.Errorf("variable block: kind=%q lines %d-%d, want block 1-3", chunks[0].Kind, chunks[0].StartLine, chunks[0].EndLine)
	}
	if !strings.Contains(chunks[1].Text, `Name = "web"`) {
		t.Errorf("resource chunk must contain nested block: %q", chunks[1].Text)
	}
}

func TestChunkFileBash(t *testing.T) {
	src := `#!/bin/bash
set -euo pipefail

greet() {
  echo "hi $1"
}

greet world
`
	chunks, err := ChunkFile("demo/repo", "run.sh", "bash", []byte(src))
	if err != nil {
		t.Fatalf("ChunkFile: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Kind != "function_definition" {
		t.Fatalf("got %v, want one function_definition", kindSummary(chunks))
	}
	if chunks[0].StartLine != 4 || chunks[0].EndLine != 6 {
		t.Errorf("greet: got lines %d-%d, want 4-6", chunks[0].StartLine, chunks[0].EndLine)
	}
}

func TestChunkFileDockerfile(t *testing.T) {
	src := `# build image
ARG GO_VERSION=1.26
FROM golang:${GO_VERSION} AS build
RUN make build

FROM debian:stable
COPY --from=build /app /app
ENTRYPOINT ["/app"]
`
	chunks, err := ChunkFile("demo/repo", "Dockerfile", "dockerfile", []byte(src))
	if err != nil {
		t.Fatalf("ChunkFile: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("got %d chunks, want 2 stages (%v)", len(chunks), kindSummary(chunks))
	}
	// Leading comment and ARG belong to the first stage.
	if chunks[0].Kind != "stage" || chunks[0].StartLine != 1 || chunks[0].EndLine != 5 {
		t.Errorf("stage 1: kind=%q lines %d-%d, want stage 1-5", chunks[0].Kind, chunks[0].StartLine, chunks[0].EndLine)
	}
	if !strings.Contains(chunks[0].Text, "ARG GO_VERSION") {
		t.Errorf("stage 1 must include leading ARG: %q", chunks[0].Text)
	}
	if chunks[1].StartLine != 6 || chunks[1].EndLine != 8 || !strings.Contains(chunks[1].Text, "ENTRYPOINT") {
		t.Errorf("stage 2: lines %d-%d text %q", chunks[1].StartLine, chunks[1].EndLine, chunks[1].Text)
	}
}

func TestChunkFileMarkdown(t *testing.T) {
	src := `# Title

Intro paragraph.

## Section one

Body text.

` + "```go\ncode here\n```" + `

## Section two

More text.
`
	chunks, err := ChunkFile("demo/repo", "README.md", "markdown", []byte(src))
	if err != nil {
		t.Fatalf("ChunkFile: %v", err)
	}
	// Top section header (title + intro, stops at the first subsection) plus
	// one chunk per leaf section.
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3 (%v)", len(chunks), kindSummary(chunks))
	}
	for i, c := range chunks {
		if c.Kind != "section" {
			t.Errorf("chunk %d kind = %q, want section", i, c.Kind)
		}
	}
	if !strings.Contains(chunks[0].Text, "# Title") || !strings.Contains(chunks[0].Text, "Intro paragraph.") {
		t.Errorf("top section missing title/intro: %q", chunks[0].Text)
	}
	if strings.Contains(chunks[0].Text, "Section one") {
		t.Errorf("top section must stop before first subsection: %q", chunks[0].Text)
	}
	if !strings.Contains(chunks[1].Text, "code here") {
		t.Errorf("section one missing code block: %q", chunks[1].Text)
	}
	if chunks[2].StartLine != 13 {
		t.Errorf("section two StartLine = %d, want 13", chunks[2].StartLine)
	}
}

func TestChunkFileProtobuf(t *testing.T) {
	src := `syntax = "proto3";

package demo;

message Greeting {
  string name = 1;
}

enum Color {
  RED = 0;
}

service Greeter {
  rpc Greet(Greeting) returns (Greeting);
}
`
	chunks, err := ChunkFile("demo/repo", "api.proto", "protobuf", []byte(src))
	if err != nil {
		t.Fatalf("ChunkFile: %v", err)
	}
	byKind := chunkByKind(chunks)
	wantKinds := map[string]int{
		"message": 1,
		"enum":    1,
		"service": 1, // header
		"rpc":     1,
	}
	for kind, n := range wantKinds {
		if len(byKind[kind]) != n {
			t.Errorf("kind %q: got %d chunks, want %d (all: %v)", kind, len(byKind[kind]), n, kindSummary(chunks))
		}
	}
	if rpcs := byKind["rpc"]; len(rpcs) == 1 && !strings.Contains(rpcs[0].Text, "rpc Greet") {
		t.Errorf("rpc chunk text: %q", rpcs[0].Text)
	}
}

func TestChunkFileSQL(t *testing.T) {
	src := `CREATE TABLE users (
  id INT PRIMARY KEY,
  name TEXT
);

CREATE INDEX idx_users_name ON users (name);

INSERT INTO users (id, name) VALUES (1, 'a');
`
	chunks, err := ChunkFile("demo/repo", "schema.sql", "sql", []byte(src))
	if err != nil {
		t.Fatalf("ChunkFile: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3 statements (%v)", len(chunks), kindSummary(chunks))
	}
	if chunks[0].Kind != "statement" || chunks[0].StartLine != 1 || chunks[0].EndLine != 4 {
		t.Errorf("create table: kind=%q lines %d-%d, want statement 1-4", chunks[0].Kind, chunks[0].StartLine, chunks[0].EndLine)
	}
}

func TestChunkFileYAML(t *testing.T) {
	src := `name: demo
spec:
  replicas: 3
  ports:
    - 80
    - 443
---
kind: Other
`
	chunks, err := ChunkFile("demo/repo", "config.yaml", "yaml", []byte(src))
	if err != nil {
		t.Fatalf("ChunkFile: %v", err)
	}
	// One chunk per top-level mapping key, across both documents; nested keys
	// stay inside their parent chunk.
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks, want 3 (%v)", len(chunks), kindSummary(chunks))
	}
	if chunks[1].Kind != "block_mapping_pair" || chunks[1].StartLine != 2 || chunks[1].EndLine != 6 {
		t.Errorf("spec: kind=%q lines %d-%d, want block_mapping_pair 2-6", chunks[1].Kind, chunks[1].StartLine, chunks[1].EndLine)
	}
	if !strings.Contains(chunks[1].Text, "replicas: 3") {
		t.Errorf("spec chunk must contain nested keys: %q", chunks[1].Text)
	}
	if !strings.Contains(chunks[2].Text, "kind: Other") {
		t.Errorf("second document chunk: %q", chunks[2].Text)
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
