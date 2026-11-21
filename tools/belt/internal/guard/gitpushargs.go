package guard

import (
	"slices"
	"strings"
)

// pushOption is one long option of git push as builtin/push.c declares it.
// value marks one that takes a value: the text after =, else the next word,
// whatever that word is. An option whose value is optional
// (--force-with-lease, --signed) takes the = form only, so it is no value
// option here.
type pushOption struct {
	name  string
	value bool
}

// pushOptions are the long options of git push. git reads any unique prefix
// of one as that option (--mir is --mirror), and --branches is an alias of
// --all.
var pushOptions = []pushOption{
	{name: "verbose"}, {name: "quiet"}, {name: "repo", value: true},
	{name: "all"}, {name: "branches"}, {name: "mirror"}, {name: "delete"},
	{name: "tags"}, {name: "dry-run"}, {name: "porcelain"}, {name: "force"},
	{name: "force-with-lease"}, {name: "force-if-includes"},
	{name: "recurse-submodules", value: true}, {name: "thin"},
	{name: "receive-pack", value: true}, {name: "exec", value: true},
	{name: "set-upstream"}, {name: "progress"}, {name: "prune"},
	{name: "no-verify"}, {name: "follow-tags"}, {name: "signed"},
	{name: "atomic"}, {name: "push-option", value: true},
	{name: "ipv4"}, {name: "ipv6"},
}

// pushShortOptions are the short options of git push, by the long option
// each stands for. Only -o takes a value.
var pushShortOptions = map[byte]string{
	'v': "verbose", 'q': "quiet", 'd': "delete", 'n': "dry-run", 'f': "force",
	'u': "set-upstream", 'o': "push-option", '4': "ipv4", '6': "ipv6",
}

// longPushOption reads a long option the way git's parse-options does,
// given the text after -- and before any =. An exact name wins. Otherwise
// every option the text is a prefix of matches, and a --no- form negates
// the options its rest is a prefix of. names are the options the word sets
// and value whether it takes a value. A negation sets nothing. A prefix of
// several options is an error to git, so nothing is pushed; it sets every
// option it may mean and takes no value, which can only add a denial.
func longPushOption(text string) (names []string, value bool) {
	for _, o := range pushOptions {
		switch text {
		case o.name:
			return []string{o.name}, o.value
		// An option named no-X (no-verify) is negated as X, so --verify
		// reads as the negation of --no-verify.
		case "no-" + o.name, strings.TrimPrefix(o.name, "no-"):
			return nil, false
		}
	}
	var matched []pushOption
	for _, o := range pushOptions {
		if strings.HasPrefix(o.name, text) {
			matched = append(matched, o)
		}
	}
	if len(matched) == 1 && !strings.HasPrefix("no-", text) {
		return []string{matched[0].name}, matched[0].value
	}
	for _, o := range matched {
		names = append(names, o.name)
	}
	return names, false
}

// shortPushOptions reads a bundle of short options, the word without its -:
// each letter is an option until one that takes a value, which takes the
// rest of the word or, when nothing is left, the next word (next). A letter
// git push does not know ends the bundle, as git stops there with an error.
func shortPushOptions(letters string) (names []string, next bool) {
	for i := 0; i < len(letters); i++ {
		name, ok := pushShortOptions[letters[i]]
		if !ok {
			return names, false
		}
		names = append(names, name)
		if name == "push-option" {
			return names, i == len(letters)-1
		}
	}
	return names, false
}

// parsePushArgs reads the options, the remote and the refspecs out of a
// parsed push, the way git's parse-options reads them: options anywhere
// before a -- or --end-of-options, a long option by any unique prefix, a
// bundle of short options letter by letter, and a value option taking the
// next word even when that word is --. The first other word is the remote
// and the rest are refspecs. A word outside the refspecs that only a shell
// can resolve is kept as shellWord: it may turn into any option, refspec or
// number of words. sweep says what makes the push update branches it does
// not name (pushSweep).
func parsePushArgs(c gitInvocation) gitPush {
	push := gitPush{
		dir: c.dir, configured: c.configured, unread: c.unread, bash32: c.bash32, moved: c.moved,
		heads: c.heads, opaque: c.opaque,
	}
	var positional []string
	ended := false
	for i := 0; i < len(c.args); i++ {
		tok := c.args[i]
		if ended || tok == "-" || !strings.HasPrefix(tok, "-") {
			positional = append(positional, tok)
			continue
		}
		if tok == "--" || tok == "--end-of-options" {
			ended = true
			continue
		}
		push.noteShellWord(tok)
		names, next := push.readOption(tok)
		push.setOptions(names)
		if next && i+1 < len(c.args) {
			i++
			push.noteShellWord(c.args[i])
		}
	}
	if len(positional) > 0 {
		push.remote = positional[0]
		push.noteShellWord(push.remote)
	}
	if len(positional) > 1 {
		push.refspecs = positional[1:]
		push.explicit = true
	}
	push.sweep = pushSweep(push, c)
	return push
}

