package guard

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/mcptool"
)

// PublishInternalNamesID identifies the publish-time internal-name firewall.
const PublishInternalNamesID = "publish-internal-names"

// PublishInternalNames blocks internal org/repo/host names on their way to a
// public (github.com) remote. write-internal-names guards the file being
// written; this guard covers everything else an agent publishes: PR and
// issue text, comments, branch and tag names, commit messages, and content
// pushed through the gh MCP without a local commit. One id is registered on
// two events: bash (git push, git commit, branch and tag creation, gh) and
// external-text (the gh MCP tools the consuming repo routes here, which the
// guard treats as public by construction). The name set is the same
// derivation write-internal-names uses, and per-guard allow_repos exempts a
// private companion repo by canonical identity.
type PublishInternalNames struct {
	cfg   config.Config
	event string
	// git runs one git command in dir and returns its trimmed stdout, or ""
	// on any failure. Every repo, branch, and commit lookup goes through it.
	// Injectable for tests.
	git func(dir string, args ...string) string
	// readFile reads a file a command references (--body-file, commit -F).
	// Injectable for tests.
	readFile func(path string) (string, error)
	// emit sends the soft-mode warning to the events service. Injectable
	// for tests.
	emit func(source, level, title, message string, tags map[string]string)
}

// NewPublishInternalNames builds the guard for one of its two events with
// the real git, filesystem, and events backends.
func NewPublishInternalNames(cfg config.Config, event string) *PublishInternalNames {
	return &PublishInternalNames{
		cfg:      cfg,
		event:    event,
		git:      gitOutput,
		readFile: readFileString,
		emit:     notify.EmitEventSync,
	}
}

func (g *PublishInternalNames) ID() string    { return PublishInternalNamesID }
func (g *PublishInternalNames) Event() string { return g.event }

// publication is one piece of text about to reach a public remote, with
// enough context for the deny reason to say where the name was found.
type publication struct {
	action string // what is being done: "git push origin feat", "gh pr create", an MCP tool name
	repo   string // canonical target repo; "" when the action has none (a gist, a new repo)
	where  string // the part carrying the text: "branch name x", "commit 1a2b3c4 message", "field body"
	text   string
}

// Check collects the text the tool call would publish, then matches the
// blocked-name set against it. Parsing comes first and derivation second on
// purpose: deriving the name set walks the workspace dirs, and most bash
// calls publish nothing.
func (g *PublishInternalNames) Check(in Input) *Denial {
	var pubs []publication
	if g.event == EventExternalText {
		pubs = g.toolPublications(in)
	} else {
		pubs = g.bashPublications(in)
	}
	if len(pubs) == 0 {
		return nil
	}
	m := newNameMatcher(g.cfg.Names)
	if m.empty() {
		return nil
	}
	var first *publication
	var found []string
	for i := range pubs {
		hits := m.hits(pubs[i].text)
		if len(hits) == 0 {
			continue
		}
		if first == nil {
			first = &pubs[i]
		}
		found = append(found, pubs[i].where+": "+summarizeHits(hits))
	}
	if first == nil {
		return nil
	}
	target := "a public repo"
	if first.repo != "" {
		target = "public repo " + first.repo
	}
	reason := fmt.Sprintf("%s would publish internal names to %s (%s). "+
		"Internal org/repo/host names must never reach a public repo — "+
		"rename, reword, or drop them before retrying.",
		first.action, target, strings.Join(found, "; "))
	if g.cfg.Guards[PublishInternalNamesID].Soft() {
		g.emit(
			"belt",
			"warn",
			"publish-internal-names (soft)",
			fmt.Sprintf("belt[%s]: %s Allowed (soft mode).", PublishInternalNamesID, reason),
			map[string]string{
				"guard": PublishInternalNamesID,
				"repo":  first.repo,
				"event": g.event,
			},
		)
		return nil
	}
	return Reasonf(PublishInternalNamesID, "%s", reason)
}

// --- external-text: the gh MCP tools ---------------------------------------

