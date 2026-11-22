package guard

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
)

// fetchLossReason is what the model reads when a fetch may move or delete a
// local branch that holds a commit no remote-tracking ref holds; why says
// what the fetch does, the branch and the commit, or why belt could not
// tell (judgeFetch).
func fetchLossReason(why error) string {
	return "this " + why.Error() + ". " + gitGoAhead + ": let the user run it with `! <command>`."
}

// mirrorWorktreesReason is what the model reads when a fetch that prunes
// runs in a bare mirror with linked worktrees (fetchConfig.mirrorWorktrees):
// belt denies every one there, since the mirror's refspecs delete a branch
// made in a worktree when the remote lacks it.
var mirrorWorktreesReason = fetchLossReason(errors.New(
	"prunes local branches in a bare mirror that has linked worktrees: the mirror's refspecs delete " +
		"each branch the remote lacks, with its reflog, one made in a linked worktree too, and belt " +
		"does not check which of them hold work; a fetch that does not prune (`--no-prune`) passes",
))

// fetchTarget names local branches a fetch may force-update or, when
// prunes is set, delete: what names them in a reason, and rev names them
// to rev-list.
type fetchTarget struct {
	what, rev string
	prunes    bool
}

// verb says what a fetch does to the branches t names, for a reason.
func (t fetchTarget) verb() string {
	if t.prunes {
		return "prunes local branches"
	}
	return "force-fetches into a local branch"
}

// deletes says what a prune through t deletes, for a reason: the one
// branch a refs/heads/<name> rev names and its reflog, or the branches and
// their reflogs.
func (t fetchTarget) deletes() string {
	if strings.HasPrefix(t.rev, "refs/heads/") {
		return t.what + " and its reflog where the remote lacks it"
	}
	return t.what + " and their reflogs where the remote lacks them"
}

// anyBranch stands for the destinations of a fetch belt cannot read.
var anyBranch = fetchTarget{
	what: "any local branch, since belt cannot read where it fetches to",
	rev:  "--branches",
}

// pruning is what a fetch's words say about pruning.
type pruning int

const (
	// pruneFromConfig leaves it to the settings and the config files.
	pruneFromConfig pruning = iota
	pruneOn
	pruneOff
)

// fetchWords is what the words of a fetch say about the refs it updates:
// the refspec words it takes from its operands and --refmap, the refspecs
// the call itself writes before it fetches (written, as git reads them),
// whether -f forces every refspec and what --prune or --no-prune says.
// anyRef marks words belt cannot read, which may reach any branch; dry
// marks a --dry-run, which updates none; pruneOnly marks a `remote prune`,
// which moves no branch and only deletes (refspecUse).
type fetchWords struct {
	specs     []string
	written   []string
	forced    bool
	prune     pruning
	anyRef    bool
	dry       bool
	pruneOnly bool
}

// readFetchCall reads a git call as the fetch it runs, ok false for one
// that runs none: a fetch or pull (readFetch) and a git remote call
// (readRemoteFetch).
func readFetchCall(c gitInvocation, fed bool) (fetchWords, bool) {
	switch c.sub {
	case "fetch", "pull":
		return readFetch(c, fed), true
	case "remote":
		return readRemoteFetch(c.args, fed)
	}
	return fetchWords{}, false
}

// readFetch reads the words of a fetch or pull call as git reads them
// (readGitWords, fetchOptions or pullOptions). The first operand is the
// repository, `tag <name>` fetches a tag, and a fetch's --multiple makes
// every operand a repository; each --refmap value maps the fetched refs the
// way a refspec does. Words belt cannot read, a fetch's --stdin, whose
// refspecs belt never sees, and words a command in front of git may hand
// over (fed) may reach any branch.
func readFetch(c gitInvocation, fed bool) fetchWords {
	table := fetchOptions
	if c.sub == "pull" {
		table = pullOptions
	}
	w, err := readGitWords(c.args, table)
	switch {
	case err != nil || fed || w.on("stdin"):
		return fetchWords{anyRef: true}
	case w.on("dry-run"):
		return fetchWords{dry: true}
	}
	f := fetchWords{forced: w.on("force"), prune: readPrune(w)}
	if operands := slices.Concat(w.operands, w.paths); len(operands) > 1 && !w.on("multiple") {
		f.specs = dropTagFetches(operands[1:])
	}
	f.specs = append(f.specs, w.values("refmap")...)
	return f
}

