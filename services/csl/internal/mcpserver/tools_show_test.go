package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// stubOpenURL replaces openURL for the test, recording the opened URL.
func stubOpenURL(t *testing.T, err error) *string {
	t.Helper()
	var got string
	orig := openURL
	openURL = func(u string) error {
		got = u
		return err
	}
	t.Cleanup(func() { openURL = orig })
	return &got
}

func TestHandleShowFile_BuildsURLAndOpens(t *testing.T) {
	cleanup := setupReadTestRepo(t, "main.go", 5)
	defer cleanup()
	opened := stubOpenURL(t, nil)

	_, out, err := handleShowFile(context.Background(), nil, showFileInput{
		Repo:      "testrepo",
		File:      "main.go",
		StartLine: 2,
		EndLine:   4,
	})
	if err != nil {
		t.Fatalf("handleShowFile: %v", err)
	}

	want := "http://127.0.0.1:7424/file?end=4&file=main.go&repo=org%2Ftestrepo&start=2"
	if out.URL != want {
		t.Errorf("URL = %q, want %q", out.URL, want)
	}
	if *opened != out.URL {
		t.Errorf("opened %q, want the returned URL %q", *opened, out.URL)
	}
	if !out.Opened {
		t.Errorf("Opened = false, want true")
	}
	if out.Repo != "org/testrepo" || out.File != "main.go" {
		t.Errorf("repo/file = %q/%q, want org/testrepo/main.go", out.Repo, out.File)
	}
	if !strings.HasSuffix(out.LocalPath, "/org/testrepo/main.go") {
		t.Errorf("LocalPath = %q, want absolute path ending in /org/testrepo/main.go", out.LocalPath)
	}
}

func TestHandleShowFile_StartLineOnlyDefaultsEnd(t *testing.T) {
	cleanup := setupReadTestRepo(t, "main.go", 5)
	defer cleanup()
	stubOpenURL(t, nil)

	_, out, err := handleShowFile(context.Background(), nil, showFileInput{
		Repo: "testrepo", File: "main.go", StartLine: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.URL, "start=3") || !strings.Contains(out.URL, "end=3") {
		t.Errorf("URL = %q, want start=3 and end=3", out.URL)
	}
}

func TestHandleShowFile_NoOpen(t *testing.T) {
	cleanup := setupReadTestRepo(t, "main.go", 5)
	defer cleanup()
	opened := stubOpenURL(t, nil)

	_, out, err := handleShowFile(context.Background(), nil, showFileInput{
		Repo: "testrepo", File: "main.go", NoOpen: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Opened || *opened != "" {
		t.Errorf("browser opened despite no_open=true (opened=%v url=%q)", out.Opened, *opened)
	}
}

func TestHandleShowFile_OpenFailureWarnsInsteadOfErroring(t *testing.T) {
	cleanup := setupReadTestRepo(t, "main.go", 5)
	defer cleanup()
	stubOpenURL(t, fmt.Errorf("no display"))

	_, out, err := handleShowFile(context.Background(), nil, showFileInput{
		Repo: "testrepo", File: "main.go",
	})
	if err != nil {
		t.Fatalf("open failure should not fail the call: %v", err)
	}
	if out.Opened || out.Warning == "" {
		t.Errorf("want Opened=false with a warning, got opened=%v warning=%q", out.Opened, out.Warning)
	}
	if out.URL == "" {
		t.Errorf("URL should still be returned when the browser fails to open")
	}
}

func TestHandleShowFile_Errors(t *testing.T) {
	cleanup := setupReadTestRepo(t, "main.go", 5)
	defer cleanup()
	stubOpenURL(t, nil)

	tests := []struct {
		name string
		in   showFileInput
	}{
		{"missing repo", showFileInput{File: "main.go"}},
		{"missing file", showFileInput{Repo: "testrepo"}},
		{"nonexistent file", showFileInput{Repo: "testrepo", File: "nope.go"}},
		{"directory", showFileInput{Repo: "testrepo", File: "."}},
		{"start after end", showFileInput{Repo: "testrepo", File: "main.go", StartLine: 5, EndLine: 2}},
		{"negative line", showFileInput{Repo: "testrepo", File: "main.go", StartLine: -1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := handleShowFile(context.Background(), nil, tt.in); err == nil {
				t.Errorf("handleShowFile(%+v) succeeded, want error", tt.in)
			}
		})
	}
}
