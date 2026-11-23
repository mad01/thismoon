package guard

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// git runs the core.fsmonitor hook while it refreshes the index. A value
// that is a git boolean is the builtin daemon (true, yes, on, 1) or no
// monitor at all (false, no, off, 0, the empty string), and runs nothing
// belt has to read. Any other value is a hook command, which git starts
// through the shell from the top of the work tree, with git's own standard
// input (git-config(1), core.fsmonitor; githooks(5), fsmonitor-watchman).
// git commit and git merge refresh the index, so such a command may run the
// heredoc either reads as its message, while git tag and git notes add
// refresh nothing. With the key unset, git reads GIT_TEST_FSMONITOR in its
// place, and any value there is a hook command. git 2.35.1 and older read
// true and false as command names too; belt reads them as git does now.

// fsmonitorSetting is what a git call's own options and environment set
// core.fsmonitor to.
type fsmonitorSetting int

const (
	fsmonitorUnset   fsmonitorSetting = iota // nothing the call itself sets
	fsmonitorBool                            // a git boolean, which runs no command
	fsmonitorCommand                         // a command, or a value belt cannot read
)

// fsmonitorKey is the config key that names the hook.
const fsmonitorKey = "core.fsmonitor"

// fsmonitorLookupBudget bounds belt's own git config call for one directory.
const fsmonitorLookupBudget = 2 * time.Second

// fsmonitorRuns reports whether c, a command whose heredoc docRuns reads as
// a git message, is a git commit or git merge that may hand that heredoc to
// a core.fsmonitor command, and returns the directory the command runs in:
// the top of the work tree git finds (shellToplevel), "" when belt cannot
// tell. The call's own -c or --config-env decides first, since git reads it
// last: a git boolean there runs no command (ownFsmonitor). A command it
// names, an assignment in front of git that may set one, and a git config
// write or an export earlier in the command (walkMemory.noteFsmonitor) each
// count. Past those, belt asks git in the directory the commit runs in
// (repoFsmonitor), which covers repo, global, system and included config
// that earlier calls set. When belt cannot place that directory, or the
// shell may be elsewhere (dirSure), it counts too, fail closed.
func (w *shellWalk) fsmonitorRuns(c shellCmd) (dir string, runs bool) {
	tokens := c.tokens()
	if words := stripWrappers(tokens); len(words) == 0 || filepath.Base(words[0]) != "git" {
		return "", false
	}
	g, ok := segmentGitCommand(tokens, w.gitDir(tokens))
	if !ok || (g.sub != "commit" && g.sub != "merge") {
		return "", false
	}
	if dir = g.dir; !w.dirSure() {
		dir = ""
	}
	switch own := ownFsmonitor(tokens); {
	case own == fsmonitorBool:
		return "", false
	case own == fsmonitorCommand, w.mem.fsmonitorSet, dir == "":
	case !repoFsmonitor(dir):
		return "", false
	}
	w.mem.hooked = true
	return shellToplevel(dir), true
}

// fsmonitorStdin is the runner a denial names for the standard input of a
// git call a core.fsmonitor command may read (hookFeeds).
const fsmonitorStdin = "the standard input git hands a core.fsmonitor command"

// hookFeeds returns the standard input of c as code, when c is a git commit
// or merge that may hand it to a core.fsmonitor command (fsmonitorRuns),
// run in a process of its own from the directory git starts that command
// in, its steps marked walkStep.fsmonitor (hookStdin). With nothing on
// standard input but the terminal, belt asks no git config.
func (w *shellWalk) hookFeeds(text string, c shellCmd, in pipeIn) []feed {
	if !c.stdinRedirected() && in.from == nil {
		return nil
	}
	dir, hooked := w.fsmonitorRuns(c)
	if !hooked {
		return nil
	}
	base := feed{runner: fsmonitorStdin, dir: dir, fsmonitor: true}
	var out []feed
	for _, s := range w.hookStdin(text, c, in) {
		out = append(out, base.with(s))
	}
	return out
}

