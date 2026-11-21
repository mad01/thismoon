package guard

import (
	"fmt"
	"path/filepath"
	"strings"
)

// xargsFeeds returns what an xargs runs from the items it reads, when it
// runs them as code (xargsRunsItems): a shell's or an interpreter's -c with
// no script of its own runs each item as its script (`xargs -0 sh -c`), and
// one whose script holds -I's string runs that script with the item in its
// place (`xargs -I{} sh -c {}`). The items are cut (xargsItems) from -a's
// file, or else from what the command reads on its standard input
// (inputScripts), the text `| sh` would read there. Each item is a feed of
// its own, as xargs starts a shell for each. Input the walk cannot read is
// one unread feed, so the command is opaque as `curl … | sh` is.
func (w *shellWalk) xargsFeeds(text string, c shellCmd, tokens []string, in pipeIn, dir string) []feed {
	at := runAt(tokens)
	call, ok := xargsRunsItems(shellNames{}, tokens[at:])
	if !ok {
		return nil
	}
	inner, _ := unwrapCommand(call.command)
	head := len(tokens) - len(inner)
	run := shellNames{}.withRunner(inner)
	name := filepath.Base(run[0])
	base := feed{
		runner: fmt.Sprintf("the text `xargs` hands `%s` to run", tokens[head]),
		dir:    dir, pre: tokens[:head], positional: call.replace == "",
	}
	if !shellInterpreters[name] {
		base.runner = fmt.Sprintf("the `%s` script `xargs` runs", tokens[head])
		base.interp, base.lang = true, interpreterLang(name)
	}
	template := ""
	if use := scriptOf(run); use.at > 0 {
		template = c.words[head+use.at].script()
		if !strings.Contains(template, call.replace) {
			template = call.replace // an expansion belt cannot read holds the item
		}
	}
	operand := -1
	if call.itemFile > 0 {
		operand = at + call.itemFile
	}
	inputs := []script{{}} // -a's file glued to its option (`-0a<(…)`) is unread
	if operand < 0 || !strings.HasPrefix(c.words[operand].raw, "-") {
		inputs = w.inputScripts(text, c, in.from, operand, true, dir)
	}
	var out []feed
	for _, s := range inputs {
		if !s.ok {
			out = append(out, base.with(s))
			continue
		}
		for _, item := range xargsItems(s.text, call) {
			if template != "" {
				item = strings.ReplaceAll(template, call.replace, item)
			}
			out = append(out, base.with(script{text: item, ok: true}))
		}
	}
	return out
}

// appendsItems reports whether a command, its words from a function's body
// on (bodyOf), is an xargs that adds the items it reads to the words of the
// command it runs: one with no -I (-i, -J, --replace) string. A shell's -c
// script there then runs with them as its arguments, $0 the first, even
// when the script is the last word (`xargs -0 sh -c 'eval "$0"'`).
func appendsItems(run []string) bool {
	at := runAt(run)
	return at < len(run) && filepath.Base(run[at]) == "xargs" && readXargs(run[at:]).replace == ""
}

// xargsItems cuts text into the items xargs reads from it. After -0 or -d
// they end at that separator alone (delimByte). Else they end at a newline,
// and at a blank too unless -I (-i, -J, --replace) makes each line one
// item, with quotes and backslashes taken off as xargs takes them off. A
// quote left open runs to the end of the text, so the walk reads more than
// xargs, which stops there.
func xargsItems(text string, call xargsCall) []string {
	if call.split {
		b, ok := delimByte(call.sep)
		if !ok {
			return []string{text}
		}
		sep := string([]byte{b})
		return strings.Split(strings.TrimSuffix(text, sep), sep)
	}
	lines := call.replace != ""
	var items []string
	var b strings.Builder
	have := false
	var quote byte
	for i := 0; i < len(text); i++ {
		switch ch := text[i]; {
		case quote != 0 && ch == quote:
			quote = 0
		case quote != 0:
			b.WriteByte(ch)
		case ch == '\'' || ch == '"':
			quote, have = ch, true
		case ch == '\\' && i+1 < len(text):
			i++
			b.WriteByte(text[i])
			have = true
		case ch == '\n' || !lines && (ch == ' ' || ch == '\t'):
			if have {
				items = append(items, b.String())
			}
			b.Reset()
			have = false
		case lines && !have && (ch == ' ' || ch == '\t'):
		default:
			b.WriteByte(ch)
			have = true
		}
	}
	if have {
		items = append(items, b.String())
	}
	return items
}
