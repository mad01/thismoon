package cli

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/mad01/thismoon/services/csl"
)

// TestClaudeMDCLIBlockMatchesCommands is the cheat-sheet gate. The fenced sh
// block in claude-md.md is what lets an agent run the CLI without spending a
// turn on --help, so every subcommand and flag it names must exist in the
// cobra tree. Inline `csl ...` spans in the prose are held to the same rule.
func TestClaudeMDCLIBlockMatchesCommands(t *testing.T) {
	block := fencedShellBlock(t, csl.ClaudeMD)
	var invocations []string
	for line := range strings.SplitSeq(block, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "csl ") {
			invocations = append(invocations, strings.TrimSpace(line))
		}
	}
	if len(invocations) < 5 {
		t.Fatalf(
			"found %d csl lines in the CLI block; the gate would pass vacuously",
			len(invocations),
		)
	}
	for _, span := range inlineCSLSpan.FindAllStringSubmatch(csl.ClaudeMD, -1) {
		invocations = append(invocations, span[1])
	}

	for _, inv := range invocations {
		if err := checkInvocation(rootCmd, inv); err != nil {
			t.Errorf("claude-md.md runs %q: %v", inv, err)
		}
	}
}

// TestCheckInvocation pins the checker itself, so a stale cheat sheet cannot
// pass because the checker accepts everything.
func TestCheckInvocation(t *testing.T) {
	tests := []struct {
		inv  string
		want bool // true = valid
	}{
		{`csl search '<query>' --repo <re> --file '<re>'`, true},
		{`csl search '<query>' -o content -C 2 --limit 100`, true},
		{`csl search 'a b' --limit 5000 | cut -d/ -f1-2 | sort -u`, true},
		{`csl outline <repo> [path] --kinds struct,function # ranked`, true},
		{`csl semantic files --skipped <path>`, true},
		{`csl repo --list --config=/tmp/c.yaml`, true}, // persistent root flag
		{`csl search '<query>' --nope`, false},
		{`csl search '<query>' -Z`, false},
		{`csl serch '<query>'`, false},
		{`csl --json`, false},
		{`grep -r foo .`, false},
	}
	for _, tc := range tests {
		t.Run(tc.inv, func(t *testing.T) {
			err := checkInvocation(rootCmd, tc.inv)
			if tc.want && err != nil {
				t.Errorf("checkInvocation(%q) = %v, want nil", tc.inv, err)
			}
			if !tc.want && err == nil {
				t.Errorf("checkInvocation(%q) = nil, want an error", tc.inv)
			}
		})
	}
}

// inlineCSLSpan matches an inline code span that runs the CLI, like
// `csl query '<query>'`; tool names such as `csl_search` do not match.
var inlineCSLSpan = regexp.MustCompile("`(csl [^`]+)`")

// checkInvocation walks one `csl ...` command line down the cobra tree from
// root: leading words name subcommands, then every flag must be defined on
// the resolved command or inherited from a parent. Positional arguments and
// flag values are not checked. Parsing stops at a pipe or a comment.
func checkInvocation(root *cobra.Command, inv string) error {
	words := shellWords(inv)
	if len(words) == 0 || words[0] != root.Name() {
		return fmt.Errorf("does not start with %q", root.Name())
	}

	cmd := root
	descending := true
	for i := 1; i < len(words); i++ {
		w := words[i]
		if w == "|" || strings.HasPrefix(w, "#") {
			break
		}
		if !strings.HasPrefix(w, "-") || w == "-" {
			if descending {
				sub := subcommand(cmd, w)
				if sub == nil && cmd == root {
					return fmt.Errorf("no subcommand %q", w)
				}
				if sub != nil {
					cmd = sub
					continue
				}
			}
			descending = false // a positional argument ends the command path
			continue
		}
		descending = false
		if cmd == root {
			return fmt.Errorf("flag %s comes before any subcommand", w)
		}
		consumesNext, err := checkFlag(cmd, w)
		if err != nil {
			return err
		}
		if consumesNext {
			i++
		}
	}
	if cmd == root {
		return fmt.Errorf("names no subcommand")
	}
	return nil
}

// checkFlag reports whether word names a flag cmd accepts, and whether that
// flag takes the next word as its value.
func checkFlag(cmd *cobra.Command, word string) (bool, error) {
	if long, ok := strings.CutPrefix(word, "--"); ok {
		name, _, hasValue := strings.Cut(long, "=")
		f := lookupFlag(cmd, name, false)
		if f == nil {
			return false, fmt.Errorf("%q has no flag --%s", cmd.CommandPath(), name)
		}
		return !hasValue && f.Value.Type() != "bool", nil
	}
	short := strings.TrimPrefix(word, "-")
	for j, r := range short {
		f := lookupFlag(cmd, string(r), true)
		if f == nil {
			return false, fmt.Errorf("%q has no flag -%c", cmd.CommandPath(), r)
		}
		if f.Value.Type() != "bool" {
			// The value is the rest of the word, or the next word.
			return j == len(short)-1, nil
		}
	}
	return false, nil
}

// lookupFlag finds a flag defined on cmd or inherited from a parent, by long
// name or by shorthand.
func lookupFlag(cmd *cobra.Command, name string, shorthand bool) *pflag.Flag {
	for _, set := range []*pflag.FlagSet{cmd.Flags(), cmd.InheritedFlags()} {
		var f *pflag.Flag
		if shorthand {
			f = set.ShorthandLookup(name)
		} else {
			f = set.Lookup(name)
		}
		if f != nil {
			return f
		}
	}
	return nil
}

// subcommand returns cmd's direct subcommand called name, or nil.
func subcommand(cmd *cobra.Command, name string) *cobra.Command {
	for _, sub := range cmd.Commands() {
		if sub.Name() == name || sub.HasAlias(name) {
			return sub
		}
	}
	return nil
}

// shellWords splits a command line on spaces outside quotes and drops the
// quotes, enough for the one-line commands the fragment shows.
func shellWords(line string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	inWord := false
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inWord = true
		case r == ' ' || r == '\t':
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out
}

// fencedShellBlock returns the body of the first ```sh block in md.
func fencedShellBlock(t *testing.T, md string) string {
	t.Helper()

	_, body, found := strings.Cut(md, "\n```sh\n")
	if !found {
		t.Fatal("claude-md.md has no ```sh block")
	}
	block, _, found := strings.Cut(body, "\n```")
	if !found {
		t.Fatal("unterminated ```sh block in claude-md.md")
	}
	return block
}