// hookStdin returns what c reads on standard input, from the last
// redirection of descriptor 0, as the shell applies them in order, else
// from the pipeline in front of it: a here-string, a < file the command
// wrote or a <( ) printer (operandScripts), or what a printer in front
// prints (produced). A heredoc is walkProse's, and a heredoc a cat pipes in
// is code already (docRuns), so neither adds anything; nor does /dev/null.
// Any other source is unread, which fails closed (walkMemory.claim): a file
// belt did not see written, a <&, <> or a command belt cannot read.
func (w *shellWalk) hookStdin(text string, c shellCmd, in pipeIn) []script {
	for i := min(len(c.redirs), len(c.targets)) - 1; i >= 0; i-- {
		r := c.redirs[i]
		if !strings.HasPrefix(r.op, "<") || r.fd != "" && r.fd != "0" {
			continue
		}
		target, _ := c.targets[i].literal()
		switch {
		case r.op == "<<" || r.op == "<<-", r.op == "<" && target == "/dev/null":
			return nil
		case r.op == "<<<":
			return []script{w.strictText(text, c.targets[i], true)}
		case r.op == "<":
			if s := w.operandScripts(text, c.targets[i], w.dir); len(s) > 0 {
				return s
			}
		}
		return []script{{}}
	}
	switch {
	case in.from == nil:
		return nil
	case len(in.from.docs) > 0 && catLike[filepath.Base(firstWord(*in.from))]:
		return nil
	}
	s, known := w.produced(text, *in.from)
	if !known {
		s = script{}
	}
	return []script{s}
}

// markFsmonitor is fn for the steps of a heredoc a core.fsmonitor command
// may run (fsmonitorRuns): each is marked walkStep.fsmonitor.
func markFsmonitor(fn func(walkStep)) func(walkStep) {
	return func(s walkStep) {
		s.fsmonitor = true
		fn(s)
	}
}

// hooksMessage reports whether a git commit or merge in command, run from
// cwd, may hand the heredoc it reads as its message to a core.fsmonitor
// command (fsmonitorRuns).
func hooksMessage(command, cwd string) bool {
	w := newShellWalk(command, cwd)
	w.walk(func(walkStep) {})
	return w.mem.hooked
}

// ownFsmonitor reads what a git call sets core.fsmonitor to by itself: its
// -c and --config-env options, the last one that names the key or may set
// it winning, as in git; else the assignments in front of git
// (envSetsFsmonitor) or a sudo, doas or su that runs git with another
// user's config, both read as a command.
func ownFsmonitor(tokens []string) fsmonitorSetting {
	words := unquoteAll(tokens)
	at := slices.IndexFunc(tokens, isGitWord)
	if at < 0 {
		return fsmonitorUnset
	}
	if setting := optionFsmonitor(words[at+1:]); setting != fsmonitorUnset {
		return setting
	}
	pre := words[:at]
	if envSetsFsmonitor(envAssignments(pre)) || slices.ContainsFunc(pre, asOtherUser) {
		return fsmonitorCommand
	}
	return fsmonitorUnset
}

// optionFsmonitor reads the global options after the git word up to the
// subcommand, as parseGitCmd skips them, and returns what the last -c or
// --config-env that may touch core.fsmonitor sets it to (configFsmonitor).
func optionFsmonitor(words []string) fsmonitorSetting {
	setting := fsmonitorUnset
	note := func(flag, word string) {
		if s := configFsmonitor(flag, word); s != fsmonitorUnset {
			setting = s
		}
	}
	for i := 0; i < len(words); i++ {
		switch tok := words[i]; {
		case (tok == "-c" || tok == "--config-env") && i+1 < len(words):
			i++
			note(tok, words[i])
		case strings.HasPrefix(tok, "--config-env="):
			note("--config-env", strings.TrimPrefix(tok, "--config-env="))
		case tok == "-C" || tok == "--git-dir" || tok == "--work-tree" || gitValueFlags[tok]:
			i++
		case !strings.HasPrefix(tok, "-"):
			return setting
		}
	}
	return setting
}