// toolPublications turns an MCP tool call into publications: every string
// in the input except the owner/repo pair that names the target. Read-only
// operations publish nothing, so a server-wide matcher costs nothing on
// fetches. The target decides as it does for gh: public-bound by the
// config's rule and not allowlisted. A call with no resolvable target (a
// new repository, a gist) follows the unknown-target rule: guarded under
// the legacy host rule, allowed once a public_repos list names the repos
// that matter.
func (g *PublishInternalNames) toolPublications(in Input) []publication {
	if !mcptool.Publishes(in.ToolName) {
		return nil
	}
	repo := toolRepo(in.ToolInput)
	if repo == "" && !g.unknownTargetGuarded() {
		return nil
	}
	if repo != "" && !g.guarded(repo) {
		return nil
	}
	var pubs []publication
	for _, key := range slices.Sorted(maps.Keys(in.ToolInput)) {
		if key == "owner" || key == "repo" {
			continue
		}
		text := collectStrings(in.ToolInput[key])
		if text == "" {
			continue
		}
		pubs = append(pubs, publication{
			action: in.ToolName, repo: repo, where: "field " + key, text: text,
		})
	}
	return pubs
}

// toolRepo resolves the canonical target of a gh MCP call from its
// owner/repo pair. The event's matcher only admits the github.com server,
// so the host is fixed.
func toolRepo(input map[string]any) string {
	owner, _ := input["owner"].(string)
	repo, _ := input["repo"].(string)
	if owner == "" || repo == "" {
		return ""
	}
	return "github.com/" + owner + "/" + repo
}

// collectStrings joins every string inside v, nested maps and arrays
// included, one per line. A label list, a reviewer team slug, and a
// push_files content entry all count: each is text that leaves the machine.
func collectStrings(v any) string {
	var parts []string
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			if t != "" {
				parts = append(parts, t)
			}
		case map[string]any:
			for _, key := range slices.Sorted(maps.Keys(t)) {
				walk(t[key])
			}
		case []any:
			for _, item := range t {
				walk(item)
			}
		}
	}
	walk(v)
	return strings.Join(parts, "\n")
}

// --- bash: git push, git commit, branch and tag creation, gh ---------------

// bashPublications walks the command's segments, tracking `cd` like
// git-push-main does, and collects what each git or gh invocation would
// publish. A heredoc body spans several segments, which is why the command
// text (not one segment) is what gets scanned for commit and gh text.
func (g *PublishInternalNames) bashPublications(in Input) []publication {
	if in.Command == "" {
		return nil
	}
	var pubs []publication
	cwd := in.Cwd
	for _, seg := range splitSegments(in.Command) {
		tokens := strings.Fields(seg.text)
		if dir, ok := parseCd(tokens); ok {
			cwd = resolveDir(cwd, dir)
			continue
		}
		for i, tok := range tokens {
			switch tok {
			case "git":
				if c, ok := parseGitCmd(tokens[i+1:]); ok {
					pubs = append(pubs, g.gitPublications(in.Command, c, cwd)...)
				}
			case "gh":
				if c, ok := parseGh(tokens[i+1:]); ok {
					pubs = append(pubs, g.ghPublications(in.Command, c, cwd)...)
				}
			default:
				continue
			}
			break // one invocation per segment, as findGitCommands does
		}
	}
	return pubs
}

// gitPublications dispatches one git invocation by subcommand.
func (g *PublishInternalNames) gitPublications(
	command string,
	c gitInvocation,
	cwd string,
) []publication {
	dir := c.dir
	if dir == "" {
		dir = cwd
	}
	switch c.sub {
	case "push":
		return g.pushPublications(c, dir)
	case "commit":
		return g.commitPublications(command, c, dir)
	case "tag":
		return g.tagPublications(command, c, dir)
	case "checkout", "switch", "branch", "worktree":
		return g.branchPublications(c, dir)
	}
	return nil
}

// publicRepoAt resolves the origin of the repo at dir and returns its
// canonical identity when it is public and not allowlisted, "" otherwise.
func (g *PublishInternalNames) publicRepoAt(dir string) string {
	return g.publicRemote(dir, "origin")
}

// publicRemote is publicRepoAt for a named remote: the push target.
func (g *PublishInternalNames) publicRemote(dir, remote string) string {
	repo := canonicalRepo(g.git(dir, "remote", "get-url", remote))
	if !g.guarded(repo) {
		return ""
	}
	return repo
}

// guarded reports whether internal names must stay out of the canonical
// repo: public-bound by the config's rule and not on the guard's
// allow_repos.
func (g *PublishInternalNames) guarded(repo string) bool {
	return g.cfg.PublicBound(repo) && !g.cfg.RepoAllowed(PublishInternalNamesID, repo)
}

