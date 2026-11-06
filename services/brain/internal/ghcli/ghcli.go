// Package ghcli runs the GitHub CLI, gh, for the steps of sharing a note
// that git alone cannot take: opening a pull request, asking what became of
// one, and opening an issue. gh brings its own login; brain holds no token.
package ghcli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// timeout bounds one gh call, which talks to GitHub.
const timeout = 2 * time.Minute

// ErrNotInstalled means gh is not on PATH.
var ErrNotInstalled = errors.New(
	"gh, the GitHub CLI, is not on PATH (install it from https://cli.github.com, then gh auth login)",
)

// Pull request states as gh pr view reports them.
const (
	StateOpen   = "OPEN"
	StateMerged = "MERGED"
	StateClosed = "CLOSED"
)

// PR is a pull request to open.
type PR struct {
	// Repo is HOST/OWNER/REPO or OWNER/REPO; empty lets gh read it from
	// the remotes of the directory it runs in.
	Repo  string
	Head  string
	Base  string
	Title string
	Body  string
}

// CreatePR opens pr with gh pr create, run in dir, and returns its URL.
func CreatePR(ctx context.Context, dir string, pr PR) (string, error) {
	args := []string{
		"pr", "create", "--head", pr.Head, "--base", pr.Base,
		"--title", pr.Title, "--body", pr.Body,
	}
	if pr.Repo != "" {
		args = append(args, "--repo", pr.Repo)
	}
	out, err := run(ctx, dir, args...)
	if err != nil {
		return "", err
	}
	return lastURL(out)
}

// PRStatus is what gh pr view reports about a pull request.
type PRStatus struct {
	State    string `json:"state"`
	MergedAt string `json:"mergedAt"`
}

// ViewPR asks gh for the state of the pull request at url.
func ViewPR(ctx context.Context, url string) (PRStatus, error) {
	out, err := run(ctx, "", "pr", "view", url, "--json", "state,mergedAt")
	if err != nil {
		return PRStatus{}, err
	}
	var s PRStatus
	if err := json.Unmarshal([]byte(out), &s); err != nil {
		return PRStatus{}, fmt.Errorf("gh pr view %s printed %q: %w", url, out, err)
	}
	s.State = strings.ToUpper(s.State)
	return s, nil
}

// CreateIssue opens an issue with gh issue create, run in dir, and returns
// its URL. repo is as in PR.
func CreateIssue(ctx context.Context, dir, repo, title, body string) (string, error) {
	args := []string{"issue", "create", "--title", title, "--body", body}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	out, err := run(ctx, dir, args...)
	if err != nil {
		return "", err
	}
	return lastURL(out)
}

// remoteRepo matches a GitHub-style remote URL: scp-like
// git@host:owner/repo, or ssh, https or git URLs, with or without .git.
var remoteRepo = regexp.MustCompile(
	`^(?:(?:https?|ssh|git)://(?:[^@/]+@)?([^/:]+)(?::\d+)?/|[^@/]+@([^:/]+):)([^/]+)/([^/]+?)(?:\.git)?/?$`,
)

// RepoOf returns HOST/OWNER/REPO for a remote URL, OWNER/REPO on
// github.com, or "" for a URL that names no such repo (a local path).
func RepoOf(remoteURL string) string {
	m := remoteRepo.FindStringSubmatch(strings.TrimSpace(remoteURL))
	if m == nil {
		return ""
	}
	host := m[1] + m[2]
	if host == "github.com" {
		return m[3] + "/" + m[4]
	}
	return host + "/" + m[3] + "/" + m[4]
}

// run runs gh with args in dir and returns its trimmed stdout. gh never
// prompts: a missing login fails instead of waiting on a terminal.
func run(ctx context.Context, dir string, args ...string) (string, error) {
	bin, err := exec.LookPath("gh")
	if err != nil {
		return "", ErrNotInstalled
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1")
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		name := "gh " + strings.Join(args[:min(2, len(args))], " ")
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return "", fmt.Errorf("%s: %w", name, err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// lastURL returns the last line of out that is a URL, which is where gh
// prints what it created.
func lastURL(out string) (string, error) {
	lines := strings.Split(out, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "https://") || strings.HasPrefix(line, "http://") {
			return line, nil
		}
	}
	return "", fmt.Errorf("gh printed no URL: %q", out)
}
