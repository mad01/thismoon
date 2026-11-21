package guard

import (
	"regexp"
	"slices"
	"strings"
)

// This file holds the consumer judge. xargs, GNU parallel, fd's -x and -X
// and find's -exec each run a command over input they read, and this one
// judge reads which command that is and what feeds it.

// consumerRun is a command a consumer runs over its input: who runs it,
// the command's words as the consumer hands them, the text that stands
// for one input in them (mark: find's and fd's {}, xargs' -I text,
// parallel's {} or -I text; "" for none), the texts the consumer fills in
// (marks, fd's {/} and parallel's {.} among them; nil for none), and
// whether the inputs follow the command's words when no word holds a mark
// (appends). The inputs are feeds of paths and words (parallel's :::
// words). call carries the toss-bin form of a delete, nil for the one
// that names the command.
type consumerRun struct {
	who     string
	command []shWord
	mark    string
	marks   *regexp.Regexp
	appends bool
	feeds   []*pathFeed
	words   []shWord
	call    *lossCall
}

// holds reports whether a word holds a text the consumer fills in.
func (r consumerRun) holds(w shWord) bool {
	return r.marks != nil && r.marks.MatchString(w.raw)
}

// appended reports whether the inputs follow the command's words: the
// consumer appends them and no word holds a mark.
func (r consumerRun) appended() bool {
	return r.appends && !slices.ContainsFunc(r.command, r.holds)
}

// consumerTokens are the command's words as tokens: a literal word by its
// value, any other by its text.
func consumerTokens(words []shWord) []string {
	tokens := make([]string, len(words))
	for i, w := range words {
		tokens[i] = w.raw
		if v, ok := w.literal(); ok {
			tokens[i] = v
		}
	}
	return tokens
}

// consumer judges the command a consumer runs. Input it runs as code
// (runsCode) denies when the text that builds it names a delete
// (consumedCode). An rm, unlink or shred drops each input (consumedRm).
// Any other command that deletes is one belt does not read there, and
// denies when its words name a delete. A shell's -c script takes the input
// as its arguments (consumedScript). Any other command passes.
func (j *discardJudge) consumer(r consumerRun, s walkStep) *fileLoss {
	if r.runsCode() {
		return consumedCode(r, s)
	}
	name, cargs, isLoss := lossCommand(skipAssignments(r.command))
	switch {
	case isLoss && isRmFamily(name):
		return j.consumedRm(r, name, cargs, s)
	case isLoss && namesLossText(strings.Join(consumerTokens(r.command), " ")):
		return &fileLoss{why: "belt does not read a " + name + " that " + r.who + " runs, and it names a delete"}
	case isLoss:
		return nil
	}
	return j.consumedScript(r, s)
}

// runsCode reports whether a consumer runs its input as code: with no
// command word, or one that holds a mark (`parallel ::: 'rm x'`, `xargs
// -I{} {}`), with an eval, and with a shell's -c whose script holds a mark
// or is missing, so the first input is the script.
func (r consumerRun) runsCode() bool {
	tokens := consumerTokens(r.command)
	head := len(tokens) - len(stripWrappers(tokens))
	switch {
	case head >= len(tokens), r.holds(r.command[head]), evalAt(tokens) >= 0:
		return true
	}
	if at := inlineScriptAt(tokens); at >= 0 {
		return r.holds(r.command[at])
	}
	return inlineScriptAt(append(tokens, "input")) == len(tokens)
}

// consumedCode is the loss for a consumer that runs its input as code
// (runsCode), when the text that builds that input names a delete: the
// consumer's own words, the pipeline in front of it, its redirection
// targets and its heredoc bodies.
func consumedCode(r consumerRun, s walkStep) *fileLoss {
	for _, text := range []string{strings.Join(s.tokens, " "), s.pipeText, strings.Join(s.targets, " "), s.docText} {
		if namesLossText(text) {
			return &fileLoss{why: r.who + " runs its input as code, which belt does not read, and `" +
				strings.TrimSpace(text) + "` names a delete"}
		}
	}
	return nil
}

// consumedRm judges an rm, unlink or shred a consumer runs: a word that is
// the mark stands for each input, and when the consumer appends the
// inputs (consumerRun.appended) they follow the operands. An operand that
// holds a mark in part names a path belt cannot tell.
func (j *discardJudge) consumedRm(r consumerRun, name string, cargs []shWord, s walkStep) *fileLoss {
	if r.appended() {
		return j.rmInputs(r, name, append(slices.Clone(cargs), literalWord(feedMark)), feedMark, s)
	}
	return j.rmInputs(r, name, cargs, r.mark, s)
}