// readRemoteFetch reads a git remote call as the fetch it runs, ok false
// for one that runs none: `remote update` (readRemoteUpdate), `remote
// prune` (readRemotePrune) and `remote add -f` (readRemoteAdd). None has
// refspec words of its own, so the settings and the repository's refspecs
// count. A subcommand word the shell may still rewrite, or one a command in
// front of git may hand over (fed), may make the call an update that
// prunes.
func readRemoteFetch(args []string, fed bool) (fetchWords, bool) {
	sub, rest, unsure := remoteSubcommand(args)
	switch {
	case unsure || (fed && sub == ""):
		return fetchWords{prune: pruneOn}, true
	case sub == "update":
		return readRemoteUpdate(rest, fed), true
	case sub == "prune":
		return readRemotePrune(rest, fed), true
	case sub == "add":
		return readRemoteAdd(rest, fed)
	}
	return fetchWords{}, false
}

// readRemoteUpdate reads the words after `remote update`, which fetches the
// remotes or groups it names, or every remote: it prunes as -p or
// --no-prune says, and words belt cannot read may ask for a prune.
func readRemoteUpdate(rest []string, fed bool) fetchWords {
	w, err := readGitWords(rest, remoteUpdateOptions)
	if err != nil || fed {
		return fetchWords{prune: pruneOn}
	}
	return fetchWords{prune: readPrune(w)}
}

// readRemotePrune reads the words after `remote prune`, which fetches
// nothing and deletes the local refs the remote's refspecs fill that the
// remote lacks (fetchWords.pruneOnly). -n or --dry-run deletes none, and
// words belt cannot read may not be one.
func readRemotePrune(rest []string, fed bool) fetchWords {
	w, err := readGitWords(rest, remotePruneOptions)
	if err == nil && !fed && w.on("dry-run") {
		return fetchWords{dry: true}
	}
	return fetchWords{prune: pruneOn, pruneOnly: true}
}

// readRemoteAdd reads the words after `remote add` as the fetch its -f or
// --fetch runs, ok false without one: git fetches the new remote through
// the refspecs the add writes (mirrorSettings), pruning as the settings and
// the config files say. A fetch mirror's refspecs force every branch they
// name; any other add's fill only refs/remotes/<name>/. Words belt cannot
// read, or a command in front of git may hand over, may fetch as a mirror.
func readRemoteAdd(rest []string, fed bool) (fetchWords, bool) {
	w, err := readGitWords(rest, remoteAddOptions)
	unread := err != nil || fed
	if !unread && !w.on("fetch") {
		return fetchWords{}, false
	}
	var f fetchWords
	for _, s := range mirrorSettings(rest, unread) {
		f.written = append(f.written, s.value)
	}
	return f, true
}

// readPrune reads what --prune and --no-prune in a call's words say, the
// last deciding.
func readPrune(w gitWords) pruning {
	o, ok := w.last("prune")
	switch {
	case !ok:
		return pruneFromConfig
	case o.unset:
		return pruneOff
	}
	return pruneOn
}

// prunes reports whether the fetch may prune: its words ask for it, or
// leave it to the settings and the config files, and repo, what the config
// files say (fetchConfig.prune), is set or a setting may turn it on
// (pruneSetting). --prune-tags adds refs/tags/*:refs/tags/* to a fetch that
// prunes, and prunes nothing without --prune, so it reaches no branch.
func (f fetchWords) prunes(settings []callSetting, repo bool) bool {
	switch f.prune {
	case pruneOn:
		return true
	case pruneOff:
		return false
	}
	return repo || slices.ContainsFunc(settings, pruneSetting)
}

// refspecUse is how a fetch uses its refspecs (specTarget): forced says -f
// forces every one, prunes that the fetch prunes, and pruneOnly that it is
// a `remote prune`, which moves no branch, forced or not, and deletes
// through an exact refspec too.
type refspecUse struct {
	forced, prunes, pruneOnly bool
}

// fetchTargets returns the local branches a fetch may move or delete
// (specTarget). The refspecs are the call's own words (wordTarget), the
// ones it writes before it fetches (fetchWords.written), each
// remote.<name>.fetch setting it runs with (settingTarget), and
// repo.refspecs, the ones its config files hold. A configured refspec
// counts with refspecs on the command line too: git still maps the fetched
// refs through it. Each rev counts once.
func fetchTargets(f fetchWords, settings []callSetting, repo fetchConfig) []fetchTarget {
	if f.anyRef {
		return []fetchTarget{anyBranch}
	}
	use := refspecUse{forced: f.forced, prunes: f.prunes(settings, repo.prune), pruneOnly: f.pruneOnly}
	var out []fetchTarget
	add := func(t fetchTarget, ok bool) {
		if ok && !slices.ContainsFunc(out, func(o fetchTarget) bool { return o.rev == t.rev }) {
			out = append(out, t)
		}
	}
	for _, spec := range f.specs {
		add(wordTarget(spec, use))
	}
	for _, s := range settings {
		add(settingTarget(s, use))
	}
	for _, spec := range slices.Concat(f.written, repo.refspecs) {
		add(specTarget(spec, use))
	}
	return out
}