// unknownTargetGuarded decides an action whose target repo cannot be named
// (a gist, a raw api call, a new repository without an owner, an MCP call
// without owner/repo). Under the legacy host rule everything on github.com
// is public-bound, so the unknown target is guarded; with a public_repos
// list the unknown target is by definition not on it.
func (g *PublishInternalNames) unknownTargetGuarded() bool {
	return !g.cfg.HasPublicRepos()
}

// commitPublications covers `git commit -m` and `-F`: the message travels
// with the next push, so the check happens where it is cheapest to reword.
// A commit that opens an editor carries no text to scan.
func (g *PublishInternalNames) commitPublications(
	command string,
	c gitInvocation,
	dir string,
) []publication {
	hasMessage := hasFlag(c.args, "-m", "--message")
	file, _ := flagValue(c.args, "-F", "--file")
	if !hasMessage && file == "" {
		return nil
	}
	repo := g.publicRepoAt(dir)
	if repo == "" {
		return nil
	}
	pubs := []publication{{action: "git commit", repo: repo, where: "command text", text: command}}
	if body, ok := g.readReferenced(dir, file); ok {
		pubs = append(pubs, publication{
			action: "git commit", repo: repo, where: "message file " + file, text: body,
		})
	}
	return pubs
}

// tagPublications covers tag creation: the name and any -m message. Listing,
// verifying, and deleting tags publish nothing.
func (g *PublishInternalNames) tagPublications(
	command string,
	c gitInvocation,
	dir string,
) []publication {
	if hasFlag(c.args, "-d", "--delete", "-l", "--list", "-v", "--verify") ||
		len(positionals(c.args)) == 0 {
		return nil
	}
	repo := g.publicRepoAt(dir)
	if repo == "" {
		return nil
	}
	return []publication{{action: "git tag", repo: repo, where: "command text", text: command}}
}

// branchPublications covers the commands that mint a branch name: checkout
// -b, switch -c, branch <name>, worktree add -b. The name becomes public on
// the next push; catching it here means a rename instead of a force-push.
func (g *PublishInternalNames) branchPublications(c gitInvocation, dir string) []publication {
	names := newBranchNames(c)
	if len(names) == 0 {
		return nil
	}
	repo := g.publicRepoAt(dir)
	if repo == "" {
		return nil
	}
	var pubs []publication
	for _, name := range names {
		pubs = append(pubs, publication{
			action: "git " + c.sub, repo: repo, where: "branch name " + name, text: name,
		})
	}
	return pubs
}

// branchListingFlags are the `git branch` forms that list, delete, move, or
// configure rather than create.
var branchListingFlags = []string{
	"-d", "-D", "--delete", "-m", "-M", "--move", "-c", "-C", "--copy",
	"-l", "--list", "-a", "--all", "-r", "--remotes", "-u", "--set-upstream-to",
	"--unset-upstream", "--edit-description", "--show-current",
}

// newBranchNames extracts the branch names a git invocation would create.
func newBranchNames(c gitInvocation) []string {
	switch c.sub {
	case "checkout":
		return flagValues(c.args, "-b", "-B", "--orphan")
	case "switch":
		return flagValues(c.args, "-c", "-C", "--create", "--force-create", "--orphan")
	case "worktree":
		if len(c.args) > 0 && c.args[0] == "add" {
			return flagValues(c.args[1:], "-b", "-B")
		}
	case "branch":
		if hasFlag(c.args, branchListingFlags...) {
			return nil
		}
		if pos := positionals(c.args); len(pos) > 0 {
			return pos[:1]
		}
	}
	return nil
}

// outgoingLogLimit bounds the commit-message scan on a push. A branch with
// more new commits than this is not what an agent session produces, and a
// hook has to stay fast.
const outgoingLogLimit = "200"

