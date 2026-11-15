package run

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/mad01/thismoon/tools/harvest/internal/reply"
	"github.com/mad01/thismoon/tools/harvest/internal/state"
)

// captureTimeout bounds one `brain capture`. A write and its duplicate
// check take seconds; one past this has hung, and nobody is watching.
const captureTimeout = 2 * time.Minute

// The `brain capture` exit codes apply tells apart. Any other failure is
// reported as failed.
const (
	brainSecretFound     = 8
	brainLikelyDuplicate = 9
)

// ApplyConfig is what Apply needs besides the session id.
type ApplyConfig struct {
	State *state.Dir
	// Brain is the brain binary; a bare name is looked up on PATH.
	Brain string
	// Sandboxed says whether this step runs under srt, for the done line.
	Sandboxed bool
}

// Apply is the apply step. It parses reply.txt, writes each capture with
// `brain capture`, moves the session's mark to the line count prepare
// recorded, and logs the done line with the report under it. It fails, and
// moves no mark, when the reply does not parse, or when there were captures
// and every one of them failed.
func Apply(ctx context.Context, cfg ApplyConfig, session string) error {
	st := cfg.State
	job, err := st.Job(session)
	if err != nil {
		return err
	}
	counts, err := job.Counts()
	if err != nil {
		return err
	}
	call, err := readCall(job.Call())
	if err != nil {
		return err
	}
	text, err := os.ReadFile(job.Reply())
	if err != nil {
		return fmt.Errorf("run: read the reply: %w", err)
	}
	plan, err := reply.Parse(string(text))
	if err != nil {
		return err
	}
	report, err := writeAll(ctx, cfg.Brain, session, plan)
	if err != nil {
		return err
	}
	if err := st.SetMark(session, counts.Lines); err != nil {
		return err
	}
	return st.LogBlock(fmt.Sprintf("done %s: model=%s digest=%dB cost=%s backend=%s sandbox=%s",
		session, orUnknown(call.Model), counts.DigestBytes, cost(call.CostUSD),
		orUnknown(call.Backend), onOff(cfg.Sandboxed)), report)
}

// writeAll writes the plan's captures and returns the report: one line per
// capture, "captured: none" when there were none, one line per capture
// Parse dropped, then how many candidates the model dropped, with their
// reasons indented under it. A candidate's own text never reaches the
// report: the model drops secrets, so a candidate can hold one.
func writeAll(ctx context.Context, brain, session string, plan reply.Plan) (string, error) {
	var lines []string
	var failures []string
	for _, c := range plan.Captures {
		line, failed := write(ctx, brain, session, c)
		lines = append(lines, line)
		if failed {
			failures = append(failures, line)
		}
	}
	if len(plan.Captures) == 0 {
		lines = append(lines, "captured: none")
	}
	for _, d := range plan.Dropped {
		lines = append(lines, fmt.Sprintf("dropped: %s: %s", d.Reason, d.Title))
	}
	lines = append(lines, declined(plan.Declined)...)
	if n := len(plan.Captures); n > 0 && len(failures) == n {
		return "", fmt.Errorf("run: all %d captures failed; the first %s", n, failures[0])
	}
	return strings.Join(lines, "\n"), nil
}

// declined is the report of the model's drops: a count line, then each
// reason the plan kept, indented. It is nothing when the model dropped none.
func declined(d reply.Declined) []string {
	if d.Count == 0 {
		return nil
	}
	head := fmt.Sprintf("dropped by the model: %d", d.Count)
	if len(d.Reasons) < d.Count {
		head += fmt.Sprintf(" (the first %d reasons)", len(d.Reasons))
	}
	lines := []string{head}
	for _, r := range d.Reasons {
		lines = append(lines, "  "+r)
	}
	return lines
}

// write runs one `brain capture` and returns its report line, and whether
// it failed for a reason other than a refusal. Every value goes as one
// --flag=value argument through exec, never through a shell, so a title
// that starts with - stays a title. The body goes on stdin.
func write(ctx context.Context, brain, session string, c reply.Capture) (string, bool) {
	args := []string{"capture", "--title=" + c.Title}
	for _, t := range c.Tags {
		args = append(args, "--tag="+t)
	}
	args = append(args, "--by=agent:"+session)
	if c.Source != "" {
		args = append(args, "--source="+c.Source)
	}
	args = append(args, "-o", "json")

	ctx, cancel := context.WithTimeout(ctx, captureTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, brain, args...)
	cmd.Stdin = strings.NewReader(c.Body)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()

	var exitErr *exec.ExitError
	code := -1
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	}
	switch {
	case err == nil:
		return captured(stdout.Bytes(), c.Title), false
	case code == brainLikelyDuplicate:
		return "dropped: likely duplicate: " + c.Title, false
	case code == brainSecretFound:
		// brain scans the whole note, title included, so the title can be
		// where the secret is.
		return "dropped: secret found", false
	default:
		return "failed: " + c.Title + ": " + firstLine(stderr.String(), err), true
	}
}

// captured is the report line for a written note: its vault and path from
// brain's JSON, or the title when brain printed none.
func captured(out []byte, title string) string {
	var res struct {
		Vault string `json:"vault"`
		Path  string `json:"path"`
	}
	if err := json.Unmarshal(out, &res); err != nil || res.Path == "" {
		return "captured: " + title
	}
	return "captured: " + res.Vault + ":" + res.Path
}

// readCall reads call.json.
func readCall(path string) (Call, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Call{}, fmt.Errorf("run: read the call: %w", err)
	}
	var c Call
	if err := json.Unmarshal(b, &c); err != nil {
		return Call{}, fmt.Errorf("run: decode the call: %w", err)
	}
	return c, nil
}

// firstLine is the first non-empty line of s, or err's text when s has
// none.
func firstLine(s string, err error) string {
	for line := range strings.SplitSeq(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return err.Error()
}

// cost renders a cost in USD; zero is a backend that reported none.
func cost(usd float64) string {
	if usd == 0 {
		return "?"
	}
	return strconv.FormatFloat(usd, 'f', -1, 64)
}

func onOff(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

func orUnknown(s string) string {
	if s == "" {
		return "?"
	}
	return s
}