// dropTagFetches drops each `tag <name>` pair from a fetch's refspec words:
// it fetches a tag into the tag of that name.
func dropTagFetches(words []string) []string {
	var out []string
	for i := 0; i < len(words); i++ {
		if unquoteWord(words[i]) == "tag" {
			i++
			continue
		}
		out = append(out, words[i])
	}
	return out
}

// wordTarget reads one refspec word as written (specTarget). A word the
// shell may still rewrite, other than by the * of a refspec pattern, may
// reach any branch.
func wordTarget(raw string, use refspecUse) (fetchTarget, bool) {
	if mayExpandIgnoringStar(raw) {
		return anyBranch, true
	}
	return specTarget(unquoteWord(raw), use)
}

// settingTarget reads a setting a fetch runs with: a remote.<name>.fetch
// value is a refspec (specTarget). A key the shell may still rewrite may be
// one, and a value only the environment or a shell holds may be any
// refspec.
func settingTarget(s callSetting, use refspecUse) (fetchTarget, bool) {
	if !s.anyKey && !remoteKey(s.key, "fetch") {
		return fetchTarget{}, false
	}
	if s.anyValue {
		return anyBranch, true
	}
	return specTarget(s.value, use)
}

// remoteKey reports whether a lower-cased config key is remote.<name>.<sub>
// for some remote name.
func remoteKey(key, sub string) bool {
	return strings.HasPrefix(key, "remote.") && strings.HasSuffix(key, "."+sub) &&
		strings.Count(key, ".") >= 2
}

// remoteName returns the remote a remote.<name>.<sub> key names (remoteKey).
func remoteName(key string) string {
	return key[len("remote."):strings.LastIndex(key, ".")]
}

// pruneSetting reports whether a setting may turn pruning on: fetch.prune
// or a remote.<name>.prune that git may read as true (gitFalse). A key the
// shell may still rewrite may be one, and a value only the environment or a
// shell holds may be true.
func pruneSetting(s callSetting) bool {
	return (s.anyKey || s.key == "fetch.prune" || remoteKey(s.key, "prune")) &&
		(s.anyValue || !gitFalse(s.value))
}

// gitFalse reports whether git reads a config value as false: false, no,
// off, an empty value or a number that is 0.
func gitFalse(v string) bool {
	switch strings.ToLower(v) {
	case "false", "no", "off", "":
		return true
	}
	n, err := strconv.ParseInt(v, 0, 64)
	return err == nil && n == 0
}

// specTarget reads one refspec: the local branches a fetch moves or deletes
// through it (specBranches), ok false for none. A leading + or -f forces
// the branches its destination names, and a fetch that prunes deletes the
// ones a pattern destination names that the remote lacks, forced or not. A
// fetch prunes nothing through an exact refspec: git refuses it when the
// remote lacks the source. A `remote prune` (use.pruneOnly) moves nothing,
// and deletes the branch an exact refspec fills too when the destination
// is written as refs/heads/<name>: git compares each local ref with the
// destination as written, and the remote's refs with the source as
// written, so `main:refs/heads/x` deletes x even when the remote has main.
func specTarget(spec string, use refspecUse) (fetchTarget, bool) {
	t, ok := specBranches(spec)
	dst, _ := fetchDst(spec)
	switch {
	case !ok:
		return fetchTarget{}, false
	case !use.pruneOnly && (use.forced || strings.HasPrefix(spec, "+")):
		return t, true
	case use.prunes && strings.Contains(spec, "*"),
		use.pruneOnly && strings.HasPrefix(dst, "refs/heads/"):
		t.prunes = true
		return t, true
	}
	return fetchTarget{}, false
}