// configFsmonitor reads one -c key=value or --config-env key=VAR word. A
// key belt cannot read or an include may set core.fsmonitor to anything. A
// --config-env value comes from the environment, so it counts as a
// command. A -c core.fsmonitor with no = is true, the builtin daemon. Any
// other key leaves it unset.
func configFsmonitor(flag, word string) fsmonitorSetting {
	key, value, hasValue := strings.Cut(word, "=")
	switch {
	case expands(key), includeKey(strings.ToLower(key)):
		return fsmonitorCommand
	case !strings.EqualFold(key, fsmonitorKey):
		return fsmonitorUnset
	case flag == "--config-env":
		return fsmonitorCommand
	case !hasValue || (!expands(value) && gitBoolean(value)):
		return fsmonitorBool
	}
	return fsmonitorCommand
}

// fsmonitorConfigFiles are the variables that point git at config files
// belt's own git call does not read.
var fsmonitorConfigFiles = wordSet("GIT_CONFIG GIT_CONFIG_GLOBAL GIT_CONFIG_SYSTEM HOME XDG_CONFIG_HOME")

// envSetsFsmonitor reports whether NAME=value assignments may make
// core.fsmonitor a command for the git that sees them: GIT_TEST_FSMONITOR
// with a value, a GIT_CONFIG_PARAMETERS that names the key or an include or
// holds an expansion, a variable that moves a config file
// (fsmonitorConfigFiles), or GIT_CONFIG_COUNT keys that may set it
// (countedFsmonitor).
func envSetsFsmonitor(assignments []string) bool {
	vars := map[string]string{}
	for _, a := range assignments {
		name, value, _ := strings.Cut(a, "=")
		name = strings.TrimSuffix(name, "+")
		vars[name] = value
		switch {
		case name == "GIT_TEST_FSMONITOR" && value != "":
			return true
		case name == "GIT_CONFIG_PARAMETERS" && (expands(value) || mayNameFsmonitor(value)):
			return true
		case fsmonitorConfigFiles[name]:
			return true
		}
	}
	return countedFsmonitor(vars)
}

// gitConfigCountVar is a variable of the GIT_CONFIG_COUNT family.
var gitConfigCountVar = regexp.MustCompile(`^GIT_CONFIG_(COUNT|KEY_[0-9]+|VALUE_[0-9]+)$`)

// maxConfigCount bounds the GIT_CONFIG_COUNT belt reads key by key.
const maxConfigCount = 100

// countedFsmonitor reports whether the GIT_CONFIG_COUNT family in vars may
// make core.fsmonitor a command. It passes only a whole set: a literal
// count, and for each index below it a literal key that is neither an
// include nor core.fsmonitor, or core.fsmonitor with a literal git boolean
// value. A set that leaves a key or the count to the environment counts.
func countedFsmonitor(vars map[string]string) bool {
	touched := false
	for name := range vars {
		touched = touched || gitConfigCountVar.MatchString(name)
	}
	if !touched {
		return false
	}
	count, err := strconv.Atoi(vars["GIT_CONFIG_COUNT"])
	if err != nil || count < 0 || count > maxConfigCount {
		return true
	}
	for i := range count {
		n := strconv.Itoa(i)
		key, ok := vars["GIT_CONFIG_KEY_"+n]
		switch {
		case !ok, expands(key), includeKey(strings.ToLower(key)):
			return true
		case !strings.EqualFold(key, fsmonitorKey):
			continue
		}
		value, ok := vars["GIT_CONFIG_VALUE_"+n]
		if !ok || expands(value) || !gitBoolean(value) {
			return true
		}
	}
	return false
}

// noteFsmonitor records a step that may make core.fsmonitor a command for
// every git after it: a git config write that may set it
// (configSetsFsmonitor), or, in a step with no git word, an assignment or
// an export that may (envSetsFsmonitor). An assignment in front of a git
// word is that call's own (ownFsmonitor). A prose step sets nothing.
func (m *walkMemory) noteFsmonitor(s walkStep) {
	if s.prose || m.fsmonitorSet {
		return
	}
	if _, hasGit := beforeGit(s.tokens); !hasGit {
		m.fsmonitorSet = envSetsFsmonitor(envAssignments(unquoteAll(s.tokens)))
		return
	}
	c, ok := segmentGitCommand(s.tokens, "")
	m.fsmonitorSet = ok && c.sub == "config" && configSetsFsmonitor(c.args)
}

