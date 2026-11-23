package guard

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// This file reads the command a launcher runs (launchers) against the deny
// list. Any word after the launcher's name may start that command, so any
// entry may start at any of those words. Each entry is read in one pass
// over the words, the way deny.leads would read it from each word on, so
// the read stays linear in the command's length. Read anew from each word,
// a command that repeats a tool's name costs the rest of the command once
// per word and entry. At a few thousand words that takes minutes, and the
// command runs once the hook times out.

// launchedDeny returns the first deny entry the command a launcher runs
// starts with (deny.leads), or nil when words start with no launcher
// (launches): of the entries that start at the earliest word after the
// launcher's name, the first in list order. A word that only happens to
// start an entry denies too. A regex entry matches anywhere in the words it
// is given, so it is tried once, from the first word on. Launchers behind a
// launcher need no reading of their own (`bundle exec npx prisma`), since
// the inner command's words are among the later words.
func launchedDeny(words []string, patterns []deny) *deny {
	if !launches(words) {
		return nil
	}
	r := readLaunch(words)
	found, at := -1, len(words)
	for j := range patterns {
		if i := r.firstStart(&patterns[j], at); i < at {
			found, at = j, i
		}
	}
	if found < 0 {
		return nil
	}
	return &patterns[found]
}

// launchRead is a launcher command read once for every entry. words are its
// words as written, the launcher's name first, dec the same words as the
// shell passes them (ansiCWords) and names their command names. line is the
// words after the name as one text, off where each word starts in it, and
// noise the same text with the quotes collapsed (lineNoise) and the space
// at its end trimmed, which nonSpace indexes; plain marks a line with no
// quotes to collapse. dashes holds, per word, where the first `--` at or
// after it is. The maps and slices after them keep the reads entries share.
type launchRead struct {
	words, dec, names []string
	line, noise       string
	plain             bool
	off, nonSpace     []int
	dashes            []int
	tools, leads      map[string][]bool
	destroy           []bool
	rmArgs            []rmArg
	rd                strings.Reader
}

// readLaunch reads a launcher command, its words with the name first.
func readLaunch(words []string) *launchRead {
	n := len(words)
	r := &launchRead{
		words:  words,
		dec:    ansiCWords(words),
		names:  make([]string, n),
		line:   strings.Join(words[1:], " "),
		off:    make([]int, n),
		dashes: make([]int, n+1),
		tools:  map[string][]bool{},
		leads:  map[string][]bool{},
	}
	for i, w := range r.dec {
		r.names[i] = commandName(w)
	}
	for i, at := 1, 0; i < n; i++ {
		r.off[i] = at
		at += len(words[i]) + 1
	}
	noise := lineNoise.Replace(r.line)
	r.plain = noise == r.line
	r.noise = strings.TrimRightFunc(noise, unicode.IsSpace)
	r.nonSpace = nextNonSpace(r.noise)
	r.dashes[n] = n
	for k := n - 1; k >= 0; k-- {
		r.dashes[k] = r.dashes[k+1]
		if r.dec[k] == "--" {
			r.dashes[k] = k
		}
	}
	return r
}

// nextNonSpace returns, for each byte of s that starts a rune and for its
// end, where the first rune at or after it that is not a space starts
// (unicode.IsSpace, as strings.TrimSpace reads one).
func nextNonSpace(s string) []int {
	out := make([]int, len(s)+1)
	var starts []int
	for i := range s {
		starts = append(starts, i)
	}
	next := len(s)
	out[len(s)] = next
	for k := len(starts) - 1; k >= 0; k-- {
		at := starts[k]
		if c, _ := utf8.DecodeRuneInString(s[at:]); !unicode.IsSpace(c) {
			next = at
		}
		out[at] = next
	}
	return out
}

