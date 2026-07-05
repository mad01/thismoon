package server

import (
	"strings"
	"testing"
)

func TestParseDiff(t *testing.T) {
	raw := `diff --git a/main.go b/main.go
index abc1234..def5678 100644
--- a/main.go
+++ b/main.go
@@ -1,5 +1,6 @@
 package main

+import "fmt"
+
 func main() {
-	println("hello")
+	fmt.Println("hello")
 }
`
	files := parseDiff(raw)
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].Name != "main.go" {
		t.Errorf("expected file name main.go, got %q", files[0].Name)
	}
	if len(files[0].Hunks) != 1 {
		t.Fatalf("expected 1 hunk, got %d", len(files[0].Hunks))
	}
	hunk := files[0].Hunks[0]
	adds, dels := 0, 0
	for _, l := range hunk.Lines {
		switch l.Type {
		case "add":
			adds++
		case "del":
			dels++
		}
	}
	if adds != 3 {
		t.Errorf("expected 3 additions, got %d", adds)
	}
	if dels != 1 {
		t.Errorf("expected 1 deletion, got %d", dels)
	}
}

func TestParseDiffMultipleFiles(t *testing.T) {
	raw := `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,3 +1,3 @@
 package a
-var x = 1
+var x = 2
diff --git a/b.go b/b.go
--- /dev/null
+++ b/b.go
@@ -0,0 +1,3 @@
+package b
+
+var y = 1
`
	files := parseDiff(raw)
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}
	if files[0].Name != "a.go" {
		t.Errorf("file 0: expected a.go, got %q", files[0].Name)
	}
	if files[1].Name != "b.go" {
		t.Errorf("file 1: expected b.go, got %q", files[1].Name)
	}
	if files[1].Status != "added" {
		t.Errorf("file 1: expected status added, got %q", files[1].Status)
	}
}

func TestRenderDiffHTML(t *testing.T) {
	files := []DiffFile{{
		Name: "test.go",
		Hunks: []DiffHunk{{
			Header: "@@ -1,3 +1,3 @@",
			Lines: []DiffLine{
				{Type: "ctx", Content: "package main", OldNum: 1, NewNum: 1},
				{Type: "del", Content: "old line", OldNum: 2},
				{Type: "add", Content: "new line", NewNum: 2},
			},
		}},
	}}
	html := renderDiffHTML(files)
	if html == "" {
		t.Error("expected non-empty HTML")
	}
	if !strings.Contains(html, "diff-add") {
		t.Error("expected diff-add class in HTML")
	}
	if !strings.Contains(html, "diff-del") {
		t.Error("expected diff-del class in HTML")
	}
	if !strings.Contains(html, "test.go") {
		t.Error("expected filename in HTML")
	}
}

func TestParseHunkHeader(t *testing.T) {
	tests := []struct {
		name             string
		line             string
		wantOld, wantNew int
	}{
		{"ranges with counts", "@@ -12,5 +34,6 @@", 12, 34},
		{"omitted counts", "@@ -7 +9 @@", 7, 9},
		{"trailing section heading", "@@ -1,3 +1,4 @@ func main()", 1, 1},
		{"unparseable falls back to 1", "@@ garbage @@", 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOld, gotNew := parseHunkHeader(tt.line)
			if gotOld != tt.wantOld || gotNew != tt.wantNew {
				t.Errorf("parseHunkHeader(%q) = (%d, %d), want (%d, %d)",
					tt.line, gotOld, gotNew, tt.wantOld, tt.wantNew)
			}
		})
	}
}
