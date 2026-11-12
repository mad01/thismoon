package guard

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// testHome is the home directory the table tests resolve ~ and $HOME to. A
// fixed path keeps ~/.tmp off /tmp whatever the machine's temp dir is.
const testHome = "/Users/tester"

// newRmTempGuard builds the guard with a fixed home and a toss-bin that is
// always found, so no test depends on the machine's HOME or PATH.
func newRmTempGuard(t *testing.T) *RmTemp {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	g := NewRmTemp()
	g.home = func() string { return testHome }
	g.lookPath = func(string) (string, error) { return "/usr/local/bin/toss-bin", nil }
	return g
}

func TestRmTempDenies(t *testing.T) {
	tests := []struct {
		command string
		// toss are the toss-bin commands the reason must hand over.
		toss []string
	}{
		{"rm -rf /tmp/x", []string{"toss-bin --temp-only /tmp/x"}},
		{"rm /tmp/x", []string{"toss-bin --temp-only /tmp/x"}},
		{"rm -f /var/tmp/a /private/tmp/b /private/var/tmp/c", []string{
			"toss-bin --temp-only /var/tmp/a /private/tmp/b /private/var/tmp/c",
		}},
		{"rm -rf ~/.tmp/x", []string{"toss-bin --temp-only ~/.tmp/x"}},
		{"rm -rf $HOME/.tmp/x", []string{"toss-bin --temp-only $HOME/.tmp/x"}},
		{`rm -rf "${HOME}/.tmp/x"`, []string{`toss-bin --temp-only "${HOME}/.tmp/x"`}},
		{"rm -rf /tmp/x/../y", []string{"toss-bin --temp-only /tmp/x/../y"}},
		{"rm -rf '/tmp/a b'", []string{"toss-bin --temp-only '/tmp/a b'"}},
		{"rm -rf /tmp/build-*", []string{"toss-bin --temp-only /tmp/build-*"}},
		{"rm -rf /tmp/x/*", []string{"toss-bin --temp-only /tmp/x/*"}},
		{
			"rm -rf '/tmp/.*'",
			[]string{"toss-bin --temp-only '/tmp/.*'"},
		}, // quoted: one literal name
		{"rm -- /tmp/-x", []string{"toss-bin --temp-only /tmp/-x"}},
		{"rm -r -f -v /tmp/x", []string{"toss-bin --temp-only /tmp/x"}},
		{"rm --recursive --force /tmp/x", []string{"toss-bin --temp-only /tmp/x"}},
		// Inside a chain, a pipeline, a subshell, a group or an if.
		{"make build && rm -rf /tmp/x", []string{"toss-bin --temp-only /tmp/x"}},
		{"make build&&rm -rf /tmp/x", []string{"toss-bin --temp-only /tmp/x"}},
		{"false || rm -rf /tmp/x; echo done", []string{"toss-bin --temp-only /tmp/x"}},
		{"(cd /tmp && rm -rf /tmp/x)", []string{"toss-bin --temp-only /tmp/x"}},
		{"{ rm -rf /tmp/x; }", []string{"toss-bin --temp-only /tmp/x"}},
		{"if true; then rm -rf /tmp/x; fi", []string{"toss-bin --temp-only /tmp/x"}},
		{"echo start\nrm -rf /tmp/x", []string{"toss-bin --temp-only /tmp/x"}},
		// Redirections and comments are not targets.
		{"rm -rf /tmp/x 2>/dev/null || true", []string{"toss-bin --temp-only /tmp/x"}},
		{"rm -rf /tmp/x >/dev/null 2>&1", []string{"toss-bin --temp-only /tmp/x"}},
		{"rm -rf /tmp/x &>/dev/null", []string{"toss-bin --temp-only /tmp/x"}},
		{"rm -rf /tmp/x # clean up", []string{"toss-bin --temp-only /tmp/x"}},
		// Other ways to name rm.
		{`\rm -rf /tmp/x`, []string{"toss-bin --temp-only /tmp/x"}},
		{"/bin/rm -rf /tmp/x", []string{"toss-bin --temp-only /tmp/x"}},
		{"FOO=1 rm -rf /tmp/x", []string{"toss-bin --temp-only /tmp/x"}},
		// A heredoc body is data; the rm after it still counts.
		{
			"cat <<EOF > notes.md\nrm -rf /etc\nEOF\nrm -rf /tmp/x",
			[]string{"toss-bin --temp-only /tmp/x"},
		},
		// Every rm in the call, in order.
		{"rm -rf /tmp/a; make; rm -f /tmp/b", []string{
			"`toss-bin --temp-only /tmp/a`, `toss-bin --temp-only /tmp/b`",
		}},
		// A variable set from mktemp in the same command.
		{`D=$(mktemp -d); touch "$D/a"; rm -rf "$D"`, []string{`toss-bin --temp-only "$D"`}},
		{`D=$(mktemp -d); rm -rf "$D/sub"`, []string{`toss-bin --temp-only "$D/sub"`}},
		{`D=$(mktemp -d) && rm -rf $D/sub/../x`, []string{`toss-bin --temp-only $D/sub/../x`}},
		{`D="$(mktemp -d -t belt)"; rm -rf "${D}"`, []string{`toss-bin --temp-only "${D}"`}},
		{"F=`mktemp`; rm -f \"$F\"", []string{`toss-bin --temp-only "$F"`}},
		{`export D=$(mktemp -d); rm -rf "$D"`, []string{`toss-bin --temp-only "$D"`}},
		{`D=$(mktemp -d /tmp/t.XXXXXX); rm -rf "$D"`, []string{`toss-bin --temp-only "$D"`}},
		{`D=$(mktemp -d -p /tmp); rm -rf "$D"`, []string{`toss-bin --temp-only "$D"`}},
		{`D=$(mktemp -d ~/.tmp/t.XXXXXX); rm -rf "$D"`, []string{`toss-bin --temp-only "$D"`}},
		{`D=$(mktemp -d /tmp/t.XXXXXX); rm -rf "$D/../sibling"`, []string{
			`toss-bin --temp-only "$D/../sibling"`,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			d := newRmTempGuard(t).Check(Input{Event: EventBash, Command: tt.command, Cwd: "/repo"})
			if d == nil {
				t.Fatalf("Check(%q) = nil, want a deny", tt.command)
			}
			if !strings.HasPrefix(
				d.Reason,
				"belt[rm-temp]: every rm target here is inside a temp folder",
			) {
				t.Errorf("reason = %q, want the rm-temp prefix and message", d.Reason)
			}
			for _, toss := range tt.toss {
				if !strings.Contains(d.Reason, toss) {
					t.Errorf("reason = %q, want it to hand over %q", d.Reason, toss)
				}
			}
		})
	}
}

