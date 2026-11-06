package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// TestFakeGH runs the fake gh the way brain would and reads its log back.
func TestFakeGH(t *testing.T) {
	bin := t.TempDir()
	if err := writeFakeGH(bin); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "gh.log")
	gh := func(args ...string) (string, string, error) {
		cmd := exec.Command(filepath.Join(bin, "gh"), args...)
		cmd.Env = append(os.Environ(), ghLogEnv+"="+log)
		var stderr strings.Builder
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		return string(out), stderr.String(), err
	}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"pr", "create", "--title", "Fix port", "--body", "one\ntwo"}, fakeGHRepo + "/pull/101\n"},
		{[]string{"pr", "create", "--title", "Promote"}, fakeGHRepo + "/pull/102\n"},
		{[]string{"issue", "create", "--title", "Wrong port"}, fakeGHRepo + "/issues/201\n"},
	} {
		if out, _, err := gh(tc.args...); err != nil || out != tc.want {
			t.Errorf("gh %v = %q, %v; want %q", tc.args, out, err, tc.want)
		}
	}
	out, _, err := gh("pr", "view", fakeGHRepo+"/pull/101", "--json", "state,mergedAt")
	var pr struct {
		State    string  `json:"state"`
		MergedAt *string `json:"mergedAt"`
	}
	if err != nil || json.Unmarshal([]byte(out), &pr) != nil || pr.State != "OPEN" || pr.MergedAt != nil {
		t.Errorf("gh pr view = %q, %v", out, err)
	}
	if _, _, err := gh("auth", "status"); err != nil {
		t.Errorf("gh auth status: %v", err)
	}
	if _, stderr, err := gh("repo", "clone", "x"); err == nil || !strings.Contains(stderr, "not faked") {
		t.Errorf("an unfaked call: err %v, stderr %q", err, stderr)
	}

	calls, err := readGHLog(log)
	want := []string{
		"pr create --title Fix port --body one\ntwo",
		"pr create --title Promote",
		"issue create --title Wrong port",
		"pr view " + fakeGHRepo + "/pull/101 --json state,mergedAt",
		"auth status",
		"repo clone x",
	}
	if err != nil || !slices.Equal(calls, want) {
		t.Errorf("log = %q (%v), want %q", calls, err, want)
	}
	if calls, err := readGHLog(filepath.Join(t.TempDir(), "none.log")); calls != nil || err != nil {
		t.Errorf("a missing log = %q, %v", calls, err)
	}
}
