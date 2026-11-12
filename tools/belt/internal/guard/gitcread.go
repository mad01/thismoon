package guard

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// GitCReadID is the allow that answers a Bash call made only of read-only
// `git -C <dir> <subcommand>` commands.
const GitCReadID = "git-c-read"

// GitCRead approves a Bash call whose every command is
// `git -C <dir> <subcommand> [args]` with a read-only subcommand in a
// read-only form, run in a repo inside an allowed root. Claude Code allow
// rules match the command text as written, so `Bash(git status:*)` never
// matches the -C form. A rule that did, `Bash(git -C * status *)`, would
// also match `git -C x -c core.fsmonitor=<cmd> status`, which runs code.
// This allow reads the whole call instead, and gives no answer for
// anything it cannot read with certainty (docs/adr/0032).
type GitCRead struct {
	cfg config.Allow // allows.git-c-read, its roots resolved only when it runs
}

// NewGitCRead builds the allow from its config entry.
func NewGitCRead(cfg config.Config) *GitCRead {
	return &GitCRead{cfg: cfg.Allows[GitCReadID]}
}

// ID implements Allow.
func (*GitCRead) ID() string { return GitCReadID }

// Event implements Allow.
func (*GitCRead) Event() string { return EventBash }

// Approve answers a Bash tool call that runs sandboxed in the foreground
// (plainBashInput) when every command in it is a read-only git -C command
// and the repo git would find from its directory (discoverRepo) is a
// normal repo whose work tree and git directories all sit inside a root.
func (g *GitCRead) Approve(in Input) *Approval {
	if in.ToolName != "Bash" || !plainBashInput(in.ToolInput) || gitEnvRedirected() {
		return nil
	}
	segs, ok := plainSegments(in.Command)
	if !ok {
		return nil
	}
	roots := g.rootsFor(in.Cwd)
	for _, words := range segs {
		dirWord, ok := gitCReadDir(words)
		if !ok {
			return nil
		}
		dir, ok := resolveGitCDir(dirWord, in.Cwd)
		if !ok {
			return nil
		}
		if repo, ok := discoverRepo(dir); !ok || !repo.within(roots) {
			return nil
		}
	}
	return approvalf(
		GitCReadID,
		"every command is a read-only `git -C` call in a repo whose work tree and git directory are inside an allowed root",
	)
}

// bashInputKeys are the Bash tool input fields an approval may carry.
// bashInputSwitches are the ones that change how the command runs, which
// pass only when false: a call with the sandbox off or run in the
// background is not the call this allow reads. Any other key, such as one
// a newer Claude Code adds, means no answer.
var (
	bashInputKeys     = []string{"command", "description", "timeout"}
	bashInputSwitches = []string{"dangerouslyDisableSandbox", "run_in_background"}
)

// plainBashInput reports whether a Bash tool input holds only the known
// keys, with every switch false.
func plainBashInput(input map[string]any) bool {
	for key, value := range input {
		if slices.Contains(bashInputKeys, key) {
			continue
		}
		if !slices.Contains(bashInputSwitches, key) || value != false {
			return false
		}
	}
	return true
}

// rootsFor resolves the roots one call is judged against: the top of the
// work tree the session's cwd sits in, unless that is home or a directory
// above it, then each configured root (config.Allow.RootDirs).
func (g *GitCRead) rootsFor(cwd string) []string {
	roots := g.cfg.RootDirs()
	if !filepath.IsAbs(cwd) {
		return roots
	}
	dir, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		return roots
	}
	if repo, ok := discoverRepo(dir); ok && !config.CoversHome(repo.top) {
		roots = append([]string{repo.top}, roots...)
	}
	return roots
}

// gitCReadDir returns the -C directory word of a read-only git command:
// git unquoted, exactly one -C right after it, no other global option
// before the subcommand, and a subcommand on the read-only list called in
// a read-only form. A ~ that starts any later word fails it.
func gitCReadDir(words []shellWord) (shellWord, bool) {
	if len(words) < 4 || words[0].quoted || words[0].text != "git" || words[1].text != "-C" {
		return shellWord{}, false
	}
	args := make([]string, 0, len(words)-4)
	for _, w := range words[4:] {
		if w.tilde {
			return shellWord{}, false
		}
		args = append(args, w.text)
	}
	if words[3].tilde || !gitReadOnly(words[3].text, args) {
		return shellWord{}, false
	}
	return words[2], true
}

// gitReadForms maps each read-only subcommand to the check its arguments
// must pass on top of riskyGitArg. Every form of the first six reads.
// branch, remote and worktree also have forms that write, so only their
// listing forms pass.
var gitReadForms = map[string]func(args []string) bool{
	"status":    anyArgs,
	"log":       anyArgs,
	"diff":      anyArgs,
	"show":      anyArgs,
	"rev-parse": anyArgs,
	"ls-files":  anyArgs,
	"branch":    branchListing,
	"remote":    remoteListing,
	"worktree":  worktreeListing,
}