// rmInputs judges an rm, unlink or shred whose operands that are mark
// stand for each input of r: each word in their place, then each feed.
func (j *discardJudge) rmInputs(r consumerRun, name string, cargs []shWord, mark string, s walkStep) *fileLoss {
	call := r.call
	if call == nil {
		call = &lossCall{toss: "hand the paths to `" + TossBin + " --safe-mode -r` in place of `" + name + "`"}
	}
	for _, w := range cargs {
		if v, ok := w.literal(); (!ok || v != mark) && r.holds(w) {
			return unresolvable(call, w)
		}
	}
	for _, w := range r.words {
		args := slices.Clone(cargs)
		for i := range args {
			if v, ok := args[i].literal(); ok && v == mark {
				args[i] = w
			}
		}
		if loss := j.lossCall(call, name, args, s, nil); loss != nil {
			return loss
		}
	}
	for _, fd := range r.feeds {
		fd.mark = mark
		if loss := j.lossCall(call, name, cargs, s, fd); loss != nil {
			return loss
		}
	}
	return nil
}

// consumedScript judges a shell's -c script a consumer runs with the
// input as the script's arguments ($0, $1 and on). A script that runs one
// of them as code runs the input as code (consumedCode). A script that is
// one rm, unlink or shred, each operand a literal word or a quoted "$1",
// "${1}" or "$@", drops what each reference names (scriptOperands), and
// the walk's own step for that script, which cannot see the input, is
// skipped (discardJudge.covered). The walk judges any other script, when
// it reads this same one; one it does not read, as a second -exec's,
// denies when it names a delete.
func (j *discardJudge) consumedScript(r consumerRun, s walkStep) *fileLoss {
	at := inlineScriptAt(consumerTokens(r.command))
	if at < 0 {
		return nil
	}
	walked := walkedScript(s) == r.command[at].raw
	script, lit := r.command[at].literal()
	cmds, read := readShell(script)
	if lit && read && slices.ContainsFunc(cmds, func(c shellCmd) bool { return runsPositional(c, c.tokens()) }) {
		return consumedCode(r, s)
	}
	if lit && read && len(cmds) == 1 && len(cmds[0].subshells) == 0 && len(cmds[0].docs) == 0 {
		args, ok := scriptOperands(plainWords(cmds[0].words), r.command[at+1:], r.appended())
		if name, cargs, isLoss := lossCommand(skipAssignments(args)); ok && isLoss && isRmFamily(name) {
			if walked {
				j.covered[s.id] = true
			}
			mark := r.mark
			if r.appended() {
				mark = feedMark
			}
			return j.rmInputs(r, name, cargs, mark, s)
		}
	}
	if !walked && namesLossText(r.command[at].raw) {
		return &fileLoss{why: "belt does not read the script " + r.who + " runs there, and it names a delete"}
	}
	return nil
}

// walkedScript is the text of the -c script the walk reads for a step
// (shellWalk.walkWords), "" for none.
func walkedScript(s walkStep) string {
	if at := inlineScriptAt(s.tokens); at >= 0 && at < len(s.words) {
		return s.words[at].raw
	}
	return ""
}

// scriptArgWord is a quoted reference to one script argument or to all
// of them past $0: "$1", "${1}", "$@".
var scriptArgWord = regexp.MustCompile(`^"\$(?:([0-9])|\{([0-9])\}|@)"$`)

// scriptOperands returns the words of a -c script's command with each
// scriptArgWord replaced by the words it names: the words after the
// script ($0 first), then the inputs when the consumer appends them
// (feedMark). ok is false for a word with any other expansion, and for a
// reference to an argument nothing gives.
func scriptOperands(words, after []shWord, appended bool) (out []shWord, ok bool) {
	input := literalWord(feedMark)
	for _, w := range words {
		if _, lit := w.literal(); lit {
			out = append(out, w)
			continue
		}
		m := scriptArgWord.FindStringSubmatch(w.raw)
		switch {
		case m == nil:
			return nil, false
		case m[1] == "" && m[2] == "":
			if len(after) > 1 {
				out = append(out, after[1:]...)
			}
			if appended {
				out = append(out, input)
			}
			continue
		}
		switch k := int((m[1] + m[2])[0] - '0'); {
		case k < len(after):
			out = append(out, after[k])
		case appended:
			out = append(out, input)
		default:
			return nil, false
		}
	}
	return out, true
}
