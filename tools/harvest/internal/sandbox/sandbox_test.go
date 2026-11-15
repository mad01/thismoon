package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/kit/llm"
)

// parent is a calling environment with secrets that no step but complete
// may see, and one that no step under the allow-list may see.
var parent = []string{
	"HOME=/home/u",
	"PATH=/home/u/code/bin:/usr/bin",
	"TMPDIR=/tmp/u",
	"GITHUB_TOKEN=gh-secret",
	"LITELLM_BASE_URL=https://user:pw@proxy.example.com:4000/v1?x=1#f", // suspenders:ignore a made-up URL
	"LITELLM_API_KEY=lite-secret",
	"OPENROUTER_API_KEY=or-secret",
	"HARVEST_STATE_DIR=/var/h",
	"HARVEST_BACKEND=openrouter",
	"HARVEST_MODEL=m",
	"CLAUDECODE=1",
}

func config(on bool) Config {
	return Config{
		On:       on,
		Launcher: "/home/u/code/bin/harvest-step",
		Self:     "/home/u/code/bin/harvest",
		Environ:  parent,
	}
}

func TestBuild(t *testing.T) {
	base := []string{
		"HOME=/home/u", "PATH=/home/u/code/bin:/usr/bin", "TMPDIR=/tmp/u",
		"HARVEST_STATE_DIR=/var/h",
	}
	tests := []struct {
		name     string
		on       bool
		step     Step
		wantPath string
		wantArgs []string
		wantEnv  []string
	}{
		{
			name:     "prepare under srt gets no provider variables",
			on:       true,
			step:     Step{Name: "prepare", Args: []string{"/t.jsonl", "s1", "/repo"}},
			wantPath: "/home/u/code/bin/harvest-step",
			wantArgs: []string{
				"/home/u/code/bin/harvest",
				"step", "prepare", "--", "/t.jsonl", "s1", "/repo",
			},
			wantEnv: append(slices.Clone(base),
				"HARVEST_BACKEND=openrouter", "HARVEST_MODEL=m", "HARVEST_CHILD=1"),
		},
		{
			name:     "apply with the sandbox off runs the binary directly",
			step:     Step{Name: "apply", Args: []string{"s1"}},
			wantPath: "/home/u/code/bin/harvest",
			wantArgs: []string{"step", "apply", "--", "s1"},
			wantEnv: append(slices.Clone(base),
				"HARVEST_BACKEND=openrouter", "HARVEST_MODEL=m", "HARVEST_CHILD=1"),
		},
		{
			name:     "complete on litellm gets the provider variables and its host",
			on:       true,
			step:     Step{Name: "complete", Args: []string{"s1"}, Backend: llm.LiteLLM},
			wantPath: "/home/u/code/bin/harvest-step",
			wantArgs: []string{
				"/home/u/code/bin/harvest",
				"step", "complete", "--", "s1",
			},
			wantEnv: append(
				slices.Clone(base),
				"HARVEST_MODEL=m",
				"HARVEST_CHILD=1",
				"HARVEST_BACKEND=litellm",
				"LITELLM_BASE_URL=https://user:pw@proxy.example.com:4000/v1?x=1#f", // suspenders:ignore a made-up URL
				"LITELLM_API_KEY=lite-secret",
				"OPENROUTER_API_KEY=or-secret",
				"SRT_RUN_ALLOW_DOMAINS=proxy.example.com:4000",
			),
		},
		{
			name:     "complete on openrouter with the sandbox off allows no host",
			step:     Step{Name: "complete", Args: []string{"s1"}, Backend: llm.OpenRouter},
			wantPath: "/home/u/code/bin/harvest",
			wantArgs: []string{"step", "complete", "--", "s1"},
			wantEnv: append(
				slices.Clone(base),
				"HARVEST_MODEL=m",
				"HARVEST_CHILD=1",
				"HARVEST_BACKEND=openrouter",
				"LITELLM_BASE_URL=https://user:pw@proxy.example.com:4000/v1?x=1#f", // suspenders:ignore a made-up URL
				"LITELLM_API_KEY=lite-secret",
				"OPENROUTER_API_KEY=or-secret",
			),
		},
		{
			name:     "complete on the claude CLI runs outside srt with the parent's env",
			on:       true,
			step:     Step{Name: "complete", Args: []string{"s1"}, Backend: llm.Claude},
			wantPath: "/home/u/code/bin/harvest",
			wantArgs: []string{"step", "complete", "--", "s1"},
			wantEnv: append(slices.DeleteFunc(slices.Clone(parent), func(kv string) bool {
				return strings.HasPrefix(kv, "HARVEST_BACKEND=")
			}), "HARVEST_CHILD=1", "HARVEST_BACKEND=claude"),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Build(config(tc.on), tc.step)
			if got.Path != tc.wantPath {
				t.Errorf("Path = %q, want %q", got.Path, tc.wantPath)
			}
			if !reflect.DeepEqual(got.Args, tc.wantArgs) {
				t.Errorf("Args = %q\nwant %q", got.Args, tc.wantArgs)
			}
			if !reflect.DeepEqual(got.Env, tc.wantEnv) {
				t.Errorf("Env = %q\nwant %q", got.Env, tc.wantEnv)
			}
			if got.Dir != "/" {
				t.Errorf("Dir = %q, want /", got.Dir)
			}
		})
	}
}

