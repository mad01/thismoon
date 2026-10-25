// Package recall ranks stored assertions against a free-form question with a
// one-shot model judge. It is deliberately not semantic search: no embeddings,
// no index. The whole store rides in one prompt and an isolated `claude -p`
// haiku call replies with the relevant assertion ids — at the store's current
// size the model judging everything beats building a retrieval pipeline
// (MAD-265).
//
// The judge runs in the client, never in serve. The kof CLI and the MCP shim
// fetch the candidates over the serve list API and rank them in their own
// process, with the user's environment and Claude credentials. serve runs
// sandboxed and keeps no model dependency.
package recall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/keeper-of-facts/internal/client"
	"github.com/mad01/thismoon/services/keeper-of-facts/internal/store"
)

// MaxResults caps how many assertions one recall returns; past a handful the
// ranking tail is noise.
const MaxResults = 5

// timeout bounds the judge call. A one-shot haiku ranking takes seconds; a
// hang must not pin the CLI or the MCP tool call forever.
const timeout = 30 * time.Second

// systemPrompt pins the judge to a machine-readable reply. The id list is the
// entire contract — any prose would just be stripped.
const systemPrompt = "You rank stored assertions by relevance to a question. " +
	"Reply with ONLY a JSON array of assertion ids, most relevant first, " +
	"at most %d ids. Include only assertions that genuinely bear on the " +
	"question; an empty array is a valid answer. No prose, no explanation."

// ErrNoQuestion rejects an empty question before anything is fetched or judged.
var ErrNoQuestion = errors.New("kof: recall needs a question")

// Lister fetches assertions from serve. *client.Client satisfies it; recall
// asks for the whole store, unfiltered, as the candidate set.
type Lister interface {
	List(subject, kind, status string) ([]client.Assertion, error)
}

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

// Recall fetches every assertion from src and returns, in rank order, the ones
// the judge finds relevant to question, at most MaxResults. Retracted
// assertions never reach the judge. Stale ones do, because a stale claim about
// the code being asked about is still the store's best signal. Every judge failure
// wraps a hint to fall back to kof_query: recall degrading must never look
// like "the store knows nothing".
func (j *Judge) Recall(
	ctx context.Context,
	src Lister,
	question string,
) ([]client.Assertion, error) {
	if strings.TrimSpace(question) == "" {
		return nil, ErrNoQuestion
	}
	all, err := src.List("", "", "")
	if err != nil {
		return nil, fmt.Errorf("kof: list recall candidates: %w", err)
	}
	candidates := live(all)
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

// live drops retracted assertions: a withdrawn claim is never an answer.
func live(as []client.Assertion) []client.Assertion {
	out := make([]client.Assertion, 0, len(as))
	for _, a := range as {
		if a.Status != store.StatusRetracted {
			out = append(out, a)
		}
	}
	return out
}

// buildPrompt lays out the question and one line per assertion. The judge
// sees id, status, subject, and statement — enough to rank, nothing else.
func buildPrompt(question string, as []client.Assertion) string {
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
func pick(as []client.Assertion, ids []string) []client.Assertion {
	byID := make(map[string]client.Assertion, len(as))
	for _, a := range as {
		byID[a.ID] = a
	}
	var out []client.Assertion
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

// sessionEnv names the variables Claude Code sets on the processes it spawns,
// the kof MCP shim included, to tie them to its session. The judge drops them
// so it runs as its own one-shot process instead of joining the caller's
// session or its messaging channel.
var sessionEnv = map[string]bool{
	"CLAUDECODE":                   true,
	"CLAUDE_CODE_ENTRYPOINT":       true,
	"CLAUDE_CODE_SESSION_ID":       true,
	"CLAUDE_CODE_CHILD_SESSION":    true,
	"CLAUDE_CODE_SESSION_ATTENDED": true,
	"CLAUDE_CODE_MESSAGING_SOCKET": true,
	"CLAUDE_CODE_MESSAGING_TOKEN":  true,
	"CLAUDE_PROJECT_DIR":           true,
	"CLAUDE_PID":                   true,
}

// judgeEnv returns environ without the session-linking variables. Everything
// else passes through, credentials and provider settings included.
func judgeEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if !sessionEnv[name] {
			out = append(out, kv)
		}
	}
	return out
}

// claudeResult is the subset of `claude -p --output-format json` output the
// runner reads.
type claudeResult struct {
	Result string `json:"result"`
}

// claudeBin resolves the claude CLI. The CLI and MCP shim usually inherit a
// PATH that has it, but an agent may spawn the shim with a minimal PATH, so
// the install location under ~/.local/bin is the fallback.
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
// session persistence, no link to the calling session, run from the temp dir
// so repo-scoped session hooks stay quiet. The model sees the prompts and
// nothing else.
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
	cmd.Env = judgeEnv(os.Environ())
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
