package guard

import (
	"strings"
	"unicode"
)

// This file holds file-discard's reading of text a command runs that the
// walk could not read: a shell fed by a pipe of commands belt cannot read
// (`sed 's/^/rm /' list | sh`), and the output of a $( ) or <( ) run as
// code (`eval "$(sed 's/^/rm /' list)"`). Such a command denies when the
// text that builds the code names a delete. Text built some other way
// passes, an install script piped into sh among it: belt guards against a
// slip, and a delete spelled in pieces (`awk '{print "r" "m"}'`) is no slip.

// unreadLoss is the loss for code a command runs that belt could not read,
// when the text that builds it names a delete (namesLossText): an opaque
// command (walkStep.opaque), by its own words and the pipeline in front of
// it, and a substitution whose output it runs (walkStep.unreadSubst).
func unreadLoss(s walkStep) *fileLoss {
	if s.opaque.what != "" && (namesLossText(strings.Join(s.tokens, " ")) || namesLossText(s.pipeText)) {
		return &fileLoss{why: "belt cannot read " + s.opaque.what + ", and the commands that build it " +
			"name a delete, so it cannot tell what it deletes"}
	}
	for _, body := range s.unreadSubst {
		if namesLossText(body) {
			return &fileLoss{why: "this runs what `" + body + "` prints as code, which belt cannot " +
				"read, and that text names a delete"}
		}
	}
	return nil
}

// namesLossText reports whether text holds a loss command, find's -delete or
// one of rsync's delete options (namesLossCommand) as a word of its own,
// cut at every byte that cannot be part of a command name or an option:
// `s/^/rm /` and `print "rm " $0` both name rm.
func namesLossText(text string) bool {
	return namesLossCommand(strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("_-.+", r)
	}))
}

// frontText is the text of the commands in front of one in its pipeline,
// read from text: their words, redirection targets, subshells and heredoc
// bodies.
func frontText(text string, front []shellCmd) string {
	var b strings.Builder
	for _, c := range front {
		for _, words := range [][]placedWord{c.words, c.targets} {
			for _, w := range words {
				b.WriteString(w.raw + " ")
			}
		}
		for _, s := range c.subshells {
			b.WriteString(text[s.from:s.to] + " ")
		}
		for _, d := range c.docs {
			b.WriteString(text[d.body.from:d.body.to] + " ")
		}
	}
	return b.String()
}

// docBodies is the text of the heredoc bodies a command reads, read from
// text.
func docBodies(text string, c shellCmd) string {
	var b strings.Builder
	for _, d := range c.docs {
		b.WriteString(text[d.body.from:d.body.to] + " ")
	}
	return b.String()
}

// unreadSubst returns the code, read from text (wordPart.code), of the
// substitutions whose output a command runs as code and the walk cannot read
// (bodyProduced): a $( ) or backtick in its command word, in the words
// eval runs or in a shell's -c script, a <( ) a shell or source reads as
// its script operand or its standard input, and a $( ) or backtick in the
// here-string or the unquoted heredoc a shell reads as its script.
func (w *shellWalk) unreadSubst(text string, c shellCmd, tokens []string) []string {
	words, bodies := codeWords(text, c, tokens)
	for _, word := range words {
		for _, p := range word.parts {
			if runsBody(text, p) {
				bodies = append(bodies, p.code(text))
			}
		}
	}
	var out []string
	for _, body := range bodies {
		if s, known := w.bodyProduced(body); !known || !s.ok {
			out = append(out, body)
		}
	}
	return out
}

// codeWords returns the words of a command, read from text, whose
// substitutions it may run as code (unreadSubst), and the substitutions
// in the unquoted heredoc bodies a shell reads as its script.
func codeWords(text string, c shellCmd, tokens []string) ([]placedWord, []string) {
	var words []placedWord
	if head := len(tokens) - len(stripWrappers(tokens)); head < len(c.words) {
		words = append(words, c.words[head])
	}
	if at := evalAt(tokens); at >= 0 {
		words = append(words, c.words[at+1:]...)
	}
	if at := inlineScriptAt(tokens); at >= 0 && at < len(c.words) {
		words = append(words, c.words[at])
	}
	r, ok := readsScript(c, tokens)
	if !ok || !r.shell {
		return words, nil
	}
	if r.operand >= 0 && r.operand < len(c.words) {
		words = append(words, c.words[r.operand])
	}
	for _, t := range c.targets {
		if op := redirectOp(text, t); op == "<" || op == "<<<" {
			words = append(words, t)
		}
	}
	var bodies []string
	for _, d := range c.docs {
		if !d.quoted {
			bodies = append(bodies, bodySubstitutions(text, d.body)...)
		}
	}
	return words, bodies
}

// runsBody reports whether a word part is a $( ), backtick or <( ) whose
// output the command may run.
func runsBody(text string, p wordPart) bool {
	switch {
	case p.body.to <= p.body.from:
		return false
	case p.kind == partSubst:
		return true
	}
	return p.kind == partUnknown && p.body.from >= 2 && text[p.body.from-2:p.body.from] == "<("
}