// gitReadOnly reports whether a subcommand and its arguments only read.
func gitReadOnly(sub string, args []string) bool {
	form, ok := gitReadForms[sub]
	return ok && !slices.ContainsFunc(args, riskyGitArg) && form(args)
}

func anyArgs([]string) bool { return true }

// riskyGitLongOptions are the long options that write a file, run a
// program, read a file their value names, or name a program to run on a
// remote.
// parse-options takes any unique prefix of a long option (git branch --del
// deletes), so an argument naming any prefix of one is risky too.
var riskyGitLongOptions = []string{
	"output",              // diff, log, show: write the patch to a file
	"ext-diff",            // run diff.external or a diff driver's command
	"textconv",            // run textconv filters
	"no-index",            // diff two paths anywhere on disk
	"exec",                // run a command
	"upload-pack",         // name the program to run on the remote
	"receive-pack",        // the same, for a push
	"open-files-in-pager", // grep -O: run the pager on the files
	"show-signature",      // log, show: run gpg.program
	"help",                // git <cmd> --help runs man or a browser
	"exclude-from",        // ls-files: read patterns from a file
	"resolve-git-dir",     // rev-parse: read a path as a git directory or .git file
}

// riskyGitArg reports whether one argument can make a read subcommand write
// a file, run a program, or read a file outside the repo by name.
func riskyGitArg(arg string) bool {
	if strings.Contains(arg, "%G") {
		return true // log and show format placeholders that verify a signature run gpg
	}
	switch {
	case strings.HasPrefix(arg, "--"):
		name, value, _ := strings.Cut(arg[2:], "=")
		return name != "" && slices.ContainsFunc(riskyGitLongOptions, func(o string) bool {
			return strings.HasPrefix(o, name)
		}) || outsidePath(value)
	case strings.HasPrefix(arg, "-"):
		// -O<orderfile> and ls-files -X<file> read a file, and grep -O
		// runs the pager. parse-options bundles short flags (-pO<file>),
		// so an O or X anywhere in a short-flag word counts.
		return strings.ContainsAny(arg, "OX")
	}
	return outsidePath(arg)
}

// outsidePath reports whether an argument, or an --option=value value,
// names a path outside the repo by its text: absolute, from home, or
// climbing with a .. segment. git diff with two paths, one outside the
// work tree, runs as diff --no-index.
func outsidePath(arg string) bool {
	return filepath.IsAbs(arg) || strings.HasPrefix(arg, "~") ||
		slices.Contains(strings.Split(arg, "/"), "..")
}

// branchListFlags are the git branch flags that only list or print.
var branchListFlags = map[string]bool{
	"--list": true, "-a": true, "--all": true, "-r": true, "--remotes": true,
	"-v": true, "-vv": true, "--verbose": true, "--show-current": true,
}

// branchListing passes git branch in a listing form: only listing flags,
// and a positional argument only beside --list, where it is a pattern. Any
// other positional names a branch to create.
func branchListing(args []string) bool {
	list, positional := false, false
	for _, a := range args {
		switch {
		case a == "--list":
			list = true
		case strings.HasPrefix(a, "-"):
			if !branchListFlags[a] {
				return false
			}
		default:
			positional = true
		}
	}
	return !positional || list
}

// remoteListing passes a bare git remote or git remote -v.
func remoteListing(args []string) bool {
	return len(args) == 0 || len(args) == 1 && (args[0] == "-v" || args[0] == "--verbose")
}

// worktreeListing passes git worktree list with its output flags only.
func worktreeListing(args []string) bool {
	if len(args) == 0 || args[0] != "list" {
		return false
	}
	return !slices.ContainsFunc(args[1:], func(a string) bool {
		return a != "--porcelain" && a != "-v" && a != "--verbose" && a != "-z"
	})
}

// resolveGitCDir turns a -C directory word into the real directory git
// would run in. A leading ~ must be ~ or ~/ (no ~user, no ~+). The path is
// joined by plain concatenation and never cleaned before EvalSymlinks: git
// -C resolves .. physically, after any symlink before it, and EvalSymlinks
// walks the path the same way, where a lexical clean would not. A path
// that does not resolve gets no answer.
func resolveGitCDir(w shellWord, cwd string) (string, bool) {
	p := w.text
	if p == "" {
		return "", false
	}
	if w.tilde {
		home, err := os.UserHomeDir()
		if err != nil || !filepath.IsAbs(home) || p != "~" && !strings.HasPrefix(p, "~/") {
			return "", false
		}
		p = home + p[1:]
	}
	if !filepath.IsAbs(p) {
		if !filepath.IsAbs(cwd) {
			return "", false
		}
		p = cwd + string(filepath.Separator) + p
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", false
	}
	return real, true
}

// insideAny reports whether dir is one of the roots or below one.
func insideAny(dir string, roots []string) bool {
	return slices.ContainsFunc(roots, func(root string) bool {
		rel, err := filepath.Rel(root, dir)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, "../") && !filepath.IsAbs(rel)
	})
}
