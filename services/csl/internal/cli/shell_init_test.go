package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// runCLI executes the root command with args against a buffer and returns
// what it printed. Cobra remembers output writers and args across Execute
// calls, so everything is reset on cleanup.
func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()

	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
	})

	err := rootCmd.Execute()
	return out.String(), err
}

func TestShellInitPrintsFunctions(t *testing.T) {
	for _, shell := range []string{"zsh", "bash"} {
		t.Run(shell, func(t *testing.T) {
			out, err := runCLI(t, "shell-init", shell)
			if err != nil {
				t.Fatalf("shell-init %s: %v", shell, err)
			}
			for _, want := range []string{"repo() {", "repo-sync() {"} {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q:\n%s", want, out)
				}
			}
			if !strings.HasSuffix(out, "\n") {
				t.Errorf("output does not end with a newline: %q", out)
			}
		})
	}
}

func TestShellInitRejectsUnknownShell(t *testing.T) {
	_, err := runCLI(t, "shell-init", "fish")
	if err == nil {
		t.Fatal("shell-init fish: nil error, want an unsupported-shell error")
	}
	if !strings.Contains(err.Error(), "fish") {
		t.Errorf("error = %v, want it to name the shell", err)
	}
}

// TestShellInitMatchesRecipe is the recipe-can't-drift gate: a ralph-managed
// machine gets `repo` and `repo-sync` from recipes/csl/recipe.toml, a
// standalone install gets them from this command. The bodies must be the same.
func TestShellInitMatchesRecipe(t *testing.T) {
	recipe := readRepoFile(t, filepath.Join("recipes", "csl", "recipe.toml"))
	out, err := runCLI(t, "shell-init", "zsh")
	if err != nil {
		t.Fatalf("shell-init zsh: %v", err)
	}

	for _, fn := range []string{"repo", "repo-sync"} {
		want := fn + "() { " + recipeFunctionBody(t, recipe, fn) + "; }"
		if !strings.Contains(out, want) {
			t.Errorf("shell-init output does not carry the recipe body for %s.\nwant line: %s\noutput:\n%s",
				fn, want, out)
		}
	}
}

// recipeFunctionBody returns the body of [shell.functions.<name>] in a ralph
// recipe, decoded as TOML so the gate does not care how the file is laid out.
func recipeFunctionBody(t *testing.T, recipe, name string) string {
	t.Helper()

	var doc struct {
		Shell struct {
			Functions map[string]struct {
				Body string `toml:"body"`
			} `toml:"functions"`
		} `toml:"shell"`
	}
	if _, err := toml.Decode(recipe, &doc); err != nil {
		t.Fatalf("decode recipe: %v", err)
	}
	fn, ok := doc.Shell.Functions[name]
	if !ok || fn.Body == "" {
		t.Fatalf("recipe has no [shell.functions.%s] body", name)
	}
	return fn.Body
}

// readRepoFile reads a path relative to the module root, found by walking up
// from the package directory to the directory holding go.mod.
func readRepoFile(t *testing.T, rel string) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}
		dir = parent
	}

	b, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}
