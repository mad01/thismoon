package guard

import (
	"cmp"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// GitPushMainID identifies the git-push-to-default-branch guard.
const GitPushMainID = "git-push-main"

var defaultBranches = map[string]bool{"main": true, "master": true}

// GitPushMain blocks `git push` to main/master unless the target repository
// is on the guard's allow_repos allowlist. A machine class where direct
// pushes are fine (a personal machine, say) disables the guard in its
// rendered config; everywhere else, and on a machine with no config at all,
// pushes to the default branch are denied (docs/adr/0010).
type GitPushMain struct {
	cfg config.Config
	// resolveBranch returns the current branch of the repo at dir, or ""
	// when it cannot be determined. Injectable for tests.
	resolveBranch func(dir string) string
	// resolveRepo returns the canonical host/owner/repo the repo at dir
	// pushes to through origin (e.g. github.com/mad01/dotfiles), or "" when
	// it cannot be determined. Injectable for tests.
	resolveRepo func(dir string) string
	// resolvePushRemote returns the remote a bare push of branch goes to
	// from the repo at dir, or "" when it cannot be determined. Injectable
	// for tests.
	resolvePushRemote func(dir, branch string) string
}

// NewGitPushMain builds the guard with the real git-backed resolvers.
func NewGitPushMain(cfg config.Config) *GitPushMain {
	return &GitPushMain{
		cfg:               cfg,
		resolveBranch:     gitCurrentBranch,
		resolveRepo:       canonicalPushRepoAt,
		resolvePushRemote: gitPushRemote,
	}
}

func (g *GitPushMain) ID() string    { return GitPushMainID }
func (g *GitPushMain) Event() string { return EventBash }

// Check scans every git push in the command (compound commands included) and
// denies when one targets main or master. Each push is judged in the
// directory it runs in (gitCommandsAt tracks `cd`/`pushd` and resolves `git
// -C`), not the session cwd. Without that, `cd <other-repo> && git push
// origin main` from an exempt repo's cwd would be checked against the exempt
// repo and wrongly allowed. A variable the command sets to a literal path
// resolves to that path first (`S=~/repo; git -C $S push`), but only when
// nothing in the command could leave it empty or change it
// (resolveLiteralVars). A cd or git -C whose target still cannot be resolved
// without a shell leaves the directory unknown, and checkPush then fails
// closed instead of guessing. A push wrapped in a subshell, a substitution,
// a shell's -c script or eval is judged like a plain one (walkCommands). A
// refspec variable the command sets to the current branch in the one shape
// branchVars vouches for reads as the current branch.
func (g *GitPushMain) Check(in Input) *Denial {
	var pushes []gitPush
	for _, c := range gitCommandsAt(in.Command, in.Cwd, "push") {
		pushes = append(pushes, parsePushArgs(c))
	}
	vars := branchVars(in.Command, pushes)
	for _, push := range pushes {
		push.branchVars = vars
		if d := g.checkPush(push); d != nil {
			return d
		}
	}
	return nil
}

// checkPush judges one push against the directory it runs in. An empty
// push.dir means that directory could not be determined, and the push is
// handed to unknownDirDenial rather than resolved against a guessed location.
// So is a push that needs the current branch where git cannot read one: the
// shell may not be in that directory at all, after a cd that failed before a
// `;` or one that found its target through CDPATH, and a push from a
// directory with no repository fails anyway. A push from code the reader
// could not follow goes to unreadDenial.
func (g *GitPushMain) checkPush(push gitPush) *Denial {
	switch {
	case push.unread:
		return unreadDenial(push)
	case push.dir == "":
		return unknownDirDenial(push)
	}
	noBranch, current := false, ""
	branch, unsure := push.targetBranch(func() string {
		current = g.resolveBranch(push.dir)
		noBranch = current == ""
		return current
	})
	switch {
	case noBranch:
		return unknownDirDenial(push)
	case unsure != "":
		return unsureRefDenial(unsure)
	case !defaultBranches[branch]:
		return nil
	case !g.pushExempt(push.dir):
		return Reasonf(
			GitPushMainID,
			"pushing to %q is blocked on this machine (direct pushes to the default branch are never allowed here). "+
				"Create a feature branch and open a PR instead: git checkout -b <branch> && git push -u origin <branch>.",
			branch,
		)
	case !g.toPlainOrigin(push, current):
		return Reasonf(
			GitPushMainID,
			"pushing to %q is blocked: this repo's direct-push exemption covers a push to its origin remote "+
				"only, with no -c, --config-env, --repo, sudo, or GIT_CONFIG*, GIT_SSH*, HOME or "+
				"XDG_CONFIG_HOME override. Push with a plain `git push origin %s`.",
			branch,
			branch,
		)
	}
	return nil
}

// toPlainOrigin reports whether a push goes to the remote named origin with
// nothing in the command that could change where origin points: the
// exemption names the repo origin's push URL resolves to, so a URL, another
// remote, --repo, or a config override would push past that check. A bare
// push goes where git picks for current, the branch it pushes.
func (g *GitPushMain) toPlainOrigin(push gitPush, current string) bool {
	switch {
	case push.configured || push.repoFlag:
		return false
	case push.remote != "":
		return push.remote == "origin"
	}
	return g.resolvePushRemote(push.dir, current) == "origin"
}

// unreadDenial decides a push the walk found in code readShell could not
// follow, so its words come from the token split and a quote may hide where
// it goes. It is denied unless its refspecs name non-default branches in
// plain words, which is safe wherever it runs.
func unreadDenial(push gitPush) *Denial {
	plain := push.explicit && !slices.ContainsFunc(push.refspecs, func(s string) bool {
		return strings.ContainsAny(s, unresolvableChars+`\#`)
	})
	if plain && unknownDirDenial(push) == nil {
		return nil
	}
	return Reasonf(
		GitPushMainID,
		"cannot read this command's quoting or substitutions, so the push in it cannot be checked. "+
			"Write the push as a plain command (git push origin <branch>), outside quotes and substitutions.",
	)
}

// unknownDirDenial decides a push whose directory could not be determined: a
// cd, pushd, popd, or git -C before it with a target only a shell can
// resolve (cd -, a stack entry, a CDPATH search, a substitution, a variable
// resolveLiteralVars cannot vouch for), a cd the shell may not have run, a
// --git-dir/--work-tree or GIT_DIR and its siblings pointing git elsewhere,
// or a directory where git reads no branch (checkPush). Neither the current
// branch nor the repo can be resolved without running git somewhere it does
// not belong, so the push is denied unless its refspecs name a non-default
// branch outright, which is safe wherever it runs.
func unknownDirDenial(push gitPush) *Denial {
	needsBranch := false
	branch, unsure := push.targetBranch(func() string {
		needsBranch = true
		return ""
	})
	if !needsBranch && unsure == "" && !defaultBranches[branch] {
		return nil
	}
	return Reasonf(
		GitPushMainID,
		"cannot tell which repo this push runs in: its directory comes from a cd, pushd, popd, or "+
			"git -C only a shell can resolve (cd -, cd +1, a CDPATH search, a substitution, or a "+
			"variable belt cannot pin to one literal path), from a cd that may not have run, from "+
			"--git-dir, --work-tree, or GIT_DIR, or git reads no branch there. Use an absolute "+
			"path, ~, or $HOME in a plain cd or git -C, or a variable set to one of those at the "+
			"start of a plain command (S=~/repo; git -C $S push), so the target repo can be checked.",
	)
}

// retargetsPush reports whether one step can change where a push goes once
// it has run: a git config write (gitConfigWrites), a git remote add,
// set-url, rename or remove, a git branch upstream change, a redirection
// into a git config file (gitConfigFile), or any other command that names
// one in its words. A command that only reads or prints (readOnlyCommand)
// writes through its redirections alone, and a comment or a heredoc body no
// shell runs (walkStep.prose) changes nothing.
func retargetsPush(s walkStep) bool {
	if s.prose {
		return false
	}
	writes := append(slices.Clone(s.targets), redirectOperands(s.tokens)...)
	if slices.ContainsFunc(writes, func(t string) bool { return gitConfigFile(t, s.dir) }) {
		return true
	}
	if c, ok := segmentGitCommand(s.tokens, s.dir); ok {
		switch c.sub {
		case "config":
			return gitConfigWrites(c.args)
		case "remote":
			return len(c.args) > 0 && remoteWrites[c.args[0]]
		case "branch":
			return slices.ContainsFunc(c.args, upstreamFlag)
		}
		return false
	}
	words := stripWrappers(s.tokens)
	if len(words) == 0 || readOnlyCommand(words) {
		return false
	}
	return slices.ContainsFunc(words, func(w string) bool { return gitConfigFile(w, s.dir) })
}

// readOnlyCommands only read the files they name, or print their words.
var readOnlyCommands = wordSet("echo printf cat head tail less more grep rg ls stat wc diff file")

// readOnlyCommand reports whether words run a readOnlyCommands command. rg
// with --pre runs a command on each file, so it is no read.
func readOnlyCommand(words []string) bool {
	name := path.Base(words[0])
	if !readOnlyCommands[name] {
		return false
	}
	return name != "rg" || !slices.ContainsFunc(words, func(w string) bool {
		return strings.HasPrefix(w, "--pre")
	})
}

// remoteWrites are the git remote subcommands that change a remote.
var remoteWrites = wordSet("add set-url rename remove rm")

// upstreamFlag reports whether a git branch argument changes a branch's
// upstream, which a bare push may follow.
func upstreamFlag(arg string) bool {
	return arg == "-u" || arg == "--unset-upstream" || strings.HasPrefix(arg, "--set-upstream")
}

// gitConfigReadFlags make a git config call read only.
var gitConfigReadFlags = wordSet(`
	--get --get-all --get-regexp --get-urlmatch --get-color --get-colorbool -l --list
`)

// gitConfigWriteFlags make a git config call write.
var gitConfigWriteFlags = wordSet(`
	--add --replace-all --unset --unset-all --rename-section --remove-section -e --edit
`)

// gitConfigValueFlags are the git config options that take the next word as
// their value.
var gitConfigValueFlags = wordSet("-f --file --blob --type --default --comment --value")

// gitConfigWrites reports whether git config with these arguments may write:
// a write flag or subcommand, or two positionals (key and value). A read
// flag, the get and list subcommands, and a lone key only read.
func gitConfigWrites(args []string) bool {
	var positional []string
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case gitConfigWriteFlags[arg]:
			return true
		case gitConfigReadFlags[arg]:
			return false
		case gitConfigValueFlags[arg]:
			i++
		case !strings.HasPrefix(arg, "-"):
			positional = append(positional, arg)
		}
	}
	switch {
	case len(positional) == 0:
		return false
	case positional[0] == "get" || positional[0] == "list":
		return false
	}
	return len(positional) > 1
}

