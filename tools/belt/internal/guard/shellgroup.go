package guard

import "slices"

// shellGroup is the body of a ( ) subshell, a { } group or a compound
// command (if, while, until, for, select, case) whose output a step's
// redirections take (walkStep.group): its commands, read from text, from
// the one that opens it up to the one before the command that closes it.
// Each keeps the reserved words it starts with (`{ :`, `if false`). A ( )
// body is one command of the subshells alone. function names the function
// a { } body defines, "" for none: its redirections open at each call, not
// where it is defined.
type shellGroup struct {
	text     string
	cmds     []shellCmd
	function string
}

// subshellGroup is the group of a command of ( ) bodies and redirections
// alone, read from text.
func subshellGroup(text string, c shellCmd) *shellGroup {
	return &shellGroup{text: text, cmds: []shellCmd{{subshells: c.subshells}}}
}

// groupPairs maps each reserved word that opens a compound command to the
// one that closes it (groupClosers).
var groupPairs = map[string]string{
	"{": "}", "if": "fi", "while": "done", "until": "done", "for": "done", "select": "done",
	"case": "esac",
}

// groupHeads are the openers a head follows, not a command (`for x in a`),
// and groupMiddles the reserved words that may stand in front of an opener
// or of the command a group runs (`then {`, `! {`, `do :`).
var (
	groupHeads   = wordSet("for select case")
	groupMiddles = wordSet("! then elif else do")
)

// openGroup is a compound command groupsOf has seen open: the command that
// opens it, the word that closes it, and for a function's body its name and
// where the { stands in that command's words.
type openGroup struct {
	at, brace        int
	closer, function string
}

// groupsOf finds the commands of cmds, read from text, that close a { }
// group or a compound command, and returns the group each closes, by the
// closer's index. A closer is a command of one word, the closing reserved
// word. A function head with no { (`f()` on a line of its own) makes a { at
// the start of the next command its body. A closer out of place drops every
// group open at that point, so none ends at the wrong command.
func groupsOf(text string, cmds []shellCmd) map[int]*shellGroup {
	var open []openGroup
	groups := map[int]*shellGroup{}
	head := ""
	for i, c := range cmds {
		pending := head
		head = ""
		if len(c.words) > 0 && groupClosers[c.words[0].raw] {
			n := len(open)
			if len(c.words) > 1 || n == 0 || open[n-1].closer != c.words[0].raw {
				open = nil
				continue
			}
			groups[i] = open[n-1].group(text, cmds[:i])
			open = open[:n-1]
			continue
		}
		open, head = opensIn(text, c, i, pending, open)
	}
	return groups
}

// opensIn pushes the groups command c, at index i, opens onto open: each
// opener among its reserved words, up to the command it runs or the head of
// a for, select or case. pending names a function whose { may start c. A
// function head with no { comes back as head.
func opensIn(
	text string, c shellCmd, i int, pending string, open []openGroup,
) ([]openGroup, string) {
	defines := definesFunction(text, c, c.tokens())
	for k := 0; k < len(c.words); k++ {
		v := c.words[k].raw
		switch closer, opens := groupPairs[v]; {
		case k == 0 && v == "{" && pending != "":
			open = append(open, openGroup{at: i, brace: 0, closer: "}", function: pending})
		case groupMiddles[v]:
		case opens:
			open = append(open, openGroup{at: i, closer: closer})
			if groupHeads[v] {
				return open, ""
			}
		case defines:
			if v == "function" && k+1 < len(c.words) {
				k++
			}
			if k+1 < len(c.words) && c.words[k+1].raw == "{" {
				fn := openGroup{at: i, brace: k + 1, closer: "}", function: c.words[k].raw}
				return append(open, fn), ""
			}
			return open, c.words[k].raw
		default:
			return open, ""
		}
	}
	return open, ""
}

// group is the group g closes, read from text, with cmds the commands up to
// its closer. A function's first command keeps only what follows its {.
func (g openGroup) group(text string, cmds []shellCmd) *shellGroup {
	body := slices.Clone(cmds[g.at:])
	if g.function != "" {
		body[0] = afterBrace(body[0], g.brace)
	}
	return &shellGroup{text: text, cmds: body, function: g.function}
}

// afterBrace is c with its words up to and including the { at index brace
// dropped, and with them the empty ( ) of a function head in front of it.
func afterBrace(c shellCmd, brace int) shellCmd {
	at := c.words[brace].at.to
	c.words = c.words[brace+1:]
	c.subshells = slices.DeleteFunc(slices.Clone(c.subshells), func(s span) bool {
		return s.from < at
	})
	return c
}

// withGroup is fn with group set on each step it sees; fn itself for a nil
// group.
func withGroup(fn func(walkStep), group *shellGroup) func(walkStep) {
	if group == nil {
		return fn
	}
	return func(s walkStep) {
		s.group = group
		fn(s)
	}
}
