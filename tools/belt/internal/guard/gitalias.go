package guard

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// aliasMention finds a git alias name in config text: alias.<name>, in any
// case, since git compares config keys that way.
var aliasMention = regexp.MustCompile(`(?i)\balias\.([a-z0-9][a-z0-9-]*)`)

// aliasNamesIn returns the alias.<name> names a config text mentions,
// lowercased, with its quotes and backslashes dropped as the shell drops
// them (`al"ias".p`).
func aliasNamesIn(text string) []string {
	var names []string
	for _, match := range aliasMention.FindAllStringSubmatch(unquoteWord(text), -1) {
		names = append(names, strings.ToLower(match[1]))
	}
	return names
}

// unquoteWord drops the quotes and backslashes of a word as written, as the
// shell drops them.
func unquoteWord(word string) string {
	return strings.NewReplacer(`"`, "", `'`, "", `\`, "").Replace(word)
}

// gitBuiltins are git's builtin commands (`git --list-cmds=builtins`). git
// runs a builtin before it looks for an alias, so an alias with one of these
// names never runs. The newest builtins and the deprecated ones are left
// out: with a git that lacks one, an alias of that name would run.
var gitBuiltins = wordSet(`
	add am annotate apply archive bisect blame branch bundle cat-file
	check-attr check-ignore check-mailmap check-ref-format checkout
	checkout--worker checkout-index cherry cherry-pick clean clone column
	commit commit-graph commit-tree config count-objects credential
	credential-cache credential-cache--daemon credential-store describe diff
	diff-files diff-index diff-tree difftool fast-export fast-import fetch
	fetch-pack fmt-merge-msg for-each-ref for-each-repo format-patch fsck
	fsck-objects fsmonitor--daemon gc get-tar-commit-id grep hash-object help
	hook index-pack init init-db interpret-trailers log ls-files ls-remote
	ls-tree mailinfo mailsplit maintenance merge merge-base merge-file
	merge-index merge-ours merge-recursive merge-recursive-ours
	merge-recursive-theirs merge-subtree merge-tree mktag mktree
	multi-pack-index mv name-rev notes pack-objects pack-refs patch-id prune
	prune-packed pull push range-diff read-tree rebase receive-pack reflog
	remote remote-ext remote-fd repack replace rerere reset restore rev-list
	rev-parse revert rm send-pack shortlog show show-branch show-index
	show-ref sparse-checkout stage stash status stripspace submodule--helper
	switch symbolic-ref tag unpack-file unpack-objects update-index
	update-ref update-server-info upload-archive upload-archive--writer
	upload-pack var verify-commit verify-pack verify-tag version worktree
	write-tree
`)

// gitConfigKeyVar is a GIT_CONFIG_KEY_<n> variable, whose value is a config
// key git sets for every git that sees it.
var gitConfigKeyVar = regexp.MustCompile(`^GIT_CONFIG_KEY_[0-9]+$`)

// noteAliasConfig records the alias names a command sets config for, before
// its own git word is judged: the key of a git -c or --config-env value, the
// key a git config write sets (readConfigSet), and a GIT_CONFIG_KEY_<n> or
// GIT_CONFIG_PARAMETERS assignment in front of the command or after export.
// A key belt cannot read may name any alias (walkMemory.anyAlias). Free text
// never counts: alias.<name> in a commit message, a grep pattern or a config
// read names no alias the command sets.
func (m *walkMemory) noteAliasConfig(tokens []string) {
	words := make([]string, 0, len(tokens))
	for _, t := range tokens {
		words = append(words, unquoteWord(t))
	}
	for _, a := range envAssignments(words) {
		name, value, _ := strings.Cut(a, "=")
		switch {
		case gitConfigKeyVar.MatchString(name):
			m.noteAliasKey(value)
		case name == "GIT_CONFIG_PARAMETERS" && strings.ContainsAny(value, "$`"):
			m.anyAlias = true
		case name == "GIT_CONFIG_PARAMETERS":
			for _, n := range aliasNamesIn(value) {
				m.aliasNames[n] = true
			}
		}
	}
	at := slices.IndexFunc(words, isGitWord)
	if at < 0 {
		return
	}
	for i := at + 1; i < len(words); i++ {
		switch tok := words[i]; {
		case (tok == "-c" || tok == "--config-env") && i+1 < len(words):
			i++
			key, _, _ := strings.Cut(words[i], "=")
			m.noteAliasKey(key)
		case strings.HasPrefix(tok, "--config-env="):
			key, _, _ := strings.Cut(strings.TrimPrefix(tok, "--config-env="), "=")
			m.noteAliasKey(key)
		case tok == "-C" || tok == "--git-dir" || tok == "--work-tree" || gitValueFlags[tok]:
			i++
		case strings.HasPrefix(tok, "-"):
		default:
			if s, ok := readConfigSet(words[i+1:]); ok && tok == "config" {
				m.noteAliasKey(words[i+1+s.key])
			}
			return
		}
	}
}