// gitConfigVars are the variables that name a git config file or the git
// directory, so a path built from one may be a config file.
var gitConfigVars = wordSet("GIT_CONFIG GIT_CONFIG_GLOBAL GIT_CONFIG_SYSTEM GIT_DIR GIT_COMMON_DIR")

// gitDirFlags are the git rev-parse options that print the git directory.
var gitDirFlags = []string{"--git-dir", "--git-common-dir", "--git-path", "--absolute-git-dir"}

// gitConfigFile reports whether a word, a path relative to dir where it is
// relative, may name a git config file: anything under a .git directory, a
// .gitconfig, the XDG git/config, a file a GIT_CONFIG* variable names (in
// the word, or the value belt's own environment holds), or a path built from
// what rev-parse says the git directory is.
func gitConfigFile(word, dir string) bool {
	w := strings.NewReplacer(`"`, "", `'`, "", `\`, "").Replace(word)
	switch {
	case w == "":
		return false
	case mentionsName(w, gitConfigVars),
		strings.Contains(w, "gitconfig"), strings.Contains(w, "git/config"),
		slices.ContainsFunc(gitDirFlags, func(f string) bool { return strings.Contains(w, f) }):
		return true
	}
	for _, name := range []string{"GIT_CONFIG", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM"} {
		if v := os.Getenv(name); v != "" && strings.Contains(w, v) {
			return true
		}
	}
	if dir != "" && !path.IsAbs(w) && !strings.HasPrefix(w, "~") {
		w = path.Join(dir, w)
	}
	return slices.Contains(strings.FieldsFunc(w, func(r rune) bool {
		return strings.ContainsRune("/ \t\n=,:;()<>{}[]$`|&", r)
	}), ".git")
}

