// Package recall ranks stored assertions against a free-form question with a
// one-shot model judge. It is deliberately not semantic search: no embeddings,
// no index. The whole store rides in one prompt and an isolated `claude -p`
// haiku call replies with the relevant assertion ids — at the store's current
// size the model judging everything beats building a retrieval pipeline
// (MAD-265).
package recall

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/keeper-of-facts/internal/store"
)

// MaxResults caps how many assertions one recall returns; past a handful the
// ranking tail is noise.
const MaxResults = 5

// timeout bounds the judge call. A one-shot haiku ranking takes seconds; a
// hang must not pin the serve request forever.
const timeout = 30 * time.Second

// systemPrompt pins the judge to a machine-readable reply. The id list is the
// entire contract — any prose would just be stripped.
const systemPrompt = "You rank stored assertions by relevance to a question. " +
	"Reply with ONLY a JSON array of assertion ids, most relevant first, " +
	"at most %d ids. Include only assertions that genuinely bear on the " +
	"question; an empty array is a valid answer. No prose, no explanation."

// Judge runs the ranking. The zero value is not usable; NewJudge wires the
// real `claude` exec, and tests inject a fake runner.
type Judge struct {
	// run executes the judge with the given system prompt and user prompt,
	// returning the model's raw text reply.
	run func(ctx context.Context, system, prompt string) (string, error)
}

// NewJudge returns a Judge backed by the claude CLI.
func NewJudge() *Judge {
	return &Judge{run: runClaude}
}

// NewJudgeWithRunner returns a Judge with a custom runner, for tests.
func NewJudgeWithRunner(
	run func(ctx context.Context, system, prompt string) (string, error),
) *Judge {
	return &Judge{run: run}
}

// Rank asks the judge which assertions bear on the question and returns them
// in rank order. Retracted assertions never reach the judge; stale ones do —
// a stale claim about code being asked about is still the store's best
// signal. Every failure wraps a hint to fall back to kof_query: recall
// degrading must never look like "the store knows nothing".
func (j *Judge) Rank(
	ctx context.Context,
	question string,
	as []store.Assertion,
) ([]store.Assertion, error) {
	candidates := make([]store.Assertion, 0, len(as))
	for _, a := range as {
		if a.Status != store.StatusRetracted {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	raw, err := j.run(ctx, fmt.Sprintf(systemPrompt, MaxResults), buildPrompt(question, candidates))
	if err != nil {
		return nil, fmt.Errorf("kof: recall judge failed (fall back to kof_query): %w", err)
	}
	ids, err := parseIDs(raw)
	if err != nil {
		return nil, fmt.Errorf("kof: recall reply unparseable (fall back to kof_query): %w", err)
	}
	return pick(candidates, ids), nil
}

// buildPrompt lays out the question and one line per assertion. The judge
// sees id, status, subject, and statement — enough to rank, nothing else.
func buildPrompt(question string, as []store.Assertion) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Question: %q\n\nAssertions:\n", question)
	for _, a := range as {
		fmt.Fprintf(&b, "- id=%s [%s] subject=%s: %s\n", a.ID, a.Status, a.Subject, a.Statement)
	}
	return b.String()
}

// parseIDs reads the judge's reply as a JSON string array, tolerating the
// markdown code fence haiku tends to wrap JSON in.
func parseIDs(raw string) ([]string, error) {
	s := strings.TrimSpace(raw)
	if after, ok := strings.CutPrefix(s, "```"); ok {
		after = strings.TrimPrefix(after, "json")
		if body, _, found := strings.Cut(after, "```"); found {
			s = strings.TrimSpace(body)
		}
	}
	var ids []string
	if err := json.Unmarshal([]byte(s), &ids); err != nil {
		return nil, fmt.Errorf("parse id array from %q: %w", clip(raw), err)
	}
	return ids, nil
}

// pick maps ranked ids back to assertions, dropping ids the judge invented
// and capping the result. Order is the judge's ranking, not store order.
func pick(as []store.Assertion, ids []string) []store.Assertion {
	byID := make(map[string]store.Assertion, len(as))
	for _, a := range as {
		byID[a.ID] = a
	}
	var out []store.Assertion
	for _, id := range ids {
		if a, ok := byID[id]; ok {
			out = append(out, a)
			delete(byID, id) // a duplicated id must not duplicate the assertion
		}
		if len(out) == MaxResults {
			break
		}
	}
	return out
}

// clip bounds an error-message excerpt of the raw reply.
func clip(s string) string {
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

// claudeResult is the subset of `claude -p --output-format json` output the
// runner reads.
type claudeResult struct {
	Result string `json:"result"`
}

// claudeBin resolves the claude CLI. serve runs as a launchd agent with a
// minimal PATH that does not include the user's shell locations, so a plain
// LookPath fails there; the install location under ~/.local/bin is the
// fallback.
func claudeBin() string {
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "claude" // let exec fail with the PATH error
	}
	return filepath.Join(home, ".local", "bin", "claude")
}

// runClaude execs an isolated one-shot judge: no tools, no MCP servers, no
// session persistence, run from the temp dir so repo-scoped session hooks
// stay quiet. The model sees the prompts and nothing else.
func runClaude(ctx context.Context, system, prompt string) (string, error) {
	cmd := exec.CommandContext(
		ctx, claudeBin(),
		"-p",
		"--model", "haiku",
		"--tools", "",
		"--strict-mcp-config",
		"--no-session-persistence",
		"--output-format", "json",
		"--system-prompt", system,
		prompt,
	)
	cmd.Dir = os.TempDir()
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("exec claude -p: %w", err)
	}
	var res claudeResult
	if err := json.Unmarshal(out, &res); err != nil {
		return "", fmt.Errorf("decode claude -p output: %w", err)
	}
	return res.Result, nil
}