// noteAliasKey records the alias a config key names, or that a key with an
// expansion in it may name any alias.
func (m *walkMemory) noteAliasKey(key string) {
	if strings.ContainsAny(key, "$`") {
		m.anyAlias = true
		return
	}
	if name, ok := aliasKeyName(key); ok {
		m.aliasNames[name] = true
	}
}

// aliasKeyName returns the alias name a config key sets, lowercased:
// alias.<name> in any case.
func aliasKeyName(key string) (string, bool) {
	const section = "alias."
	if len(key) <= len(section) || !strings.EqualFold(key[:len(section)], section) {
		return "", false
	}
	return strings.ToLower(key[len(section):]), true
}

// envAssignments returns the NAME=value words a command puts in the
// environment: the ones in front of its command word, env's and the other
// wrappers' included, and the operands of export and its kin (varBuiltins).
func envAssignments(words []string) []string {
	var out []string
	head := len(words)
	for i, w := range words {
		switch {
		case isAssignment(w):
			out = append(out, w)
		case wrapperCommands[filepath.Base(w)], i > 0 && strings.HasPrefix(w, "-"):
		default:
			head = i
		}
		if head < len(words) {
			break
		}
	}
	if head < len(words) && varBuiltins[words[head]] {
		for _, w := range words[head+1:] {
			if isAssignment(w) {
				out = append(out, w)
			}
		}
	}
	return out
}

// gitCall is one git invocation as placed words: the global options before
// the subcommand, the subcommand as the walk compares it, and the arguments.
type gitCall struct {
	pre     []string // the tokens in front of the git word
	globals []placedWord
	sub     string
	args    []placedWord
}

// readGitCall reads the first git word in a command's words the way
// parseGitCmd reads its tokens, keeping the words so an alias body can be
// put back in their place. ok is false when no git word or no subcommand is
// there.
func readGitCall(tokens []string, words []placedWord) (gitCall, bool) {
	at := slices.IndexFunc(tokens, isGitWord)
	if at < 0 || len(words) != len(tokens) {
		return gitCall{}, false
	}
	for i := at + 1; i < len(tokens); i++ {
		switch tok := tokens[i]; {
		case tok == "-C" || tok == "--git-dir" || tok == "--work-tree" || gitValueFlags[tok]:
			i++
		case strings.HasPrefix(tok, "-"):
		default:
			return gitCall{
				pre: tokens[:at], globals: words[at+1 : i], sub: tok, args: words[i+1:],
			}, true
		}
	}
	return gitCall{}, false
}

// ownAlias is an alias an invocation's own -c or --config-env sets, with
// its body unread when it comes from an expansion, the environment, or a
// key with no value.
type ownAlias struct {
	name string
	body script
}

// ownAliases returns the aliases the invocation's own -c and --config-env
// options set.
func (c gitCall) ownAliases() []ownAlias {
	var out []ownAlias
	for i := 0; i < len(c.globals); i++ {
		flag := c.globals[i].token()
		var value placedWord
		switch {
		case (flag == "-c" || flag == "--config-env") && i+1 < len(c.globals):
			i++
			value = c.globals[i]
		case strings.HasPrefix(flag, "--config-env="):
			value = placedWord{
				shWord: shWord{raw: strings.TrimPrefix(c.globals[i].raw, "--config-env=")},
			}
		default:
			continue
		}
		lit, ok := value.literal()
		k, body, set := strings.Cut(lit, "=")
		name, isAlias := aliasKeyName(k)
		switch {
		case ok && flag == "-c" && isAlias && set:
			out = append(out, ownAlias{name: name, body: script{text: body, ok: true}})
		case ok && flag == "-c" && !isAlias:
		default:
			for _, n := range aliasNamesIn(value.raw + " " + lit) {
				out = append(out, ownAlias{name: n})
			}
		}
	}
	return out
}

// rememberOwnAliases records the aliases an invocation's own options set,
// since git hands its -c options on to every git an alias body runs.
func (m *walkMemory) rememberOwnAliases(call gitCall) {
	for _, a := range call.ownAliases() {
		m.aliasNames[a.name] = true
		m.aliasBodies[a.name] = append(m.aliasBodies[a.name], a.body)
	}
}

// rawWords joins words as written, so text built from them reads again the
// way the shell read it.
func rawWords(words []placedWord) string {
	out := make([]string, 0, len(words))
	for _, w := range words {
		out = append(out, w.raw)
	}
	return strings.Join(out, " ")
}

// aliasScript is the text git runs for an alias body: a body that starts
// with ! runs in a shell with the arguments after it, any other body takes
// the subcommand's place behind the same global options.
func (c gitCall) aliasScript(body string) string {
	if shellBody, ok := strings.CutPrefix(body, "!"); ok {
		return shellBody + " " + rawWords(c.args)
	}
	return "git " + rawWords(c.globals) + " " + body + " " + rawWords(c.args)
}

// mayBeAlias reports whether a git subcommand may run an alias the command
// set config for (noteAliasConfig): never a builtin, which git runs before it
// looks for an alias.
func (m *walkMemory) mayBeAlias(name string) bool {
	if gitBuiltins[name] {
		return false
	}
	return m.anyAlias || m.aliasNames[name] || len(m.aliasBodies[name]) > 0
}

