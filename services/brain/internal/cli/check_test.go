package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/check"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

func TestCheckCommand(t *testing.T) {
	root := setupCLI(t, "")
	clean := runBrain(t, "", "check")
	if clean.code != exitOK || !strings.Contains(clean.stdout, "checked 4 note(s): 0 error(s)") {
		t.Fatalf(
			"check a clean vault: exit %d, stdout %q, stderr %q",
			clean.code,
			clean.stdout,
			clean.stderr,
		)
	}
	if got := runBrain(t, "", "retract", "notes/beta.md", "--reason", "error"); got.code != exitOK {
		t.Fatalf("retract: exit %d, stderr %q", got.code, got.stderr)
	}

	text := runBrain(t, "", "check", "notes/alpha.md")
	wantLine := "notes/alpha.md:10: error [link-revoked] links [[beta]] (default:notes/beta.md), which is retracted"
	if text.code != exitError || !strings.Contains(text.stdout, wantLine) {
		t.Errorf(
			"check a revoked citation: exit %d, stdout %q, want %q",
			text.code,
			text.stdout,
			wantLine,
		)
	}

	gh := runBrain(t, "", "check", filepath.Join(root, "notes", "alpha.md"), "-o", "github")
	wantAnnotation := "::error file=notes/alpha.md,line=10,title=brain check link-revoked::links [[beta]]"
	if gh.code != exitError || !strings.Contains(gh.stdout, wantAnnotation) {
		t.Errorf(
			"check -o github by file path: exit %d, stdout %q, want %q",
			gh.code,
			gh.stdout,
			wantAnnotation,
		)
	}

	js := runBrain(t, "", "check", "notes/alpha.md", "notes/gone.md", "-o", "json")
	r := decodeStrict[check.Report](t, js.stdout)
	if js.code != exitError || r.OK || r.Checked != 1 || r.Errors != 1 {
		t.Errorf("check -o json: exit %d, report %+v", js.code, r)
	}
	if len(r.Skipped) != 1 || r.Skipped[0].Path != "notes/gone.md" ||
		!strings.Contains(js.stderr, "skipped notes/gone.md: no such note") {
		t.Errorf("a deleted path: skipped %+v, stderr %q", r.Skipped, js.stderr)
	}

	for _, args := range [][]string{
		{"check", "-o", "yaml"},
		{"search", "kubernetes", "-o", "github"},
	} {
		if got := runBrain(t, "", args...); got.code != exitUsage {
			t.Errorf("%v: exit %d, want %d", args, got.code, exitUsage)
		}
	}
}

func TestWriteAndEditWarnAboutCitations(t *testing.T) {
	setupCLI(t, "")
	write := runBrain(
		t,
		"",
		"write",
		"--tier",
		"notes",
		"--title",
		"Cites",
		"--body",
		"See [[raw]].",
		"-o",
		"json",
	)
	res := decodeStrict[vault.PathResult](t, write.stdout)
	if write.code != exitOK || len(res.Warnings) != 1 ||
		!strings.Contains(res.Warnings[0], "which is raw") {
		t.Fatalf("write citing a raw note: exit %d, warnings %q", write.code, res.Warnings)
	}
	if !strings.Contains(write.stderr, "warning: line ") ||
		!strings.Contains(write.stderr, "links [[raw]]") {
		t.Errorf("write stderr = %q, want the warning", write.stderr)
	}

	if got := runBrain(t, "", "retract", "notes/beta.md", "--reason", "error"); got.code != exitOK {
		t.Fatalf("retract: exit %d, stderr %q", got.code, got.stderr)
	}
	edit := runBrain(t, "", "edit", res.Path, "--append", "Also [[beta]].", "-o", "json")
	res = decodeStrict[vault.PathResult](t, edit.stdout)
	if edit.code != exitOK || len(res.Warnings) != 2 ||
		!strings.Contains(res.Warnings[1], "which is retracted") {
		t.Errorf("edit citing a retracted note: exit %d, warnings %q", edit.code, res.Warnings)
	}

	quiet := runBrain(t, "", "edit", res.Path, "--tag", "x", "-o", "json")
	if res = decodeStrict[vault.PathResult](t, quiet.stdout); quiet.code != exitOK ||
		len(res.Warnings) != 0 ||
		quiet.stderr != "" {
		t.Errorf(
			"a frontmatter-only edit warned: exit %d, warnings %q, stderr %q",
			quiet.code,
			res.Warnings,
			quiet.stderr,
		)
	}

	plain := runBrain(t, "", "write", "--tier", "inbox", "--title", "Plain", "--body", "No links.")
	if plain.code != exitOK || plain.stderr != "" {
		t.Errorf("write without links: exit %d, stderr %q", plain.code, plain.stderr)
	}
}
