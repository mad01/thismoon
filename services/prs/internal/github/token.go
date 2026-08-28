// Package github fetches open pull requests from GitHub hosts (github.com
// and GitHub Enterprise) over their GraphQL APIs. Authentication reuses the
// gh CLI's existing logins: a token is minted per host with
// `gh auth token --hostname <host>` and cached in memory, so no token ever
// lives in config or on disk, and gh is exec'd once per host instead of once
// per API call.
package github

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ghFallbackPaths are tried when gh is not on PATH — launchd agents run with
// a minimal PATH that misses the Homebrew prefix.
var ghFallbackPaths = []string{"/opt/homebrew/bin/gh", "/usr/local/bin/gh"}

// mintFailureTTL is how long a failed mint is remembered. Without it, a
// host gh is not logged into would exec gh once per repo per poll cycle;
// with it, once per minute.
const mintFailureTTL = time.Minute

// TokenSource mints and caches one API token per host. Safe for concurrent
// use by the poll workers.
type TokenSource struct {
	mu       sync.Mutex
	mint     func(host string) (string, error)
	now      func() time.Time
	tokens   map[string]string
	failures map[string]mintFailure
}

// mintFailure remembers a failed mint so it is not retried per repo.
type mintFailure struct {
	err error
	at  time.Time
}

// NewTokenSource returns a TokenSource backed by the gh CLI.
func NewTokenSource() *TokenSource {
	return &TokenSource{
		mint:     ghMint,
		now:      time.Now,
		tokens:   map[string]string{},
		failures: map[string]mintFailure{},
	}
}

// Token returns the cached token for host, minting one on first use. A mint
// failure is cached for mintFailureTTL and returned as-is until it expires.
func (t *TokenSource) Token(host string) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if tok, ok := t.tokens[host]; ok {
		return tok, nil
	}
	if f, ok := t.failures[host]; ok && t.now().Sub(f.at) < mintFailureTTL {
		return "", f.err
	}
	tok, err := t.mint(host)
	if err != nil {
		t.failures[host] = mintFailure{err: err, at: t.now()}
		return "", err
	}
	delete(t.failures, host)
	t.tokens[host] = tok
	return tok, nil
}

// Invalidate drops the cached token (and any cached failure) for host,
// forcing a re-mint on the next Token call — the recovery path for a 401
// after a token expired or was revoked mid-flight.
func (t *TokenSource) Invalidate(host string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.tokens, host)
	delete(t.failures, host)
}

// ghMint shells out to `gh auth token --hostname <host>` and returns the
// trimmed token. A failure surfaces gh's stderr, which names the fix
// (`gh auth login --hostname <host>`).
func ghMint(host string) (string, error) {
	bin, err := ghPath()
	if err != nil {
		return "", err
	}
	out, err := exec.Command(bin, "auth", "token", "--hostname", host).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("gh auth token --hostname %s: %s",
				host, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("gh auth token --hostname %s: %w", host, err)
	}
	tok := strings.TrimSpace(string(out))
	if tok == "" {
		return "", fmt.Errorf("gh auth token --hostname %s returned an empty token", host)
	}
	return tok, nil
}

// GhAvailable reports whether the gh binary is resolvable, for the doctor.
func GhAvailable() error {
	_, err := ghPath()
	return err
}

// ghPath resolves the gh binary: PATH first, then the Homebrew locations.
func ghPath() (string, error) {
	if p, err := exec.LookPath("gh"); err == nil {
		return p, nil
	}
	for _, p := range ghFallbackPaths {
		if _, err := exec.LookPath(p); err == nil {
			return p, nil
		}
	}
	return "", errors.New("gh not found on PATH (install GitHub CLI: brew install gh)")
}