// firstStart returns the first word before limit the entry starts at, or
// limit: where its argv match or its text match starts, or 1 for a regex
// entry that matches the words after the launcher's name.
func (r *launchRead) firstStart(d *deny, limit int) int {
	if strings.HasPrefix(d.pattern, "re:") {
		if limit > 1 && d.leads(r.words[1:]) {
			return 1
		}
		return limit
	}
	at := limit
	if d.entry != nil {
		at = r.argvStart(d.entry, at)
	}
	if d.re != nil {
		at = r.textStart(d, at)
	}
	return at
}

// toolAt returns, per word, whether it names the tool (commandName, case
// aside), the one word an entry's argv match may start at.
func (r *launchRead) toolAt(tool string) []bool {
	if at, ok := r.tools[tool]; ok {
		return at
	}
	at := make([]bool, len(r.words))
	for i := 1; i < len(at); i++ {
		at[i] = strings.EqualFold(r.names[i], tool)
	}
	r.tools[tool] = at
	return at
}

// leadAt returns, per word, whether it holds a text match's first word
// (deny.lead), kept by the lead's expression, which entries share.
func (r *launchRead) leadAt(lead *regexp.Regexp) []bool {
	key := lead.String()
	if at, ok := r.leads[key]; ok {
		return at
	}
	at := make([]bool, len(r.words))
	for i := 1; i < len(at); i++ {
		at[i] = lead.MatchString(r.words[i])
	}
	r.leads[key] = at
	return at
}

// textStart returns the first word before limit the entry's text match
// starts at (deny.leads), or limit. Only a word that names the entry's tool
// or holds the text match's first word is tried. An empty word, or one of
// quotes and brackets alone, is never one: a text match from it also
// matches from the next word on.
func (r *launchRead) textStart(d *deny, limit int) int {
	var tool []bool
	if d.entry != nil {
		tool = r.toolAt(d.entry.tool)
	}
	lead := r.leadAt(d.lead)
	for i := 1; i < limit; i++ {
		if (lead[i] || (tool != nil && tool[i])) && r.textAt(d.start, i) {
			return i
		}
	}
	return limit
}

// textAt reports whether the text from word i on starts with the entry's
// words (start): as written, or with the quotes collapsed and the space
// around it trimmed.
func (r *launchRead) textAt(start *regexp.Regexp, i int) bool {
	off := r.off[i]
	if r.startsWith(start, r.line[off:]) {
		return true
	}
	if off >= len(r.noise) {
		return false
	}
	from := r.nonSpace[off]
	if r.plain && from == off && len(r.noise) == len(r.line) {
		return false // the same text again
	}
	return r.startsWith(start, r.noise[from:])
}

// startsWith reports whether an expression anchored at the start matches a
// text. A reader feeds it the text, so the match reads only as far as it
// gets: on a string the engine may pick one that first clears a table as
// long as the text.
func (r *launchRead) startsWith(start *regexp.Regexp, text string) bool {
	r.rd.Reset(text)
	return start.MatchReader(&r.rd)
}

// argvStart returns the first word before limit the entry's argv match
// starts at (denyEntry.runs), or limit: a word that names the entry's tool,
// followed by every option, verb and phrase the entry names. A phrase found
// from a word on is found from any earlier word too, as the text the
// phrases read from there holds the later text whole (phraseText), so the
// phrases are read once, from the first word the rest matches at.
func (r *launchRead) argvStart(e *denyEntry, limit int) int {
	tool := r.toolAt(e.tool)
	if !slices.Contains(tool[:limit], true) {
		return limit
	}
	if e.tool == "rm" {
		return r.rmStart(e, tool, limit)
	}
	at := r.verbStart(e, r.optionStarts(e, tool, limit), limit)
	if at < limit && !e.phrasesIn(r.dec[at+1:]) {
		return limit
	}
	return at
}

// optionStarts returns the words before limit that name the tool and are
// followed by every option the entry requires (entryOption.in), in order.
func (r *launchRead) optionStarts(e *denyEntry, tool []bool, limit int) []int {
	from := make([][]bool, len(e.options))
	for j, o := range e.options {
		from[j] = r.beforeDash(func(k int) bool { return o.at(r.dec, k) })
	}
	var out []int
	for i := 1; i < limit; i++ {
		if tool[i] && allFrom(from, i+1) {
			out = append(out, i)
		}
	}
	return out
}