func TestRmTempLeavesAlone(t *testing.T) {
	tests := []struct {
		name    string
		command string
	}{
		{"repo path", "rm repo/file"},
		{"bare name", "rm file"},
		{"mixed targets", "rm /tmp/x repo/file"},
		{"temp and absolute", "rm -rf /tmp/x /etc/y"},
		{"one rm of two outside", "rm -rf /tmp/x; rm repo/file"},
		{"no rm at all", "make build"},
		// Paths that only look like temp.
		{"dotdot out of tmp", "rm -rf /tmp/../etc"},
		{"tmp itself", "rm -rf /tmp"},
		{"tmp with slash", "rm -rf /tmp/"},
		{"tmp dot", "rm -rf /tmp/."},
		{"back up to tmp", "rm -rf /tmp/x/.."},
		{"var tmp itself", "rm -rf /var/tmp"},
		{"home tmp itself", "rm -rf ~/.tmp"},
		{"home tmp dotdot", "rm -rf $HOME/.tmp/../x"},
		{"home tmp dotdot braced", `rm -rf "${HOME}/.tmp/../x"`},
		{"relative tmp", "rm -rf tmp/x"},
		{"dot relative tmp", "rm -rf ./tmp/x"},
		{"tmp prefix only", "rm -rf /tmpfoo/x"},
		{"other case", "rm -rf /TMP/x"},
		{"single-quoted home var", "rm -rf '$HOME/.tmp/x'"},
		{"single-quoted tilde", "rm -rf '~/.tmp/x'"},
		{"other user tilde", "rm -rf ~other/.tmp/x"},
		// Globs that can leave the folder.
		{"dot glob", "rm -rf /tmp/.*"},
		{"dot glob deeper", "rm -rf /tmp/x/.?"},
		{"glob then dotdot", "rm -rf /tmp/*/.."},
		{"recursive glob then dotdot", "rm -rf /tmp/**/../x"},
		{"glob in the root", "rm -rf /t*p/x"},
		{"glob on the root name", "rm -rf /tmp*/x"},
		{"brace expansion", "rm -rf /tmp/{a,../etc}"},
		{"zsh glob qualifier", "rm -rf /tmp/*(D)"},
		// Targets the guard cannot resolve.
		{"unknown variable", `rm -rf "$X"`},
		{"positional parameter", "rm -rf /tmp/$1"},
		{"default expansion", `rm -rf "${D:-/tmp/x}"`},
		{"command substitution", "rm -rf $(cat list)"},
		{"backtick substitution", "rm -rf `ls /tmp`"},
		{"unterminated quote", `rm -rf "/tmp/x`},
		// rm as an argument or inside a string is not an rm.
		{"echo argument", "echo rm -rf /tmp/x"},
		{"double-quoted string", `echo "rm -rf /tmp/x"`},
		{"quoted separator", `git commit -m "fix; rm -rf /tmp/x"`},
		{"single-quoted separator", "echo 'a && rm -rf /tmp/x'"},
		{"bash -c string", "bash -c 'rm -rf /tmp/x'"},
		{"xargs", "ls /tmp/x | xargs rm -rf"},
		{"find exec", `find /tmp -name x -exec rm {} \;`},
		{"sudo", "sudo rm -rf /tmp/x"},
		{"heredoc body", "cat <<'EOF' > notes.md\nrm -rf /tmp/x\nEOF"},
		{"comment", "# rm -rf /tmp/x"},
		{"rm in a word", "firm /tmp/x"},
		// Option shapes the guard does not vouch for.
		{"no operand", "rm -rf"},
		{"help", "rm --help"},
		{"option after operand", "rm /tmp/x -f"},
		{"unknown option", "rm -Z /tmp/x"},
		// mktemp variables the guard cannot place.
		{"no mktemp", `rm -rf "$D"`},
		{"mktemp dotdot", `D=$(mktemp -d); rm -rf "$D/.."`},
		{"mktemp sibling", `D=$(mktemp -d); rm -rf "$D/../x"`},
		{"mktemp suffix", `D=$(mktemp -d); rm -rf "${D}x"`},
		{"mktemp relative template", `D=$(mktemp -d ./x.XXXXXX); rm -rf "$D"`},
		{"mktemp outside dir", `D=$(mktemp -d -p /home/x); rm -rf "$D"`},
		{"mktemp template outside", `D=$(mktemp -d /etc/x.XXXXXX); rm -rf "$D"`},
		{"mktemp tmpdir template", `D=$(mktemp -d -t a/b); rm -rf "$D"`},
		{"mktemp unknown flag", `D=$(mktemp -z); rm -rf "$D"`},
		{"mktemp variable dir", `D=$(mktemp -d -p "$X"); rm -rf "$D"`},
		{"not mktemp", `D=$(pwd); rm -rf "$D"`},
		{"reassigned", `D=$(mktemp -d); D=/etc; rm -rf "$D"`},
		{"appended", `D=$(mktemp -d); D+=/../..; rm -rf "$D"`},
		{"prefix assignment", `D=$(mktemp -d) rm -rf "$D"`},
		{"rm before mktemp", `rm -rf "$D"; D=$(mktemp -d)`},
		{"mktemp plus outside", `D=$(mktemp -d); rm -rf "$D" /etc/x`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if d := newRmTempGuard(t).Check(Input{Event: EventBash, Command: tt.command}); d != nil {
				t.Errorf("Check(%q) = %q, want no answer", tt.command, d.Reason)
			}
		})
	}
}