func TestBuildDefaultsPath(t *testing.T) {
	cfg := config(true)
	cfg.Environ = []string{"HOME=/home/u"}
	got := Build(cfg, Step{Name: "prepare"})
	if !slices.Contains(got.Env, "PATH="+defaultPath) {
		t.Errorf("Env = %q, want PATH=%s", got.Env, defaultPath)
	}
}

func TestProviderHost(t *testing.T) {
	tests := []struct {
		backend, baseURL, want string
	}{
		{llm.LiteLLM, "https://proxy.example.com", "proxy.example.com"},
		{llm.LiteLLM, "https://proxy.example.com:4000/v1", "proxy.example.com:4000"},
		{llm.LiteLLM, "http://u:p@10.0.0.2:4000?q=1", "10.0.0.2:4000"},
		{llm.LiteLLM, "proxy.local:8080/v1", "proxy.local:8080"},
		{llm.LiteLLM, "https://proxy.example.com#frag", "proxy.example.com"},
		{llm.LiteLLM, "", ""},
		// A comma or whitespace would name a second host: none is allowed.
		{llm.LiteLLM, "https://proxy.example.com,evil.example.com", ""},
		{llm.LiteLLM, "https://proxy.example.com evil.example.com", ""},
		{llm.LiteLLM, "https://proxy.example.com\tevil.example.com/v1", ""},
		{llm.LiteLLM, "https://u:p@proxy.example.com,evil.example.com:4000", ""},
		{llm.OpenRouter, "https://ignored", "openrouter.ai"},
		{llm.Claude, "https://ignored", ""},
	}
	for _, tc := range tests {
		got := ProviderHost(tc.backend, map[string]string{envLiteLLMBaseURL: tc.baseURL})
		if got != tc.want {
			t.Errorf("ProviderHost(%s, %q) = %q, want %q", tc.backend, tc.baseURL, got, tc.want)
		}
	}
}

// TestLiteLLMBaseURLIsAProviderVariable keeps the host lookup in step with
// the variables kit/llm reads.
func TestLiteLLMBaseURLIsAProviderVariable(t *testing.T) {
	if !slices.Contains(llm.ProviderEnv, envLiteLLMBaseURL) {
		t.Errorf("llm.ProviderEnv %v lacks %s", llm.ProviderEnv, envLiteLLMBaseURL)
	}
}