// readOption reads one option word: the options it sets, and whether it
// takes the next word as its value.
func (p *gitPush) readOption(tok string) (names []string, next bool) {
	if !strings.HasPrefix(tok, "--") {
		return shortPushOptions(tok[1:])
	}
	text, _, inline := strings.Cut(tok[2:], "=")
	names, value := longPushOption(text)
	return names, value && !inline
}

// setOptions records the options of one word that decide what a push sends.
func (p *gitPush) setOptions(names []string) {
	for _, name := range names {
		switch name {
		case "all", "branches":
			p.all = true
		case "mirror":
			p.mirror = true
		case "tags":
			p.tags = true
		case "delete":
			p.del = true
		case "repo":
			p.repoFlag = true
		}
	}
}

// noteShellWord keeps the first word outside the refspecs that is not plain
// text (plainRefspec): a variable, a substitution, a quote, a brace or a
// glob there may expand into --mirror, a remote and a refspec, or no word.
func (p *gitPush) noteShellWord(word string) {
	if p.shellWord == "" && !plainRefspec(word) {
		p.shellWord = word
	}
}

// pushSweep names what makes a push update branches it does not name, a
// default branch among them, or returns "" when it pushes only what it
// names. That is --mirror or --all (or a prefix git reads as one), the
// matching refspec : (every branch both sides have), or config that may
// choose or map its refspecs: a -c or --config-env key that does
// (sweepConfigKey for a push that names no refspec, remapConfigKey for one
// with a refspec git maps), or config the command sets or changes some other
// way (gitInvocation.setsConfig). A push that deletes, or whose refspecs all
// name a destination, pushes only what it names.
func pushSweep(push gitPush, c gitInvocation) string {
	sweeps := sweepConfigKey
	switch {
	case push.mirror:
		return "--mirror"
	case push.all:
		return "--all"
	case slices.ContainsFunc(push.refspecs, matchingRefspec):
		return `the matching refspec ":"`
	case push.explicit && (push.del || !slices.ContainsFunc(push.refspecs, mappedRefspec)):
		return ""
	case push.explicit:
		sweeps = remapConfigKey
	}
	if i := slices.IndexFunc(c.config, sweeps); i >= 0 {
		return "the config override " + c.config[i]
	}
	if c.setsConfig {
		return "config the command sets or changes"
	}
	return ""
}

// matchingRefspec reports whether a refspec is :, git's matching refspec,
// with or without the + that forces it.
func matchingRefspec(spec string) bool {
	return strings.TrimPrefix(spec, "+") == ":"
}

// mappedRefspec reports whether git may map a refspec onto another
// destination: one with no :dst, which set_refspecs in builtin/push.c sends
// where remote.<name>.push or an upstream push would.
func mappedRefspec(spec string) bool {
	return !strings.Contains(spec, ":")
}

// configWord is what gitInvocation.config keeps of a -c or --config-env
// value: the key=value word of a -c, and only the key of a --config-env,
// whose value comes from the environment.
func configWord(flag, word string) string {
	if flag == "--config-env" {
		key, _, _ := strings.Cut(word, "=")
		return key
	}
	return word
}

// safePushDefaults are the push.default values that push the current branch
// alone, to a branch of the same name; the guard judges that branch.
var safePushDefaults = wordSet("simple current nothing")

// namedPushDefaults are the push.default values that leave a refspec with no
// :dst on the branch of the same name. Only upstream, and tracking, its old
// name, map one to the branch's upstream.
var namedPushDefaults = wordSet("simple current nothing matching")