// specBranches returns the local branches a refspec's destination names,
// ok false for none. A destination pattern that covers refs/heads/ from
// above it (`refs/*`) may reach every branch, and so may a branch name
// rev-list would read as more than a ref (`..`, `:`, a suffix) or one that
// is not plain text.
func specBranches(spec string) (fetchTarget, bool) {
	dst, ok := fetchDst(spec)
	if !ok {
		return fetchTarget{}, false
	}
	if above, _, pattern := strings.Cut(dst, "*"); pattern && above != "" &&
		len(above) < len("refs/heads/") && strings.HasPrefix("refs/heads/", above) {
		return fetchTarget{what: "every branch `" + dst + "` covers", rev: "--branches"}, true
	}
	name, ok := fetchBranch(dst)
	switch {
	case !ok:
		return fetchTarget{}, false
	case !asWritten(strings.ReplaceAll(name, "*", "")) || strings.Contains(name, ":") ||
		strings.Contains(name, ".."):
		return anyBranch, true
	case strings.Contains(name, "*"):
		return fetchTarget{
			what: "the branches `refs/heads/" + name + "`",
			rev:  "--glob=refs/heads/" + name,
		}, true
	}
	return fetchTarget{what: "the branch `" + name + "`", rev: "refs/heads/" + name}, true
}

// fetchBranch returns the branch a fetch destination names, as git reads it
// (get_local_ref): refs/heads/<name> and heads/<name> name it, any other
// refs/, tags/ or remotes/ name is no branch, and every other name is the
// branch of that name. An empty name is no branch: git refuses it.
func fetchBranch(dst string) (string, bool) {
	for _, prefix := range []string{"refs/heads/", "heads/"} {
		if name, ok := strings.CutPrefix(dst, prefix); ok {
			return name, name != ""
		}
	}
	for _, prefix := range []string{"refs/", "tags/", "remotes/"} {
		if strings.HasPrefix(dst, prefix) {
			return "", false
		}
	}
	return dst, dst != ""
}

// judgeFetch returns nil when no target holds a commit that no
// remote-tracking ref holds (GitDiscard.unpushed), else what the fetch does
// to the first one that does. A branch that does not exist holds none. ctx
// bounds every count.
func (g *GitDiscard) judgeFetch(ctx context.Context, dir string, targets []fetchTarget) error {
	if dir == "" {
		return errors.New(targets[0].verb() + ": belt cannot tell which repository it runs in")
	}
	for _, t := range targets {
		commit, err := g.unpushed(ctx, dir, t.rev)
		switch {
		case err != nil:
			return fmt.Errorf("%s: belt cannot tell what %s holds (%w)", t.verb(), t.what, err)
		case commit == "":
			continue
		}
		short := commit[:min(len(commit), shortOIDLen)]
		if t.prunes {
			return fmt.Errorf(
				"%s: it may delete %s, with commit %s, which no remote-tracking ref holds; push it first",
				t.verb(), t.deletes(), short,
			)
		}
		return fmt.Errorf(
			"%s: it may move %s off commit %s, which no remote-tracking ref holds; "+
				"push it or run `git branch <name> %s` first",
			t.verb(), t.what, short, short,
		)
	}
	return nil
}