// script writes an executable shell script into dir.
func script(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunFailsClosed(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain")
	if err := os.WriteFile(plain, []byte("exit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ran := filepath.Join(dir, "ran")
	self := script(t, dir, "harvest", "touch "+ran+"\n")

	tests := []struct {
		name     string
		launcher string
		want     string
	}{
		{"no launcher", filepath.Join(dir, "none"), "sandbox: launcher:"},
		{"launcher not executable", plain, "is not an executable file"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{On: true, Launcher: tc.launcher, Self: self}
			err := Run(context.Background(), cfg, Step{Name: "prepare"})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Run() error = %v, want it to contain %q", err, tc.want)
			}
			if _, err := os.Stat(ran); err == nil {
				t.Error("the step ran although the sandbox could not start it")
			}
		})
	}
}

func TestRunStartsTheStepThroughTheLauncher(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	// The fake launcher records its argv and environment, then runs the
	// command it was handed, as the real one does under srt.
	launcher := script(t, dir, "harvest-step",
		"printf '%s\\n' \"$@\" > "+calls+"\nenv >> "+calls+"\nexec \"$@\"\n")
	self := script(t, dir, "harvest", "exit 0\n")
	cfg := Config{
		On: true, Launcher: launcher, Self: self,
		Environ: []string{"HOME=" + dir, "PATH=/usr/bin:/bin", "GITHUB_TOKEN=gh-secret"},
	}
	if err := Run(context.Background(), cfg, Step{Name: "apply", Args: []string{"s1"}}); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	b, err := os.ReadFile(calls)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if !strings.HasPrefix(got, self+"\nstep\napply\n--\ns1\n") {
		t.Errorf("launcher argv:\n%s", got)
	}
	if strings.Contains(got, "gh-secret") {
		t.Errorf("a secret reached the sandbox:\n%s", got)
	}
	if !strings.Contains(got, "HARVEST_CHILD=1") {
		t.Errorf("the step lacks HARVEST_CHILD:\n%s", got)
	}
}

// TestRunReportsTheStepsError runs a script that prints to stderr as a
// failed step does, main's "harvest: " line last, and checks the error Run
// returns, which is what the run logs.
func TestRunReportsTheStepsError(t *testing.T) {
	tests := []struct {
		name   string
		stderr string // what the step prints on stderr before it exits 1
		want   string
	}{
		{
			name:   "one line after some noise",
			stderr: "some noise\nharvest: reply: no JSON object in the reply\n",
			want:   "reply: no JSON object in the reply",
		},
		{
			// kit/llm puts the body of a failed call in its error, and a
			// proxy answers with indented JSON.
			name: "an error that spans lines keeps its start",
			stderr: "harvest: llm: litellm HTTP 500: {\n" +
				"  \"error\": {\n    \"message\": \"upstream down\"\n  }\n}\n",
			want: `llm: litellm HTTP 500: { "error": { "message": "upstream down" } }`,
		},
		{
			name:   "no harvest line: the launcher's last line",
			stderr: "srt-run: starting\nsrt-run: no settings file at /x.json\n",
			want:   "srt-run: no settings file at /x.json",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			out := filepath.Join(dir, "stderr")
			if err := os.WriteFile(out, []byte(tc.stderr), 0o644); err != nil {
				t.Fatal(err)
			}
			self := script(t, dir, "harvest", "cat "+out+" >&2\nexit 1\n")
			err := Run(context.Background(), Config{Self: self}, Step{Name: "complete"})
			if err == nil || err.Error() != tc.want {
				t.Errorf("Run() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRunReportsTheExitStatusOfAQuietStep(t *testing.T) {
	quiet := script(t, t.TempDir(), "quiet", "exit 3\n")
	err := Run(context.Background(), Config{Self: quiet}, Step{Name: "apply"})
	if err == nil || !strings.Contains(err.Error(), "exit status 3") {
		t.Errorf("Run() error = %v, want the exit status", err)
	}
}

func TestStepErrorCutsALongError(t *testing.T) {
	long := strings.Repeat("é", maxErrLine)
	got := stepError("harvest: " + long)
	if len(got) > maxErrLine+len("...") || !strings.HasSuffix(got, "...") {
		t.Errorf("stepError() = %d bytes, want at most %d and a ... suffix", len(got), maxErrLine+3)
	}
	if !strings.HasPrefix(long, strings.TrimSuffix(got, "...")) {
		t.Error("stepError() split a character or lost the start")
	}
}