// rmStart returns the first word before limit that runs rm with every
// option and operand the rm entry names (rmRuns), or limit. rm reads an
// option or an operand among the words before the first `--` after its
// name (readRmArg), and every word past that `--` as an operand.
func (r *launchRead) rmStart(e *denyEntry, tool []bool, limit int) int {
	want, _ := readRm(e.rm)
	args := r.rmRead()
	var options, operands, after [][]bool
	for name := range want.options {
		options = append(options, r.beforeDash(func(k int) bool {
			return slices.Contains(args[k].options, name)
		}))
	}
	for _, operand := range want.operands {
		operands = append(operands, r.beforeDash(func(k int) bool {
			return args[k].operand && r.dec[k] == operand
		}))
		after = append(after, r.anyFrom(func(k int) bool { return r.dec[k] == operand }))
	}
	for i := 1; i < limit; i++ {
		if !tool[i] || !allFrom(options, i+1) {
			continue
		}
		dash := r.dashes[i+1]
		ok := true
		for j := range operands {
			ok = ok && (operands[j][i+1] || (dash < len(r.dec) && after[j][dash+1]))
		}
		if ok {
			return i
		}
	}
	return limit
}

// rmRead returns each word read as one of rm's arguments (readRmArg).
func (r *launchRead) rmRead() []rmArg {
	if r.rmArgs == nil {
		r.rmArgs = make([]rmArg, len(r.dec))
		for k, w := range r.dec {
			r.rmArgs[k] = readRmArg(w)
		}
	}
	return r.rmArgs
}

// beforeDash returns, for each word and the end, whether a word from it on
// and before the first `--` holds, read from the last word back.
func (r *launchRead) beforeDash(holds func(k int) bool) []bool {
	n := len(r.dec)
	out := make([]bool, n+1)
	for k := n - 1; k >= 1; k-- {
		out[k] = r.dec[k] != "--" && (out[k+1] || holds(k))
	}
	return out
}

// anyFrom returns, for each word and the end, whether a word from it on
// holds.
func (r *launchRead) anyFrom(holds func(k int) bool) []bool {
	n := len(r.dec)
	out := make([]bool, n+1)
	for k := n - 1; k >= 1; k-- {
		out[k] = out[k+1] || holds(k)
	}
	return out
}

// allFrom reports whether every one of the per-word reads holds at word k.
func allFrom(from [][]bool, k int) bool {
	for _, f := range from {
		if !f[k] {
			return false
		}
	}
	return true
}

// verbStart returns the first of starts, each a word that names the tool,
// whose verbs run in the words after it (verbsRun), or limit. One pass over
// the words reads them for every start at once (verbPass): a start joins
// the pass at the word after it.
func (r *launchRead) verbStart(e *denyEntry, starts []int, limit int) int {
	if len(starts) == 0 {
		return limit
	}
	if len(e.verbs) == 0 {
		return starts[0]
	}
	var destroy []bool
	if destroyTool(e.tool) {
		destroy = r.destroyFrom()
	}
	p := newVerbPass(e.verbs, limit)
	next := 0
	for k := 1; k < len(r.dec); k++ {
		fresh, on := p.found, 0
		if next < len(starts) && starts[next] == k-1 {
			fresh = starts[next]
			next++
			if destroy != nil && destroy[k] {
				on = 1
			}
		}
		p.read(r.dec[k], skippable(e.tool, r.dec[k]), r.takesNext(k-1), fresh, on)
	}
	return p.found
}

// takesNext reports whether word k is a flag that may take the word after
// it as its value (verbWords).
func (r *launchRead) takesNext(k int) bool {
	w := r.dec[k]
	return k >= 1 && w != "--" && mayBeFlag(w) && flagTakesNext(w)
}