// unpushedCommit returns a commit the branches rev names hold that no
// remote-tracking ref holds, "" when there is none: `rev-list -n 1
// --ignore-missing <rev> --not --remotes`, so a branch that does not exist
// holds none.
func unpushedCommit(ctx context.Context, dir, rev string) (string, error) {
	out, err := runProofGit(
		ctx,
		dir,
		"rev-list",
		"-n",
		"1",
		"--ignore-missing",
		rev,
		"--not",
		"--remotes",
	)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// fetchConfig is what the config files git reads in a repository say about
// its fetches (parseFetchConfig).
type fetchConfig struct {
	// refspecs are the remote.<name>.fetch values that count: each remote's,
	// and a mirror's only where git reads the repository as no bare one.
	refspecs []string
	// prune says fetch.prune or a remote.<name>.prune may be true.
	prune bool
	// bareMirrors are the refspecs of each mirror remote when core.bare is
	// true. They count for none only when git reads the directory as a
	// bare repository: a linked worktree of one reads core.bare as true
	// too, and there the user works on the branches (repoFetchConfig).
	bareMirrors []string
	// mirrorWorktrees says the mirror's refspecs counted for none in a bare
	// repository that has linked worktrees: a fetch that prunes through
	// them deletes a branch made in a worktree when the remote lacks it,
	// so checkFetch denies one (mirrorWorktreesReason).
	mirrorWorktrees bool
}

// fetchConfigKeys matches the keys parseFetchConfig reads, as git config
// --get-regexp writes them, with section and key in lower case.
const fetchConfigKeys = `^(remote\..+\.(fetch|mirror|prune)|fetch\.prune|core\.bare)$`

// repoFetchConfig reads what the config files git reads in dir say about
// its fetches with one `git config --null --get-regexp`, where status 1
// means none of the keys is set. When it holds a mirror remote and
// core.bare is true, one `git rev-parse` decides whether the mirror's
// refspecs count (gitBareRepo): only a bare repository drops them, and one
// with linked worktrees marks it (fetchConfig.mirrorWorktrees).
func repoFetchConfig(ctx context.Context, dir string) (fetchConfig, error) {
	out, code, err := runProofGitCode(ctx, dir, nil, "config", "--null", "--get-regexp", fetchConfigKeys)
	switch {
	case err != nil:
		return fetchConfig{}, err
	case code == 1:
		return fetchConfig{}, nil
	case code != 0:
		return fetchConfig{}, fmt.Errorf("git config exited %d", code)
	}
	cfg := parseFetchConfig(nulFields(out))
	if len(cfg.bareMirrors) == 0 {
		return cfg, nil
	}
	bare, worktrees := gitBareRepo(ctx, dir)
	if !bare {
		cfg.refspecs = append(cfg.refspecs, cfg.bareMirrors...)
	}
	cfg.mirrorWorktrees = bare && worktrees
	cfg.bareMirrors = nil
	return cfg, nil
}

// gitBareRepo reports whether git reads dir as a bare repository and, for
// one, whether it has linked worktrees (hasLinkedWorktrees), with one `git
// rev-parse --is-bare-repository --git-common-dir`. A call that fails or
// runs past ctx counts as no bare repository, so a mirror's refspecs then
// count: the check fails closed.
func gitBareRepo(ctx context.Context, dir string) (bare, worktrees bool) {
	out, err := runProofGit(ctx, dir, "rev-parse", "--is-bare-repository", "--git-common-dir")
	if err != nil {
		return false, false
	}
	isBare, common, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	if isBare != "true" {
		return false, false
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(dir, common)
	}
	return true, hasLinkedWorktrees(common)
}

// hasLinkedWorktrees reports whether the repository whose common git
// directory is common has a linked worktree, as git lists them: an entry
// under <common>/worktrees whose folder holds a gitdir file. No worktrees
// folder means none; a read that fails counts as one, so a fetch that
// prunes then denies.
func hasLinkedWorktrees(common string) bool {
	root := filepath.Join(common, "worktrees")
	entries, err := os.ReadDir(root)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false
	case err != nil:
		return true
	}
	for _, e := range entries {
		_, err := os.Stat(filepath.Join(root, e.Name(), "gitdir"))
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) {
			return true
		}
	}
	return false
}

// parseFetchConfig reads `git config --null --get-regexp fetchConfigKeys`
// entries: a key, a newline and its value, or a key alone for one set with
// no value, which git reads as true. The last value of a mirror, prune or
// core.bare key decides, as git reads the config files in order. A remote
// whose remote.<name>.mirror is true is a mirror, as `git clone --mirror`
// sets one up: in a bare repository its refspecs (+refs/*:refs/*) keep each
// local ref a copy of the remote's by design, so with core.bare true they
// go to bareMirrors. In a repository with a work tree, as `git remote add
// --mirror` writes one, they move and delete the user's branches, and
// count.
func parseFetchConfig(entries []string) fetchConfig {
	var (
		names  []string
		specs  = map[string][]string{}
		mirror = map[string]bool{}
		prune  = map[string]bool{}
		bare   bool
	)
	for _, entry := range entries {
		key, value, valued := strings.Cut(entry, "\n")
		switch {
		case remoteKey(key, "fetch") && valued:
			name := remoteName(key)
			if _, seen := specs[name]; !seen {
				names = append(names, name)
			}
			specs[name] = append(specs[name], value)
		case remoteKey(key, "mirror"):
			mirror[remoteName(key)] = !valued || gitTrue(value)
		case key == "fetch.prune" || remoteKey(key, "prune"):
			prune[key] = !valued || !gitFalse(value)
		case key == "core.bare":
			bare = !valued || gitTrue(value)
		}
	}
	var cfg fetchConfig
	for _, name := range names {
		if mirror[name] && bare {
			cfg.bareMirrors = append(cfg.bareMirrors, specs[name]...)
		} else {
			cfg.refspecs = append(cfg.refspecs, specs[name]...)
		}
	}
	for _, on := range prune {
		cfg.prune = cfg.prune || on
	}
	return cfg
}