// configSetting reads a gitInvocation.config word as git reads
// section.key=value: the key in lower case, since git reads section and key
// names in any case, and the value. valued is false for a word with no =,
// such as a --config-env key, whose value only the environment holds.
func configSetting(word string) (key, value string, valued bool) {
	key, value, valued = strings.Cut(unquoteWord(word), "=")
	return strings.ToLower(key), value, valued
}

// includeKey reports whether a config key reads another config file,
// include.path or includeIf.<condition>.path, whose keys may set any of the
// ones that pick or map a push's refspecs.
func includeKey(key string) bool {
	return key == "include.path" || (strings.HasPrefix(key, "includeif.") && strings.HasSuffix(key, ".path"))
}

// sweepConfigKey reports whether a gitInvocation.config word sets a key that
// may make a push without a refspec update branches it does not name:
// push.default set to matching, upstream, tracking or any value belt cannot
// read, remote.<name>.push, remote.<name>.mirror, branch.<name>.merge, the
// upstream an upstream push goes to, or an include (includeKey). A key only
// a shell can expand may be any of them.
func sweepConfigKey(word string) bool {
	key, value, _ := configSetting(word)
	switch {
	case !plainRefspec(key) || includeKey(key):
		return true
	case key == "push.default":
		return !safePushDefaults[value]
	case strings.HasPrefix(key, "remote."):
		return strings.HasSuffix(key, ".push") || strings.HasSuffix(key, ".mirror")
	case strings.HasPrefix(key, "branch."):
		return strings.HasSuffix(key, ".merge")
	}
	return false
}

// remapConfigKey reports whether a gitInvocation.config word sets a key that
// may send a refspec with no :dst (mappedRefspec) to a default branch:
// remote.<name>.push with a destination that may be one (pushDestMayBeDefault)
// or a value belt cannot see, push.default set to upstream, tracking or a
// value belt cannot read, branch.<name>.merge, the upstream such a push goes
// to, or an include (includeKey). A key only a shell can expand may be any of
// them.
func remapConfigKey(word string) bool {
	key, value, valued := configSetting(word)
	switch {
	case !plainRefspec(key) || includeKey(key):
		return true
	case key == "push.default":
		return !namedPushDefaults[value]
	case strings.HasPrefix(key, "remote.") && strings.HasSuffix(key, ".push"):
		return !valued || pushDestMayBeDefault(value)
	case strings.HasPrefix(key, "branch."):
		return strings.HasSuffix(key, ".merge")
	}
	return false
}

// pushDestMayBeDefault reports whether a remote.<name>.push value may map a
// branch onto a default branch: its destination, after the last colon
// (destBranch), is one or HEAD, or the value is a pattern or text belt cannot
// read. A value with no colon names no destination, and git maps nothing
// through it.
func pushDestMayBeDefault(value string) bool {
	switch {
	case !plainRefspec(value):
		return true
	case !strings.Contains(value, ":"):
		return false
	}
	dest := destBranch(value)
	return defaultBranches[dest] || dest == "HEAD"
}

// setsConfigEnv reports whether a step puts a variable git reads config
// from in the environment (configEnvVar), in front of its command word or
// through export and its kin, or runs git as another user (sudo, doas, su),
// with config belt cannot read. A comment or a text body sets nothing.
func setsConfigEnv(s walkStep) bool {
	if s.prose {
		return false
	}
	words := make([]string, 0, len(s.tokens))
	for _, t := range s.tokens {
		words = append(words, unquoteWord(t))
	}
	for _, a := range envAssignments(words) {
		name, _, _ := strings.Cut(a, "=")
		if configEnvVar(strings.TrimSuffix(name, "+")) {
			return true
		}
	}
	pre, hasGit := beforeGit(words)
	return hasGit && slices.ContainsFunc(pre, func(w string) bool {
		return w == "sudo" || w == "doas" || w == "su"
	})
}

// configEnvVar reports whether git reads config from a variable: a
// GIT_CONFIG* one sets keys or names a config file, and HOME and
// XDG_CONFIG_HOME place the global one.
func configEnvVar(name string) bool {
	return strings.HasPrefix(name, "GIT_CONFIG") || name == "HOME" || name == "XDG_CONFIG_HOME"
}
