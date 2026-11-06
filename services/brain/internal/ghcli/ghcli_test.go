package ghcli_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/ghcli"
	"github.com/mad01/thismoon/services/brain/internal/testgh"
)

func TestRepoOf(t *testing.T) {
	cases := map[string]string{
		"git@github.com:example-org/brain-shared.git":       "example-org/brain-shared",
		"https://github.com/example-org/brain-shared":       "example-org/brain-shared",
		"https://github.com/example-org/brain-shared.git/":  "example-org/brain-shared",
		"ssh://git@github.com/example-org/brain-shared.git": "example-org/brain-shared",
		"https://git.example.com/team/notes.git":            "git.example.com/team/notes",
		"ssh://git@git.example.com:2222/team/notes":         "git.example.com/team/notes",
		"/tmp/remote.git":        "",
		"file:///tmp/remote.git": "",
	}
	for in, want := range cases {
		if got := ghcli.RepoOf(in); got != want {
			t.Errorf("RepoOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCallsGoToTheFake(t *testing.T) {
	fake := testgh.Install(t, "https://github.com/example-org/brain-shared/pull/7",
		"https://github.com/example-org/brain-shared/issues/3")
	ctx := context.Background()
	url, err := ghcli.CreatePR(ctx, t.TempDir(), ghcli.PR{
		Repo: "example-org/brain-shared", Head: "brain/promote/x", Base: "main",
		Title: "Promote note: X", Body: "line one\nline two",
	})
	if err != nil || url != "https://github.com/example-org/brain-shared/pull/7" {
		t.Fatalf("CreatePR() = %q, %v", url, err)
	}
	fake.PRState("7", ghcli.StateMerged)
	status, err := ghcli.ViewPR(ctx, url)
	if err != nil || status.State != ghcli.StateMerged || status.MergedAt == "" {
		t.Fatalf("ViewPR() = %+v, %v", status, err)
	}
	if _, err := ghcli.ViewPR(ctx, "https://github.com/example-org/brain-shared/pull/8"); err == nil {
		t.Error("ViewPR of an unknown pull request succeeded")
	}
	calls := fake.Calls()
	if len(calls) != 3 || testgh.Flag(calls[0], "--body") != "line one\nline two" ||
		testgh.Flag(calls[0], "--repo") != "example-org/brain-shared" {
		t.Errorf("calls = %q", calls)
	}

	t.Setenv("PATH", t.TempDir())
	if _, err := ghcli.ViewPR(ctx, url); !errors.Is(err, ghcli.ErrNotInstalled) {
		t.Errorf("without gh: %v, want ErrNotInstalled", err)
	}
}
