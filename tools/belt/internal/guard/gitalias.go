package guard

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// aliasMention finds a git alias name a text sets or reads: alias.<name>, in
// any case, since git compares config keys that way.
var aliasMention = regexp.MustCompile(`(?i)\balias\.([a-z0-9][a-z0-9-]*)`)

// noteAliasNames adds every alias name a text mentions (aliasNamesIn) to the
// names a git subcommand may be an alias of.
func (m *walkMemory) noteAliasNames(text string) {
	for _, name := range aliasNamesIn(text) {
		m.aliasNames[name] = true
	}
}

// aliasNamesIn returns the alias.<name> names a text mentions, lowercased,
// with its quotes and backslashes dropped as the shell drops them
// (`al"ias".p`).
func aliasNamesIn(text string) []string {
	text = strings.NewReplacer(`"`, "", `'`, "", `\`, "").Replace(text)
	var names []string
	for _, match := range aliasMention.FindAllStringSubmatch(text, -1) {
		names = append(names, strings.ToLower(match[1]))
	}
	return names
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

// ownAliasBodies returns the bodies the invocation's own -c options give the
// alias name, and whether one of them cannot be read: a --config-env, whose
// value comes from the environment, a -c with an expansion in it, or a key
// with no value.
func (c gitCall) ownAliasBodies(name string) (bodies []string, unread bool) {
	key := "alias." + name
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
		switch {
		case ok && flag == "-c" && strings.EqualFold(k, key) && set:
			bodies = append(bodies, body)
		case !ok || flag != "-c" || strings.EqualFold(k, key):
			// An expansion, an environment value or a bare key: only a key
			// that names this alias leaves its body unread.
			unread = unread || slices.Contains(aliasNamesIn(value.raw+" "+lit), name)
		}
	}
	return bodies, unread
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

// aliasFeeds returns what a git invocation runs when its subcommand may be a
// git alias: one the invocation's own -c sets, one a git config write earlier
// in the command set (walkMemory.aliasBodies), or one the command names as
// alias.<name> anywhere (walkMemory.aliasNames). Each body is text for the
// walk to read. An alias whose body the walk cannot read, or one met again
// while its own body is read, is one feed it cannot read. git ignores an
// alias that hides one of its own commands; belt reads the body anyway, which
// only adds steps. A body that starts with ! runs in gitDir, the directory
// git works in; any other body is a git command read from shellDir, the
// shell's directory, behind the same global options.
func (w *shellWalk) aliasFeeds(call gitCall, gitDir, shellDir string) []feed {
	name := strings.ToLower(call.sub)
	own, unread := call.ownAliasBodies(name)
	set := w.mem.aliasBodies[name]
	if len(own) == 0 && !unread && len(set) == 0 && !w.mem.aliasNames[name] {
		return nil
	}
	what := fmt.Sprintf("what the git alias `%s` runs", call.sub)
	if slices.Contains(w.expanding, name) {
		return []feed{{runner: what + " inside its own body", alias: name}}
	}
	bodies := own
	for _, s := range set {
		unread = unread || !s.ok
		bodies = append(bodies, s.text)
	}
	if unread || len(bodies) == 0 {
		return []feed{{runner: what, alias: name}}
	}
	feeds := make([]feed, 0, len(bodies))
	for _, body := range bodies {
		f := feed{
			runner: what, text: call.aliasScript(body), ok: true, alias: name, dir: shellDir, pre: call.pre,
		}
		if strings.HasPrefix(body, "!") {
			f.dir = gitDir
		}
		feeds = append(feeds, f)
	}
	return feeds
}

// tokenAlias returns what the token walk reports for a git invocation whose
// subcommand may be an alias: the walk reads no words there, so it cannot
// read the body, and the invocation is opaque. "" for any other command.
func (w *shellWalk) tokenAlias(tokens []string) string {
	c, ok := segmentGitCommand(tokens, "")
	name := strings.ToLower(c.sub)
	if !ok || (!w.mem.aliasNames[name] && len(w.mem.aliasBodies[name]) == 0) {
		return ""
	}
	return fmt.Sprintf("what the git alias `%s` runs", c.sub)
}

// rememberAliasWrite records the alias body a git config write sets
// (`git config [--global] alias.p push`, `git config set alias.p push`), or
// that it sets one the walk cannot read: a value with an expansion, or an
// --unset, --add or --replace-all shape.
func (m *walkMemory) rememberAliasWrite(call gitCall) {
	if call.sub != "config" {
		return
	}
	args := make([]string, 0, len(call.args))
	for _, a := range call.args {
		args = append(args, a.token())
	}
	if !gitConfigWrites(args) {
		return
	}
	var positional []placedWord
	plainSet := true
	for i := 0; i < len(args); i++ {
		switch arg := args[i]; {
		case gitConfigValueFlags[arg]:
			i++
		case arg == "--global" || arg == "--local" || arg == "--system" || arg == "--worktree":
		case strings.HasPrefix(arg, "-"):
			plainSet = false
		default:
			positional = append(positional, call.args[i])
		}
	}
	if len(positional) > 0 && positional[0].token() == "set" {
		positional = positional[1:]
	}
	if len(positional) == 0 {
		return
	}
	key, ok := positional[0].literal()
	name, isAlias := strings.CutPrefix(strings.ToLower(key), "alias.")
	if !ok || !isAlias {
		return
	}
	body := script{}
	if len(positional) == 2 && plainSet {
		body.text, body.ok = positional[1].literal()
	}
	m.aliasNames[name] = true
	m.aliasBodies[name] = append(m.aliasBodies[name], body)
}