func TestRmTempReason(t *testing.T) {
	const prefix = "belt[rm-temp]: every rm target here is inside a temp folder, so delete with " +
		"toss-bin instead: it needs no permission prompt and moves the paths to the trash. "
	tests := []struct {
		command string
		want    string
	}{
		{
			"rm -rf /tmp/x ~/.tmp/y",
			prefix + "Replace the rm with `toss-bin --temp-only /tmp/x ~/.tmp/y`.",
		},
		{
			"rm -rf /tmp/a && rm /tmp/b",
			prefix + "Replace the rm commands, in order, with " +
				"`toss-bin --temp-only /tmp/a`, `toss-bin --temp-only /tmp/b`.",
		},
		{
			`D=$(mktemp -d); rm -rf "$D"`,
			prefix + "Replace the rm with `toss-bin --temp-only \"$D\"`. " +
				"toss-bin takes only /tmp, /var/tmp and ~/.tmp, not $TMPDIR, so give mktemp a " +
				"template there: `mktemp -d /tmp/<name>.XXXXXX` (no -d for a file).",
		},
		{
			`D=$(mktemp -d /tmp/t.XXXXXX); rm -rf "$D"`,
			prefix + "Replace the rm with `toss-bin --temp-only \"$D\"`.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			d := newRmTempGuard(t).Check(Input{Event: EventBash, Command: tt.command})
			if d == nil {
				t.Fatalf("Check(%q) = nil, want a deny", tt.command)
			}
			if d.Reason != tt.want {
				t.Errorf("reason =\n  %q\nwant\n  %q", d.Reason, tt.want)
			}
		})
	}
}