// pushPublications covers `git push`: the names of the refs being pushed
// and the messages of the commits the remote does not have yet. Deletions
// publish nothing (they remove a name). The base for "new commits" is the
// remote-tracking ref of the pushed branch when it exists, else the
// remote's HEAD; with neither, only ref names are checked.
func (g *PublishInternalNames) pushPublications(c gitInvocation, dir string) []publication {
	push := parsePushArgs(c)
	if push.del {
		return nil
	}
	remote, upstreamBranch := push.remote, ""
	if remote == "" {
		remote, upstreamBranch = g.pushDefault(dir)
	}
	repo := g.publicRemote(dir, remote)
	if repo == "" {
		return nil
	}
	action := "git push " + remote
	if push.explicit {
		action += " " + strings.Join(push.refspecs, " ")
	}
	var pubs []publication
	add := func(where, text string) {
		pubs = append(pubs, publication{action: action, repo: repo, where: where, text: text})
	}
	for _, ref := range g.pushedRefs(push, dir, upstreamBranch) {
		add(ref.kind+" name "+ref.name, ref.name)
		if ref.local == "" {
			continue
		}
		base := g.pushBase(dir, remote, ref.name)
		if base == "" {
			continue
		}
		for _, commit := range g.outgoingCommits(dir, base, ref.local) {
			add("commit "+commit.sha+" message", commit.message)
		}
	}
	return pubs
}

// pushedRef is one ref a push would create or update on the remote: its
// name there, the local ref holding the commits (empty for --all/--tags
// sweeps, which check names only), and whether it is a branch or a tag.
type pushedRef struct {
	name  string
	local string
	kind  string // "branch" or "tag"
}

// pushedRefs resolves what a push sends: explicit refspecs, the current
// branch for a bare push, or every local branch and tag for the sweep
// flags.
func (g *PublishInternalNames) pushedRefs(push gitPush, dir, upstreamBranch string) []pushedRef {
	var refs []pushedRef
	if push.all || push.mirror {
		for _, name := range g.localRefs(dir, "refs/heads") {
			refs = append(refs, pushedRef{name: name, kind: "branch"})
		}
	}
	if push.tags || push.mirror {
		for _, name := range g.localRefs(dir, "refs/tags") {
			refs = append(refs, pushedRef{name: name, kind: "tag"})
		}
	}
	if push.all || push.mirror {
		return refs
	}
	if !push.explicit {
		if push.tags {
			return refs
		}
		name := upstreamBranch
		if name == "" {
			name = g.git(dir, "rev-parse", "--abbrev-ref", "HEAD")
		}
		if name == "" || name == "HEAD" {
			return refs
		}
		return append(refs, pushedRef{name: name, local: "HEAD", kind: "branch"})
	}
	for _, spec := range push.refspecs {
		if ref, ok := g.parseRefspec(dir, spec); ok {
			refs = append(refs, ref)
		}
	}
	return refs
}

// parseRefspec reads one push refspec into the ref it updates. A refspec
// with an empty source (":branch") deletes and is skipped; HEAD resolves to
// the current branch name.
func (g *PublishInternalNames) parseRefspec(dir, spec string) (pushedRef, bool) {
	spec = strings.TrimPrefix(spec, "+")
	src, dst, hasDst := strings.Cut(spec, ":")
	if src == "" {
		return pushedRef{}, false
	}
	name := src
	if hasDst && dst != "" {
		name = dst
	}
	kind := "branch"
	if strings.HasPrefix(name, "refs/tags/") || strings.HasPrefix(src, "refs/tags/") {
		kind = "tag"
	}
	name = strings.TrimPrefix(strings.TrimPrefix(name, "refs/heads/"), "refs/tags/")
	if name == "HEAD" {
		name = g.git(dir, "rev-parse", "--abbrev-ref", "HEAD")
		if name == "" || name == "HEAD" {
			return pushedRef{}, false
		}
	}
	return pushedRef{name: name, local: src, kind: kind}, true
}