// aliasFeeds returns what a git invocation runs when its subcommand may be a
// git alias the command sets: with the invocation's own -c, with a git
// config write earlier in the command (walkMemory.aliasBodies), or with a key
// belt cannot read (walkMemory.anyAlias). Each body is text for the walk to
// read. An alias whose body the walk cannot read, or one met again while its
// own body is read, is one feed it cannot read. A body that starts with !
// runs in gitDir, the directory git works in; any other body is a git
// command read from shellDir, the shell's directory, behind the same global
// options. git runs a builtin before any alias (gitBuiltins), so a builtin
// subcommand has none.
func (w *shellWalk) aliasFeeds(call gitCall, gitDir, shellDir string) []feed {
	name := strings.ToLower(call.sub)
	if gitBuiltins[name] {
		return nil
	}
	var bodies []script
	for _, a := range call.ownAliases() {
		if a.name == name {
			bodies = append(bodies, a.body)
		}
	}
	if len(bodies) == 0 && !w.mem.mayBeAlias(name) {
		return nil
	}
	what := fmt.Sprintf("what the git alias `%s` runs", call.sub)
	if slices.Contains(w.expanding, name) {
		return []feed{{runner: what + " inside its own body", alias: name}}
	}
	bodies = append(bodies, w.mem.aliasBodies[name]...)
	unread := slices.ContainsFunc(bodies, func(s script) bool { return !s.ok })
	if w.mem.anyAlias || unread || len(bodies) == 0 {
		return []feed{{runner: what, alias: name}}
	}
	feeds := make([]feed, 0, len(bodies))
	for _, body := range bodies {
		f := feed{
			runner: what, text: call.aliasScript(body.text), ok: true, alias: name,
			dir: shellDir, pre: call.pre,
		}
		if strings.HasPrefix(body.text, "!") {
			f.dir = gitDir
			f.positional = len(call.args) > 0
		}
		feeds = append(feeds, f)
	}
	return feeds
}

// tokenAlias returns what the token walk reports for a git invocation whose
// subcommand may be an alias the command set (mayBeAlias): the walk reads no
// words there, so it cannot read the body, and the invocation is opaque. ""
// for any other command.
func (w *shellWalk) tokenAlias(tokens []string) string {
	c, ok := segmentGitCommand(tokens, "")
	if !ok || !w.mem.mayBeAlias(strings.ToLower(c.sub)) {
		return ""
	}
	return fmt.Sprintf("what the git alias `%s` runs", c.sub)
}

// configSet is what a git config call sets: the indexes of its key and its
// value (-1 when none) in the arguments, and plain when nothing but a scope
// option comes with them, so the value is the whole new value.
type configSet struct {
	key, value int
	plain      bool
}

// configScopeFlags pick which config file a git config call writes.
var configScopeFlags = wordSet("--global --local --system --worktree")

// configClearFlags and configClearSubs make a git config call remove or
// rename what it names, or open an editor, rather than set a key.
var (
	configClearFlags = wordSet("--unset --unset-all --rename-section --remove-section -e --edit")
	configClearSubs  = wordSet("unset rename-section remove-section edit")
)

// readConfigSet reads the key a git config call sets a value for. ok is
// false for a read (gitConfigWrites), an unset, a section rename or removal,
// and an edit.
func readConfigSet(args []string) (configSet, bool) {
	if !gitConfigWrites(args) {
		return configSet{}, false
	}
	var positional []int
	plain := true
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case configClearFlags[arg]:
			return configSet{}, false
		case gitConfigValueFlags[arg]:
			i++
		case configScopeFlags[arg]:
		case strings.HasPrefix(arg, "-"):
			plain = false
		default:
			positional = append(positional, i)
		}
	}
	if len(positional) > 0 && configClearSubs[args[positional[0]]] {
		return configSet{}, false
	}
	if len(positional) > 0 && args[positional[0]] == "set" {
		positional = positional[1:]
	}
	if len(positional) == 0 {
		return configSet{}, false
	}
	s := configSet{key: positional[0], value: -1, plain: plain && len(positional) == 2}
	if len(positional) > 1 {
		s.value = positional[1]
	}
	return s, true
}

// rememberAliasWrite records the alias body a git config write sets
// (`git config [--global] alias.p push`, `git config set alias.p push`), or
// that it sets one the walk cannot read: a value with an expansion, or an
// --add or --replace-all shape.
func (m *walkMemory) rememberAliasWrite(call gitCall) {
	if call.sub != "config" {
		return
	}
	args := make([]string, 0, len(call.args))
	for _, a := range call.args {
		args = append(args, a.token())
	}
	s, ok := readConfigSet(args)
	if !ok {
		return
	}
	key, ok := call.args[s.key].literal()
	name, isAlias := aliasKeyName(key)
	if !ok || !isAlias {
		return
	}
	body := script{}
	if s.plain {
		body.text, body.ok = call.args[s.value].literal()
	}
	m.aliasNames[name] = true
	m.aliasBodies[name] = append(m.aliasBodies[name], body)
}
