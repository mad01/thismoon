package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureHome is the HOME the golden files were generated under.
const fixtureHome = "/home/fixture"

// TestSwitchMatchesScriptOutput pins the routes file each profile writes. The
// golden files are what the d-man-profile shell script this command replaces
// wrote for the same fixtures with HOME=/home/fixture, byte for byte.
func TestSwitchMatchesScriptOutput(t *testing.T) {
	for _, name := range []string{"work", "personal"} {
		t.Run(name, func(t *testing.T) {
			dir := fixtureDir(t)
			routes := filepath.Join(dir, "routes.toml")
			if err := Switch(routes, name, fixtureHome); err != nil {
				t.Fatalf("Switch: %v", err)
			}
			want := readFile(t, filepath.Join("testdata", "routes."+name+".golden.toml"))
			if got := readFile(t, routes); got != want {
				t.Errorf("routes file =\n%s\nwant\n%s", got, want)
			}
			if got := readFile(t, filepath.Join(dir, RecordFile)); got != name+"\n" {
				t.Errorf("record = %q, want %q", got, name+"\n")
			}
			if got, err := Current(routes); err != nil || got != name {
				t.Errorf("Current = %q, %v; want %q", got, err, name)
			}
		})
	}
}

// A routes file left as a symlink (the layout before generation) is replaced
// by a regular file; the file it pointed at is not written through.
func TestSwitchReplacesSymlink(t *testing.T) {
	dir := fixtureDir(t)
	routes := filepath.Join(dir, "routes.toml")
	if err := os.Symlink(filepath.Join(dir, BaseFile), routes); err != nil {
		t.Fatal(err)
	}
	if err := Switch(routes, "work", fixtureHome); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	info, err := os.Lstat(routes)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != fileMode {
		t.Errorf(
			"routes mode = %v, want a regular file with %v",
			info.Mode(),
			os.FileMode(fileMode),
		)
	}
	if got := readFile(t, filepath.Join(dir, BaseFile)); strings.Contains(got, "blocklist") {
		t.Error("the symlink target was written through")
	}
}

// The default profile reads its block list when one exists, like any other.
func TestSwitchDefaultUsesItsBlocklistWhenPresent(t *testing.T) {
	dir := fixtureDir(t)
	writeFile(t, dir, BlocklistFile(Default), "blocklist = [\"own.example\"]\n")
	routes := filepath.Join(dir, "routes.toml")
	if err := Switch(routes, Default, fixtureHome); err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if got := readFile(t, routes); !strings.HasPrefix(got, "blocklist = [\"own.example\"]\n") {
		t.Errorf("routes file does not start with the personal block list:\n%s", got)
	}
}

// Every refusal leaves the live routes file and the record as they were.
func TestSwitchRefusals(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		setup   func(t *testing.T, dir string)
		wantErr string
	}{
		{
			name:    "missing base",
			profile: "work",
			setup: func(t *testing.T, dir string) {
				removeFile(t, filepath.Join(dir, BaseFile))
			},
			wantErr: "read base routes",
		},
		{
			name:    "profile without a block list names the ones that exist",
			profile: "focus",
			wantErr: "profiles: personal, work",
		},
		{
			name:    "assembled routes that do not validate",
			profile: "work",
			setup: func(t *testing.T, dir string) {
				writeFile(t, dir, BlocklistFile("work"), "blocklist = [\"csl.this\"]\n")
			},
			wantErr: "is also a route",
		},
		{
			name:    "a base that repeats a block-list key",
			profile: "work",
			setup: func(t *testing.T, dir string) {
				writeFile(t, dir, BaseFile, "blocklist = []\n")
			},
			wantErr: "left unchanged",
		},
		{
			name:    "name that is not a file-name segment",
			profile: "../work",
			wantErr: "invalid profile name",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := fixtureDir(t)
			routes := writeFile(t, dir, "routes.toml", "# live routes\n")
			record := writeFile(t, dir, RecordFile, "personal\n")
			if tt.setup != nil {
				tt.setup(t, dir)
			}
			err := Switch(routes, tt.profile, fixtureHome)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Switch error = %v, want one containing %q", err, tt.wantErr)
			}
			if got := readFile(t, routes); got != "# live routes\n" {
				t.Errorf("routes file changed to:\n%s", got)
			}
			if got := readFile(t, record); got != "personal\n" {
				t.Errorf("record changed to %q", got)
			}
		})
	}
}

func TestCurrent(t *testing.T) {
	tests := []struct {
		name    string
		record  *string
		want    string
		wantErr bool
	}{
		{name: "nothing recorded", want: Default},
		{name: "empty record", record: ptr(" \n"), want: Default},
		{name: "recorded profile", record: ptr("work\n"), want: "work"},
		{name: "record that is not a name", record: ptr("../x\n"), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.record != nil {
				writeFile(t, dir, RecordFile, *tt.record)
			}
			got, err := Current(filepath.Join(dir, "routes.toml"))
			if (err != nil) != tt.wantErr {
				t.Fatalf("Current error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("Current = %q, want %q", got, tt.want)
			}
		})
	}
}

// fixtureDir copies the testdata inputs into a fresh config directory.
func fixtureDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{BaseFile, BlocklistFile("work")} {
		writeFile(t, dir, name, readFile(t, filepath.Join("testdata", name)))
	}
	return dir
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func removeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}

func ptr(s string) *string { return &s }
