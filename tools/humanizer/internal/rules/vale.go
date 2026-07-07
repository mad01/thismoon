package rules

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// defaultValeTimeout bounds a single vale invocation. The MCP SDK does
// not guarantee a deadline on incoming tool calls, so we supply one.
const defaultValeTimeout = 30 * time.Second

// Finding is a single vale alert, remapped into humanizer-flavored fields.
type Finding struct {
	RuleID   string `json:"rule_id"`
	Name     string `json:"rule_name,omitempty"`
	Category string `json:"category,omitempty"`
	Severity string `json:"severity"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Match    string `json:"match,omitempty"`
	Message  string `json:"message"`
	Link     string `json:"link,omitempty"`
}

// DetectOptions tunes a Detect call.
type DetectOptions struct {
	// CacheDir is where the Vale style pack gets extracted. If empty,
	// DefaultCacheDir() is used.
	CacheDir string
	// ValeBinary overrides the path to the `vale` CLI. Empty = "vale"
	// on $PATH.
	ValeBinary string
	// Extension tells vale which format to parse. Default ".md".
	Extension string
	// Rules restricts results to these rule IDs (full names like
	// "Humanizer.AIVocabulary"). Empty means no filter.
	Rules []string
	// MinSeverity filters findings below this level (suggestion|warning|error).
	// Empty means no filter.
	MinSeverity string
	// Timeout bounds a single vale invocation. Zero uses defaultValeTimeout.
	Timeout time.Duration
}

// Status describes the state of the `vale` binary we depend on.
type Status struct {
	Installed   bool   `json:"installed"`
	Binary      string `json:"binary"`
	Version     string `json:"version,omitempty"`
	CacheDir    string `json:"cache_dir"`
	RuleCount   int    `json:"rule_count"`
	StylePack   string `json:"style_pack"`
	Error       string `json:"error,omitempty"`
	InstallHint string `json:"install_hint,omitempty"`
}

// CheckStatus reports whether vale is installed, its version, and the
// state of the cached style pack. Mirrors Vale-MCP's `vale_status` tool.
func CheckStatus(ctx context.Context, opts DetectOptions) Status {
	bin := binaryOrDefault(opts.ValeBinary)
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		if cd, err := DefaultCacheDir(); err == nil {
			cacheDir = cd
		}
	}
	s := Status{
		Binary:    bin,
		CacheDir:  cacheDir,
		RuleCount: len(All()),
		StylePack: "Humanizer",
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		s.Error = err.Error()
		s.InstallHint = "brew install vale — or: GOBIN=$HOME/code/bin go install github.com/errata-ai/vale/v3/cmd/vale@latest"
		return s
	}
	s.Binary = path
	s.Installed = true
	cmd := exec.CommandContext(ctx, bin, "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err == nil {
		s.Version = strings.TrimSpace(out.String())
	}
	return s
}

// Detect runs vale on text and returns findings annotated with metadata
// from the embedded rule pack (category + friendly name).
//
// Failure modes:
//   - Vale not installed: returns *ValeNotInstalledError (stable type so
//     callers can surface a good error).
//   - Pack extraction failed: wrapped os error.
//   - Vale process failed: wraps the process error, including stderr.
func Detect(ctx context.Context, text string, opts DetectOptions) ([]Finding, error) {
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	ext := opts.Extension
	if ext == "" {
		ext = ".md"
	}
	// Vale is most reliable linting a real file (stdin + --ext or --path
	// is brittle across versions). Stage a temp file with the right
	// extension, lint it, clean up.
	tmp, err := os.CreateTemp("", "humanizer-*"+ext)
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return nil, fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("close temp file: %w", err)
	}
	return runVale(ctx, tmp.Name(), opts)
}

// DetectFile is like Detect but lints a real file on disk instead of
// a text blob. Mirrors Vale-MCP's `check_file`. The file is passed
// directly to vale so its format detection uses the real extension.
func DetectFile(ctx context.Context, filePath string, opts DetectOptions) ([]Finding, error) {
	if strings.TrimSpace(filePath) == "" {
		return nil, errors.New("rules: file path is required")
	}
	if _, err := os.Stat(filePath); err != nil {
		return nil, fmt.Errorf("stat %s: %w", filePath, err)
	}
	return runVale(ctx, filePath, opts)
}

// runVale is the shared backbone for Detect / DetectFile. It ensures the
// style pack is extracted, resolves the vale binary, applies a timeout,
// runs vale on the given file path, and filters+enriches the result.
func runVale(ctx context.Context, filePath string, opts DetectOptions) ([]Finding, error) {
	cacheDir := opts.CacheDir
	if cacheDir == "" {
		cd, err := DefaultCacheDir()
		if err != nil {
			return nil, err
		}
		cacheDir = cd
	}
	if err := EnsurePack(cacheDir); err != nil {
		return nil, fmt.Errorf("extract vale pack: %w", err)
	}

	bin := binaryOrDefault(opts.ValeBinary)
	if _, err := exec.LookPath(bin); err != nil {
		return nil, &ValeNotInstalledError{Binary: bin}
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultValeTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cfg := filepath.Join(cacheDir, ".vale.ini")
	cmd := exec.CommandContext(
		ctx, bin,
		"--config", cfg,
		"--no-wrap",
		"--output=JSON",
		filePath,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	// Vale exits non-zero when it finds alerts at >= its min level. That's
	// not a failure — we still get JSON on stdout. Only treat the run as
	// failed when stdout is empty (no JSON to parse).
	if err != nil && stdout.Len() == 0 {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("vale: %w: %s", err, strings.TrimSpace(stderr.String()))
		}
		return nil, fmt.Errorf("vale: %w", err)
	}
	findings, err := parseValeJSON(stdout.Bytes())
	if err != nil {
		return nil, fmt.Errorf(
			"parse vale output: %w (stderr: %s)",
			err,
			strings.TrimSpace(stderr.String()),
		)
	}
	return filterAndEnrich(findings, opts), nil
}

func binaryOrDefault(bin string) string {
	if bin == "" {
		return "vale"
	}
	return bin
}

// valeAlert matches vale's --output=JSON payload shape.
type valeAlert struct {
	Check       string `json:"Check"`
	Description string `json:"Description"`
	Line        int    `json:"Line"`
	Link        string `json:"Link"`
	Match       string `json:"Match"`
	Message     string `json:"Message"`
	Severity    string `json:"Severity"`
	Span        []int  `json:"Span"`
}

// parseValeJSON accepts either the map shape vale emits for files
//
//	{"stdin.md": [ {alert...}, ... ]}
//
// or an empty object (no findings) or an empty array.
func parseValeJSON(data []byte) ([]Finding, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("{}")) ||
		bytes.Equal(trimmed, []byte("[]")) {
		return nil, nil
	}
	// Most common: top-level map of filename -> alerts.
	byFile := map[string][]valeAlert{}
	if err := json.Unmarshal(data, &byFile); err != nil {
		// Fallback: some Vale versions emit a plain array when reading stdin.
		var flat []valeAlert
		if err2 := json.Unmarshal(data, &flat); err2 == nil {
			return alertsToFindings(flat), nil
		}
		return nil, err
	}
	var alerts []valeAlert
	for _, as := range byFile {
		alerts = append(alerts, as...)
	}
	return alertsToFindings(alerts), nil
}

func alertsToFindings(alerts []valeAlert) []Finding {
	out := make([]Finding, 0, len(alerts))
	for _, a := range alerts {
		col := 0
		if len(a.Span) > 0 {
			col = a.Span[0]
		}
		out = append(out, Finding{
			RuleID:   a.Check,
			Severity: a.Severity,
			Line:     a.Line,
			Column:   col,
			Match:    a.Match,
			Message:  a.Message,
			Link:     a.Link,
		})
	}
	return out
}

// filterAndEnrich drops findings that don't match the caller's filter
// and annotates the rest with humanizer rule metadata (friendly name +
// category). The metadata map is fetched once from the rules registry —
// cheap, since it's cached across calls.
func filterAndEnrich(findings []Finding, opts DetectOptions) []Finding {
	allowID := map[string]struct{}{}
	for _, id := range opts.Rules {
		allowID[id] = struct{}{}
	}
	minRank := severityRank(opts.MinSeverity)
	meta := byID()
	out := make([]Finding, 0, len(findings))
	for _, f := range findings {
		if len(allowID) > 0 {
			if _, ok := allowID[f.RuleID]; !ok {
				continue
			}
		}
		if severityRank(f.Severity) < minRank {
			continue
		}
		if m, ok := meta[f.RuleID]; ok {
			if f.Name == "" {
				f.Name = m.Name
			}
			if f.Category == "" {
				f.Category = m.Category
			}
		}
		out = append(out, f)
	}
	return out
}

// severityRank orders vale's levels. "" (unset) matches anything >= suggestion.
func severityRank(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "suggestion", "info", "":
		return 1
	case "warning", "warn":
		return 2
	case "error", "err":
		return 3
	}
	return 0
}

// ValeNotInstalledError is returned by Detect when the `vale` binary is
// not on PATH. Callers should surface a clear message pointing the user
// at the claude-mcp install hook.
type ValeNotInstalledError struct{ Binary string }

func (e *ValeNotInstalledError) Error() string {
	return fmt.Sprintf(
		"vale binary %q not found on PATH; install with `brew install vale` or `go install github.com/errata-ai/vale/v3/cmd/vale@latest`",
		e.Binary,
	)
}