// redirectOperands returns what the redirections in a token-walk command's
// tokens name (`>>f`, `>> f`, `2>f`).
func redirectOperands(tokens []string) []string {
	var out []string
	for i, tok := range tokens {
		redir, operandNext := redirection(tok)
		switch {
		case !redir:
		case operandNext && i+1 < len(tokens):
			out = append(out, tokens[i+1])
		case !operandNext:
			out = append(out, strings.TrimLeft(tok, "0123456789&<>|"))
		}
	}
	return out
}

// unsureRefDenial decides a push with a refspec word only a shell can
// resolve (targetBranch): it may name a default branch, or a colon and one,
// so the push is denied wherever it runs.
func unsureRefDenial(word string) *Denial {
	return Reasonf(
		GitPushMainID,
		"cannot tell which branch the refspec %s pushes to: a variable, substitution, $'...' "+
			"string, brace or glob may name main or master. Name the branch in plain text "+
			"(git push origin <branch>), or use HEAD or \"$(git branch --show-current)\" for the "+
			"current branch.",
		word,
	)
}

// pushExempt reports whether the repo at dir may take a direct default-
// branch push: the shared direct_main_repos list (this guard is one of its
// two readers, docs/adr/0013) or the guard's own allow_repos. Empty lists
// and unresolved repos fail closed: only an explicit, successfully-resolved
// match exempts a push. Patterns match as they do everywhere else in the
// config — exactly, or by trailing "/*" org wildcard.
func (g *GitPushMain) pushExempt(dir string) bool {
	repo := g.resolveRepo(dir)
	if repo == "" {
		return false
	}
	return g.cfg.DirectMain(repo) ||
		config.RepoMatches(g.cfg.Guards[GitPushMainID].AllowRepos, repo)
}