// pushDefault resolves where a bare `git push` goes: the branch's push
// upstream when configured (remote and remote branch), else origin and the
// current branch.
func (g *PublishInternalNames) pushDefault(dir string) (remote, branch string) {
	upstream := g.git(dir, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{push}")
	if remote, branch, ok := strings.Cut(upstream, "/"); ok && remote != "" && branch != "" {
		return remote, branch
	}
	return "origin", ""
}

// pushBase finds the commit the remote already has for a pushed branch: its
// remote-tracking ref, else the remote's HEAD. "" means unknown, and then
// commit messages are not scanned rather than scanning all of history.
func (g *PublishInternalNames) pushBase(dir, remote, branch string) string {
	for _, ref := range []string{
		"refs/remotes/" + remote + "/" + branch,
		"refs/remotes/" + remote + "/HEAD",
	} {
		if g.git(dir, "rev-parse", "--verify", "--quiet", ref) != "" {
			return ref
		}
	}
	return ""
}

// outgoingCommit is one commit the push would publish.
type outgoingCommit struct {
	sha     string
	message string
}

// outgoingCommits lists the commits in base..local with their full
// messages, NUL-separated so a message may span lines.
func (g *PublishInternalNames) outgoingCommits(dir, base, local string) []outgoingCommit {
	out := g.git(dir, "log", "-z", "--format=%h%n%B", "-n", outgoingLogLimit, base+".."+local)
	var commits []outgoingCommit
	for record := range strings.SplitSeq(out, "\x00") {
		sha, message, _ := strings.Cut(strings.TrimSpace(record), "\n")
		if sha == "" {
			continue
		}
		commits = append(commits, outgoingCommit{sha: sha, message: message})
	}
	return commits
}

// localRefs lists the short names under a ref namespace (refs/heads,
// refs/tags).
func (g *PublishInternalNames) localRefs(dir, namespace string) []string {
	out := g.git(dir, "for-each-ref", "--format=%(refname:short)", namespace)
	return strings.Fields(out)
}

// --- gh -------------------------------------------------------------------

// ghInvocation is one parsed `gh` call: the command group (pr, issue, api),
// its subcommand (create, comment; the endpoint for api), and every token
// after `gh`.
type ghInvocation struct {
	group string
	sub   string
	args  []string
}

// ghPublishing lists the gh subcommands that put text on github.com. A nil
// list means every subcommand of the group publishes (api). Anything else
// (view, list, checkout, status, diff) reads.
var ghPublishing = map[string][]string{
	"pr":      {"create", "edit", "comment", "review", "merge", "close", "reopen"},
	"issue":   {"create", "edit", "comment", "close", "reopen", "develop"},
	"release": {"create", "edit"},
	"repo":    {"create", "edit", "rename"},
	"gist":    {"create", "edit", "rename"},
	"label":   {"create", "edit"},
	"api":     nil,
}

// parseGh reads the tokens after `gh`: the first two non-flag tokens are
// the group and subcommand. ok is false when no group follows.
func parseGh(tokens []string) (ghInvocation, bool) {
	pos := positionals(tokens)
	if len(pos) == 0 {
		return ghInvocation{}, false
	}
	c := ghInvocation{group: pos[0], args: tokens}
	if len(pos) > 1 {
		c.sub = pos[1]
	}
	return c, true
}

// publishes reports whether the invocation is one of the publishing
// subcommands.
func (c ghInvocation) publishes() bool {
	subs, ok := ghPublishing[c.group]
	if !ok {
		return false
	}
	return subs == nil || slices.Contains(subs, c.sub)
}

// label names the invocation in a deny reason.
func (c ghInvocation) label() string {
	if c.group == "api" || c.sub == "" {
		return "gh " + c.group
	}
	return "gh " + c.group + " " + c.sub
}

// ghPublications covers the gh CLI: the invocation's own text (title, body,
// head, tag, heredoc bodies are all in the command) plus the files it
// reads a body from.
func (g *PublishInternalNames) ghPublications(
	command string,
	c ghInvocation,
	cwd string,
) []publication {
	if !c.publishes() {
		return nil
	}
	repo, public := g.ghTarget(c, cwd)
	if !public {
		return nil
	}
	pubs := []publication{{action: c.label(), repo: repo, where: "command text", text: command}}
	for _, file := range c.files() {
		if body, ok := g.readReferenced(cwd, file); ok {
			pubs = append(pubs, publication{
				action: c.label(), repo: repo, where: "file " + file, text: body,
			})
		}
	}
	return pubs
}

// ghTarget resolves which repo a gh call writes to and whether it is public
// and unexempted: the -R/--repo flag first, then the repo the call names
// itself (a `repos/{owner}/{repo}` api endpoint, the OWNER/REPO argument of
// `repo create`), then the cwd's origin. Resolving the named repo matters
// for an org that lives on github.com but is internal: its allow_repos
// wildcard can only exempt a call whose target is known. Gists and raw api
// calls without a repo still reach github.com, so they are public with no
// identity to exempt. An internal --hostname exempts an api call the way an
// internal remote does.
func (g *PublishInternalNames) ghTarget(c ghInvocation, cwd string) (repo string, public bool) {
	if v, ok := flagValue(c.args, "-R", "--repo"); ok {
		repo = ghRepoArg(v)
		return repo, g.guarded(repo)
	}
	if host, ok := flagValue(c.args, "--hostname"); ok && host != "github.com" {
		return "", false
	}
	if repo = c.namedRepo(); repo != "" {
		return repo, g.guarded(repo)
	}
	if repo = canonicalRepo(g.git(cwd, "remote", "get-url", "origin")); repo != "" {
		return repo, g.guarded(repo)
	}
	switch c.group {
	case "gist", "api":
		return "", g.unknownTargetGuarded()
	case "repo":
		return "", c.sub == "create" && g.unknownTargetGuarded()
	}
	return "", false
}

// namedRepo returns the canonical repo a gh call names in its own
// arguments: the `repos/{owner}/{repo}` prefix of an api endpoint, or the
// OWNER/REPO argument of `repo create`. "" when the call names none.
func (c ghInvocation) namedRepo() string {
	switch {
	case c.group == "api":
		endpoint := strings.TrimPrefix(c.sub, "https://api.github.com/")
		parts := strings.Split(strings.TrimPrefix(endpoint, "/"), "/")
		if len(parts) >= 3 && parts[0] == "repos" {
			return ghRepoArg(parts[1] + "/" + parts[2])
		}
	case c.group == "repo" && c.sub == "create":
		if pos := positionals(c.args); len(pos) > 2 && strings.Contains(pos[2], "/") {
			return ghRepoArg(pos[2])
		}
	}
	return ""
}

// ghRepoArg canonicalizes a -R value: OWNER/REPO (github.com implied),
// HOST/OWNER/REPO, or a full URL.
func ghRepoArg(v string) string {
	if strings.Contains(v, "://") {
		return canonicalRepo(v)
	}
	v = strings.TrimSuffix(v, ".git")
	switch strings.Count(v, "/") {
	case 1:
		return "github.com/" + v
	case 2:
		return v
	}
	return ""
}

// files lists the paths a gh call reads text from: body/notes/input file
// flags, `key=@path` field values, and the file arguments of gist create.
func (c ghInvocation) files() []string {
	files := flagValues(c.args, "--body-file", "-F", "--notes-file", "--input")
	for _, tok := range c.args {
		if _, path, ok := strings.Cut(tok, "=@"); ok && path != "" {
			files = append(files, path)
		}
	}
	if c.group == "gist" && c.sub == "create" {
		if pos := positionals(c.args); len(pos) > 2 {
			files = append(files, pos[2:]...)
		}
	}
	return files
}

// --- shared token helpers -------------------------------------------------

// readReferenced reads a file a command names, resolved against dir. A
// missing or unreadable file (or "-" for stdin) contributes nothing: the
// same command may be creating it, and its text is then in the command.
func (g *PublishInternalNames) readReferenced(dir, path string) (string, bool) {
	if path == "" || path == "-" {
		return "", false
	}
	switch {
	case strings.HasPrefix(path, "~"):
		path = config.ExpandHome(path)
	case filepath.IsAbs(path):
	case dir == "":
		return "", false
	default:
		path = filepath.Join(dir, path)
	}
	body, err := g.readFile(path)
	if err != nil {
		return "", false
	}
	return body, true
}

// readFileString is the real readFile backend.
func readFileString(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// hasFlag reports whether any of the flags appears among the args, in bare
// or =value form.
func hasFlag(args []string, flags ...string) bool {
	for _, arg := range args {
		name, _, _ := strings.Cut(arg, "=")
		if slices.Contains(flags, arg) || slices.Contains(flags, name) {
			return true
		}
	}
	return false
}

// flagValue returns the value of the first of the given flags: the next
// token, or the part after = for long flags.
func flagValue(args []string, flags ...string) (string, bool) {
	values := flagValues(args, flags...)
	if len(values) == 0 {
		return "", false
	}
	return values[0], true
}

// flagValues returns every value given to any of the flags, in order.
func flagValues(args []string, flags ...string) []string {
	var values []string
	for i := 0; i < len(args); i++ {
		if slices.Contains(flags, args[i]) {
			if i+1 < len(args) {
				values = append(values, args[i+1])
				i++
			}
			continue
		}
		name, value, ok := strings.Cut(args[i], "=")
		if ok && strings.HasPrefix(name, "--") && slices.Contains(flags, name) {
			values = append(values, value)
		}
	}
	return values
}

// positionals returns the non-flag tokens. Flags with a separate value are
// not modelled here; callers that need a value read it with flagValues.
func positionals(args []string) []string {
	var pos []string
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			pos = append(pos, arg)
		}
	}
	return pos
}