// destroyFrom returns, for each word and the end, whether -destroy is on
// for the arguments from it on (destroys): the last -destroy flag before
// the first `--` decides.
func (r *launchRead) destroyFrom() []bool {
	if r.destroy != nil {
		return r.destroy
	}
	n := len(r.dec)
	on := make([]bool, n+1)
	seen := false
	for k := n - 1; k >= 1; k-- {
		if r.dec[k] == "--" {
			seen = false
			continue
		}
		if v, ok := destroyFlag(r.dec[k]); ok && !seen {
			on[k], seen = v, true
			continue
		}
		on[k] = on[k+1]
	}
	r.destroy = on
	return on
}

// verbPass reads a tool's arguments for an entry's verbs (verbsRun) from
// many starts in one pass. first[raw][on][j] is the first start whose read
// has matched j verbs. raw marks a read past a `--`, which reads every word
// as a verb word, flags too (verbWords), and on one with -destroy on for
// terraform or tofu (destroys). Two reads that agree on all three read the
// words left alike, so only the first start is kept. found is the first
// start that matched every verb, or the limit, and a read from a start no
// earlier is dropped.
type verbPass struct {
	verbs []string
	first [2][2][]int
	next  []int
	found int
}

// newVerbPass starts a pass with no read in it.
func newVerbPass(verbs []string, limit int) *verbPass {
	p := &verbPass{verbs: verbs, found: limit, next: make([]int, len(verbs))}
	for raw := range p.first {
		for on := range p.first[raw] {
			p.first[raw][on] = slices.Repeat([]int{limit}, len(verbs))
		}
	}
	return p
}

// read reads one word for every read in the pass, then for a fresh start's
// read, whose first word it is, with -destroy on or not; fresh is found for
// none. skip says the verbs may skip the word whatever comes before it
// (skippable), and afterFlag that the word before it is a flag that may take
// it as its value, which a fresh read has no part of.
func (p *verbPass) read(word string, skip, afterFlag bool, fresh, on int) {
	for o := range 2 {
		p.step(p.first[1][o], destroyText(word, o == 1), skip)
	}
	flags := &p.first[0]
	switch {
	case word == "--":
		for o := range 2 {
			for j, s := range flags[o] {
				p.first[1][o][j] = min(p.first[1][o][j], s)
				flags[o][j] = p.found
			}
		}
		p.first[1][on][0] = min(p.first[1][on][0], fresh)
	case mayBeFlag(word):
		flags[on][0] = min(flags[on][0], fresh)
	default:
		skip = skip || isAssignment(word)
		for o := range 2 {
			p.step(flags[o], destroyText(word, o == 1), skip || afterFlag)
		}
		p.stepFresh(flags[on], fresh, destroyText(word, on == 1), skip)
	}
}

// step reads one verb word for the reads in set: a read that matched j
// verbs matches one more when the word is the next verb, and stays where it
// is when the verbs may skip the word. Any other read ends.
func (p *verbPass) step(set []int, text string, skip bool) {
	next := p.next
	for j := range next {
		next[j] = p.found
	}
	for j, s := range set {
		if s >= p.found {
			continue
		}
		if strings.EqualFold(text, p.verbs[j]) {
			if j+1 == len(p.verbs) {
				p.found = s
			} else {
				next[j+1] = min(next[j+1], s)
			}
		}
		if skip {
			next[j] = min(next[j], s)
		}
	}
	copy(set, next)
}

// stepFresh reads a fresh start's first verb word into set, as step does.
func (p *verbPass) stepFresh(set []int, fresh int, text string, skip bool) {
	if fresh >= p.found {
		return
	}
	if strings.EqualFold(text, p.verbs[0]) {
		if len(p.verbs) == 1 {
			p.found = fresh
		} else {
			set[1] = min(set[1], fresh)
		}
	}
	if skip {
		set[0] = min(set[0], fresh)
	}
}
