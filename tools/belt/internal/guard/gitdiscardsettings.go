package guard

import (
	"maps"
	"slices"
	"strings"
)

// callSetting is one config setting a git call runs with beyond the config
// files belt reads before the command runs: a -c or --config-env, a
// GIT_CONFIG_KEY_<n> and GIT_CONFIG_VALUE_<n> pair or a
// GIT_CONFIG_PARAMETERS entry in its environment, or a write an earlier step
// makes. key is in lower case, as git reads section and key names. anyKey
// and anyValue mark a part the shell may still rewrite, or one only the
// environment holds, which may then be any.
type callSetting struct {
	key, value       string
	anyKey, anyValue bool
}

// callSettings returns the settings the git call c at steps[i] runs with:
// its own -c and --config-env words (wordSetting), what its environment
// sets (envSettings) and what the steps before it write (writtenSettings).
func callSettings(c gitInvocation, steps []walkStep, i int) []callSetting {
	out := make([]callSetting, 0, len(c.config))
	for _, w := range c.config {
		out = append(out, wordSetting(w))
	}
	return slices.Concat(out, envSettings(steps, i), writtenSettings(steps, i))
}

// mayExpandIgnoringStar reports whether the shell may hand git another word
// than the one written (mayExpand), a * aside: in a refspec, or a setting
// that may hold one, a * is a pattern git reads itself.
func mayExpandIgnoringStar(word string) bool {
	return mayExpand(strings.ReplaceAll(word, "*", ""))
}

// wordSetting reads a gitInvocation.config word: key=value as written, or a
// --config-env key alone, whose value only the environment holds.
func wordSetting(word string) callSetting {
	key, value, valued := strings.Cut(word, "=")
	return callSetting{
		key:      strings.ToLower(unquoteWord(key)),
		value:    unquoteWord(value),
		anyKey:   mayExpand(key),
		anyValue: !valued || mayExpandIgnoringStar(value),
	}
}

// settingEnvName reports whether git reads settings from a variable
// directly: GIT_CONFIG_PARAMETERS, where git passes -c on to the git
// commands it runs, and the GIT_CONFIG_KEY_<n> and GIT_CONFIG_VALUE_<n>
// pairs.
func settingEnvName(name string) bool {
	return name == "GIT_CONFIG_PARAMETERS" || strings.HasPrefix(name, "GIT_CONFIG_KEY_") ||
		strings.HasPrefix(name, "GIT_CONFIG_VALUE_")
}

// envSettings returns the settings the command puts in the environment of
// the git call at steps[i] (settingEnvName): an assignment in front of it,
// and one in any step before it, exported or not, since the variable may
// already be exported. The last assignment of a name counts, and an
// appending one (+=) may hold anything. A GIT_CONFIG_KEY_<n> pairs with
// the GIT_CONFIG_VALUE_<n> of the same n; one with no value assigned may
// have any value, and a value with no key may set any key. GIT_CONFIG_COUNT
// is not read: it may already be exported. A comment or a text body sets
// nothing.
func envSettings(steps []walkStep, i int) []callSetting {
	vars := map[string]string{}
	for j, s := range steps[:i+1] {
		if s.prose {
			continue
		}
		tokens := s.tokens
		if j == i {
			tokens, _ = beforeGit(tokens)
		}
		for _, a := range envAssignments(tokens) {
			name, value, _ := strings.Cut(a, "=")
			if trimmed, appends := strings.CutSuffix(name, "+"); appends {
				name, value = trimmed, "$"+trimmed
			}
			if settingEnvName(name) {
				vars[name] = value
			}
		}
	}
	var out []callSetting
	for _, name := range slices.Sorted(maps.Keys(vars)) {
		raw := vars[name]
		switch {
		case name == "GIT_CONFIG_PARAMETERS":
			out = append(out, parameterSettings(raw)...)
		case strings.HasPrefix(name, "GIT_CONFIG_KEY_"):
			value, valued := vars["GIT_CONFIG_VALUE_"+strings.TrimPrefix(name, "GIT_CONFIG_KEY_")]
			s := wordSetting(raw + "=" + value)
			s.anyValue = s.anyValue || !valued
			out = append(out, s)
		case !hasKey(vars, strings.TrimPrefix(name, "GIT_CONFIG_VALUE_")):
			s := wordSetting("=" + raw)
			s.anyKey = true
			out = append(out, s)
		}
	}
	return out
}

// hasKey reports whether vars assigns the GIT_CONFIG_KEY_<n> for n.
func hasKey(vars map[string]string, n string) bool {
	_, ok := vars["GIT_CONFIG_KEY_"+n]
	return ok
}