// gitPush is one parsed `git push` invocation.
type gitPush struct {
	dir      string   // the directory the push runs in; "" when unknown
	remote   string   // first positional; "" for a bare push
	refspecs []string // positional args after the remote
	explicit bool     // true when at least one refspec was given
	// The sweep and delete flags decide what publish-internal-names scans:
	// --all/--mirror push every branch, --tags every tag, and --delete
	// removes a ref rather than publishing one.
	all, tags, mirror, del bool
	// configured, repoFlag and unread decide whether the exemption may apply:
	// a config override (gitInvocation.configured), a --repo flag naming the
	// destination, and words from code the reader could not follow.
	configured, repoFlag, unread bool
	// moved marks a git -C: a substitution in the push's words runs in the
	// shell's directory, not the one git pushes from.
	moved bool
	// branchVars are the variables the command sets to the current branch
	// (branchVars), so a refspec "$NAME" reads like HEAD.
	branchVars map[string]bool
}

// targetBranch resolves which branch the push lands on. For explicit refspecs
// the destination side (after ':') decides; bare pushes and HEAD ask
// currentBranch for the branch checked out in the push directory. A refspec
// word that is not plain text (plainRefspec) is unsure, the first such word
// returned as written, unless it is a current-branch idiom
// (currentBranchWord) or a variable branchVars vouches for, which read like
// HEAD. A default branch named outright wins over an unsure word.
func (p gitPush) targetBranch(currentBranch func() string) (branch, unsure string) {
	if !p.explicit {
		return currentBranch(), ""
	}
	for _, spec := range p.refspecs {
		if !plainRefspec(spec) {
			name, isVar := refspecVar(spec)
			readsAsCurrent := currentBranchWord(spec) || (isVar && p.branchVars[name])
			if p.moved || !readsAsCurrent {
				unsure = cmp.Or(unsure, spec)
				continue
			}
			spec = "HEAD"
		}
		if spec = destBranch(spec); spec == "HEAD" {
			spec = currentBranch()
		}
		if defaultBranches[spec] {
			return spec, ""
		}
	}
	return "", unsure
}

// destBranch returns the branch name a plain refspec pushes to, the way git
// reads it: the side after the last colon (the source when there is none),
// without a leading +, and without refs/heads/ or the heads/ git expands to
// it. @ is HEAD, as it is to git.
func destBranch(spec string) string {
	spec = strings.TrimPrefix(spec, "+")
	if i := strings.LastIndex(spec, ":"); i >= 0 {
		spec = spec[i+1:]
	}
	if name, ok := strings.CutPrefix(spec, "refs/heads/"); ok {
		return name
	}
	if spec == "@" {
		return "HEAD"
	}
	return strings.TrimPrefix(spec, "heads/")
}

// refspecShellChars are the characters a refspec token still holds when
// placedWord.token could not reduce it to plain text: an expansion, a quote
// around text with shell characters, an escape, a brace or a glob the shell
// expands, a ( glued on.
const refspecShellChars = "$`\"'\\{}*?[]()<>"

// plainRefspec reports whether a refspec token is text the shell passes as
// written. A ~ stays plain: HEAD~1:feature names its destination outright.
func plainRefspec(spec string) bool {
	return !strings.ContainsAny(spec, refspecShellChars)
}