// TestRmTempNeedsTossBin keeps the guard silent where its fix cannot run.
func TestRmTempNeedsTossBin(t *testing.T) {
	g := newRmTempGuard(t)
	g.lookPath = func(string) (string, error) { return "", errors.New("not found") }
	if d := g.Check(Input{Event: EventBash, Command: "rm -rf /tmp/x"}); d != nil {
		t.Errorf("Check without toss-bin = %q, want no answer", d.Reason)
	}
}

// TestRmTempRealHome checks the ~/.tmp root against $HOME through the
// constructor's own home lookup.
func TestRmTempRealHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	g := NewRmTemp()
	g.lookPath = func(string) (string, error) { return "/usr/local/bin/toss-bin", nil }
	for _, command := range []string{
		"rm -rf ~/.tmp/x",
		"rm -rf " + filepath.Join(home, ".tmp", "x"),
	} {
		if d := g.Check(Input{Event: EventBash, Command: command}); d == nil {
			t.Errorf("Check(%q) = nil, want a deny with HOME=%s", command, home)
		}
	}
}

// TestRmTempRegistered checks the guard runs on the bash event by default
// and that its toggle switches it off.
func TestRmTempRegistered(t *testing.T) {
	ids := func(cfg config.Config) []string {
		var out []string
		for _, g := range ForEvent(EventBash, cfg) {
			out = append(out, g.ID())
		}
		return out
	}
	if got := ids(config.Config{}); !strings.Contains(strings.Join(got, " "), RmTempID) {
		t.Errorf("bash guards = %v, want %s enabled by default", got, RmTempID)
	}
	off := false
	cfg := config.Config{Guards: map[string]config.Toggle{RmTempID: {Enabled: &off}}}
	if got := ids(cfg); strings.Contains(strings.Join(got, " "), RmTempID) {
		t.Errorf("bash guards = %v, want %s off when disabled", got, RmTempID)
	}
}