// parameterSettings reads a GIT_CONFIG_PARAMETERS value as written. git
// writes each setting single-quoted, as 'key'='value' or the older
// 'key=value', blank-separated; with its quotes gone, as unquoteWord reads
// every word, each field is key=value. A value with a blank in it splits
// into fields, each read the same way. A value the shell may still rewrite
// may set any key to any value.
func parameterSettings(raw string) []callSetting {
	if mayExpandIgnoringStar(raw) {
		return []callSetting{{anyKey: true, anyValue: true}}
	}
	var out []callSetting
	for _, field := range strings.Fields(unquoteWord(raw)) {
		out = append(out, wordSetting(field))
	}
	return out
}

// writtenSettings returns the settings the steps before steps[i] write to
// a config file, which the config read belt runs before the command cannot
// see: each git config write (configWriteSettings), in any scope or file,
// and the refspecs a `git remote add` writes for a fetch mirror
// (mirrorSettings). A subcommand word of git remote the shell may still
// rewrite, or one a command in front of git may hand over, may be add. A
// comment or a text body writes nothing.
func writtenSettings(steps []walkStep, i int) []callSetting {
	var out []callSetting
	for _, s := range steps[:i] {
		c, ok := segmentGitCommand(s.tokens, "")
		if s.prose || !ok {
			continue
		}
		fed := fedOperands(s.tokens)
		switch c.sub {
		case "config":
			out = append(out, configWriteSettings(c.args, fed)...)
		case "remote":
			sub, rest, unsure := remoteSubcommand(c.args)
			if sub == "add" || unsure || (fed && sub == "") {
				out = append(out, mirrorSettings(rest, fed || sub != "add")...)
			}
		}
	}
	return out
}

// configWriteSettings returns the setting a git config call writes
// (readConfigSet), its key and value as written. Words a command in front
// of git may hand over (fed) may make any call a write of any key, unless
// its own words make it a read.
func configWriteSettings(args []string, fed bool) []callSetting {
	if fed {
		if gitConfigWrites(slices.Concat(args, []string{"key", "value"})) {
			return []callSetting{{anyKey: true, anyValue: true}}
		}
		return nil
	}
	s, ok := readConfigSet(args)
	if !ok || s.value < 0 {
		return nil
	}
	return []callSetting{wordSetting(args[s.key] + "=" + args[s.value])}
}

// allRefsMirror is the refspec `git remote add --mirror=fetch` writes
// without -t: every remote ref onto the local ref of the same name, forced.
const allRefsMirror = "+refs/*:refs/*"

// mirrorSettings returns the refspecs the words after `git remote add`
// write when they set the remote up to fetch as a mirror (fetchMirror), as
// remote.<name>.fetch settings: +refs/<b>:refs/<b> for each -t <b>, the
// last --no-track clearing them, and allRefsMirror without one. Any other
// add writes refspecs into refs/remotes/<name>/. Words belt cannot read,
// words a command in front of git may hand over (unread) and a -t value the
// shell may still rewrite may write allRefsMirror.
func mirrorSettings(rest []string, unread bool) []callSetting {
	w, err := readGitWords(rest, remoteAddOptions)
	all := []callSetting{{key: "remote.*.fetch", value: allRefsMirror}}
	switch {
	case err != nil || unread:
		return all
	case !fetchMirror(w):
		return nil
	}
	var tracks []string
	for _, o := range w.opts {
		switch {
		case o.name != "track":
		case o.unset:
			tracks = nil
		case mayExpand(o.value):
			return all
		default:
			tracks = append(tracks, unquoteWord(o.value))
		}
	}
	if len(tracks) == 0 {
		return all
	}
	out := make([]callSetting, 0, len(tracks))
	for _, b := range tracks {
		out = append(out, callSetting{key: "remote.*.fetch", value: "+refs/" + b + ":refs/" + b})
	}
	return out
}

// fetchMirror reports whether a remote add's words set the remote up to
// fetch as a mirror: --mirror alone or --mirror=fetch, the last --mirror or
// --no-mirror deciding. A value the shell may still rewrite may be fetch;
// git refuses any value but fetch and push.
func fetchMirror(w gitWords) bool {
	o, ok := w.last("mirror")
	if !ok || o.unset {
		return false
	}
	return o.value == "" || mayExpand(o.value) || unquoteWord(o.value) == "fetch"
}

// remoteSubcommand returns the subcommand of a git remote call and the
// words after it: the first word that is no option, since only -v may come
// before it. sub is "" for none, which lists the remotes. unsure says the
// shell may still rewrite a word up to it, which may then be any
// subcommand.
func remoteSubcommand(args []string) (sub string, rest []string, unsure bool) {
	for i, a := range args {
		switch {
		case mayExpand(a):
			return "", nil, true
		case !strings.HasPrefix(unquoteWord(a), "-"):
			return unquoteWord(a), args[i+1:], false
		}
	}
	return "", nil, false
}