// configSetsFsmonitor reports whether a git config call with these
// arguments may leave core.fsmonitor naming a command: a write of the key
// whose value is not a literal git boolean, a write of a key belt cannot
// read or of an include, which may bring in any value, an unset, a section
// removal or rename that may touch the key and uncover a value from another
// file, and an edit. Any scope counts, --file included.
func configSetsFsmonitor(args []string) bool {
	words := unquoteAll(args)
	if configEdits(words) {
		return true
	}
	s, ok := readConfigSet(words)
	if !ok {
		return gitConfigWrites(words) && slices.ContainsFunc(words, mayNameFsmonitor)
	}
	key := words[s.key]
	switch {
	case expands(key), includeKey(strings.ToLower(key)):
		return true
	case !strings.EqualFold(key, fsmonitorKey):
		return false
	}
	return s.value < 0 || expands(words[s.value]) || !gitBoolean(words[s.value])
}

// configEdits reports whether a git config call opens an editor on a config
// file: -e, --edit or the edit subcommand.
func configEdits(words []string) bool {
	for i := 0; i < len(words); i++ {
		switch w := words[i]; {
		case w == "-e" || w == "--edit":
			return true
		case gitConfigValueFlags[w]:
			i++
		case !strings.HasPrefix(w, "-"):
			return w == "edit"
		}
	}
	return false
}

// mayNameFsmonitor reports whether text may name core.fsmonitor, its
// section or an include: it holds an expansion, fsmonitor or include in any
// case, or is the word core.
func mayNameFsmonitor(text string) bool {
	lower := strings.ToLower(text)
	return expands(text) || lower == "core" ||
		strings.Contains(lower, "fsmonitor") || strings.Contains(lower, "include")
}

// gitBooleans are the literals git reads as a boolean (git-config(1),
// Values): yes, on, true and 1, and no, off, false, 0 and the empty string,
// in any case. git also reads any other integer as one; belt reads it as a
// command, fail closed.
var gitBooleans = wordSet("true yes on 1 false no off 0")

// gitBoolean reports whether a config value is one of gitBooleans or empty.
func gitBoolean(value string) bool {
	return value == "" || gitBooleans[strings.ToLower(value)]
}

// expands reports whether a word as the shell passes it may still hold an
// expansion belt cannot read: a $ or a backtick.
func expands(word string) bool {
	return strings.ContainsAny(word, "$`")
}

// asOtherUser reports whether a word in front of git runs it as another
// user, with that user's config.
func asOtherUser(word string) bool {
	return word == "sudo" || word == "doas" || word == "su"
}

// unquoteAll returns each token with its quotes and backslashes dropped
// (unquoteWord).
func unquoteAll(tokens []string) []string {
	out := make([]string, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, unquoteWord(t))
	}
	return out
}

// fsmonitorLookups holds what repoFsmonitor learned per directory. One
// belt hook call is one process, and every guard walks the command again,
// so the cache keeps the cost to one git call per directory per call.
var fsmonitorLookups = struct {
	sync.Mutex
	byDir map[string]bool
}{byDir: map[string]bool{}}

// repoFsmonitor reports whether git, run in dir, may read core.fsmonitor as
// a command: `git config --get core.fsmonitor` prints a value that is not a
// git boolean, or finds the key unset while belt's own environment, which
// the session's shell shares, sets GIT_TEST_FSMONITOR. Any other failure,
// or no answer within fsmonitorLookupBudget, counts as a command. A config
// read refreshes no index, so the call runs no fsmonitor hook itself, and
// -c core.fsmonitor=false would only change the answer.
func repoFsmonitor(dir string) bool {
	fsmonitorLookups.Lock()
	defer fsmonitorLookups.Unlock()
	if runs, ok := fsmonitorLookups.byDir[dir]; ok {
		return runs
	}
	ctx, cancel := context.WithTimeout(context.Background(), fsmonitorLookupBudget)
	defer cancel()
	out, status, err := runProofGitCode(ctx, dir, nil, "config", "--get", fsmonitorKey)
	var runs bool
	switch {
	case err != nil:
		runs = true
	case status == 1:
		runs = os.Getenv("GIT_TEST_FSMONITOR") != ""
	case status != 0:
		runs = true
	default:
		runs = !gitBoolean(strings.TrimSuffix(string(out), "\n"))
	}
	fsmonitorLookups.byDir[dir] = runs
	return runs
}