// currentBranchCommands print the branch checked out where they run; a
// branch name holds no colon or space, so the word is that branch.
var currentBranchCommands = map[string]bool{
	"git branch --show-current":       true,
	"git rev-parse --abbrev-ref HEAD": true,
	"git symbolic-ref --short HEAD":   true,
}

// currentBranchWord reports whether a refspec token is one substitution of a
// currentBranchCommands command, in double quotes or bare:
// "$(git branch --show-current)" or `git branch --show-current`.
func currentBranchWord(spec string) bool {
	if inner, ok := strings.CutPrefix(spec, `"`); ok {
		if spec, ok = strings.CutSuffix(inner, `"`); !ok {
			return false
		}
	}
	var body string
	switch {
	case strings.HasPrefix(spec, "$(") && strings.HasSuffix(spec, ")"):
		body = spec[2 : len(spec)-1]
	case len(spec) > 1 && spec[0] == '`' && spec[len(spec)-1] == '`':
		body = spec[1 : len(spec)-1]
	default:
		return false
	}
	return currentBranchCommands[strings.Join(strings.Fields(body), " ")]
}

// pushFlagsWithValue are `git push` flags that consume the next token, so the
// token must not be mistaken for a remote or refspec. --force-with-lease is
// absent on purpose: it takes the = form only.
var pushFlagsWithValue = map[string]bool{
	"-o": true, "--push-option": true, "--receive-pack": true, "--exec": true,
	"--repo": true,
}

// parsePushArgs reads the remote and refspecs out of a parsed push.
func parsePushArgs(c gitInvocation) gitPush {
	push := gitPush{dir: c.dir, configured: c.configured, unread: c.unread, moved: c.moved}
	var positional []string
	for i := 0; i < len(c.args); i++ {
		tok := c.args[i]
		if strings.HasPrefix(tok, "-") {
			switch tok {
			case "--all", "--branches":
				push.all = true
			case "--tags":
				push.tags = true
			case "--mirror":
				push.mirror = true
			case "--delete", "-d":
				push.del = true
			}
			push.repoFlag = push.repoFlag || tok == "--repo" || strings.HasPrefix(tok, "--repo=")
			if pushFlagsWithValue[tok] {
				i++
			}
			continue
		}
		positional = append(positional, tok)
	}
	// First positional is the remote; the rest are refspecs.
	if len(positional) > 0 {
		push.remote = positional[0]
	}
	if len(positional) > 1 {
		push.refspecs = positional[1:]
		push.explicit = true
	}
	return push
}

// canonicalPushRepoAt resolves where origin pushes to from the repo at dir:
// its push URL, with pushurl and the pushInsteadOf and insteadOf rewrites
// applied, as host/owner/repo. "" when there is no resolvable origin.
func canonicalPushRepoAt(dir string) string {
	return gitloc.CanonicalRepo(gitOutput(dir, "remote", "get-url", "--push", "origin"))
}

// gitPushRemote returns the remote a bare push of branch goes to from the
// repo at dir, as git picks it: branch.<name>.pushRemote, remote.pushDefault
// or branch.<name>.remote, else origin. "" when git cannot tell, as for a
// branch with no commit yet.
func gitPushRemote(dir, branch string) string {
	if branch == "" {
		return ""
	}
	out := gitOutput(
		dir,
		"for-each-ref",
		"--format=remote:%(push:remotename)",
		"refs/heads/"+branch,
	)
	name, ok := strings.CutPrefix(out, "remote:")
	switch {
	case !ok:
		return ""
	case name == "":
		return "origin"
	}
	return name
}

// segment is one piece of a compound shell command, with its byte offset in
// the original command so callers can scan from the segment onward.
type segment struct {
	text string
	off  int
}

// splitSegments splits a compound shell command on &&, ||, ;, | and newlines.
// Separators are replaced by same-length filler so offsets stay valid.
func splitSegments(command string) []segment {
	replaced := command
	for _, sep := range []string{"&&", "||", ";", "|", "\n"} {
		replaced = strings.ReplaceAll(replaced, sep, strings.Repeat("\x00", len(sep)))
	}
	var segs []segment
	pos := 0
	for _, piece := range strings.Split(replaced, "\x00") {
		if strings.TrimSpace(piece) != "" {
			segs = append(segs, segment{text: piece, off: pos})
		}
		pos += len(piece) + 1
	}
	return segs
}
