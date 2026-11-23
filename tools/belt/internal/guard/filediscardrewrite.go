package guard

import (
	"slices"
	"strings"
)

// This file holds file-discard's in-place edits: sed -i and perl -i write
// new text into each file they edit. A backup keeps the old text under
// another name, so the edit passes. A file the command names as plain text
// passes too: an agent edits a file it names on purpose, and the Edit tool
// is the way to do it with a diff (ADR-0033). The files a glob, a feeder,
// find's -exec or an expansion hands the edit are judged the way an
// overwrite's file is, one file each. GNU sed and perl replace a symlink
// with a new file and leave its target alone, and BSD sed refuses one, so
// only GNU's --follow-symlinks writes through a link.

// rewriteCommands are the commands that edit files in place, by the name
// each runs as: GNU sed from Homebrew installs as gsed.
var rewriteCommands = map[string]string{"sed": "sed", "gsed": "sed", "perl": "perl"}

// isRewriteFamily reports whether a loss command name is sed or perl.
func isRewriteFamily(name string) bool { return name == "sed" || name == "perl" }

// rewriteKeep is the lossless form a rewrite's denial offers.
const rewriteKeep = "To keep the work, commit it first, add a backup suffix " +
	"(`sed -i.bak`, `perl -pi.bak`), or make the change with the Edit tool."

// rewriteCall is the call for an in-place edit: a replace of each file, so
// the ladder takes it one file at a time.
func rewriteCall() *lossCall {
	return &lossCall{replace: true, rewrite: true, keep: rewriteKeep}
}

// namedLiterally reports whether a word names one file as plain text: no
// expansion and no glob character the shell expands. A variable the
// command sets to a literal earlier is that literal here
// (resolveLiteralVars), so it names the file too.
func namedLiterally(w shWord) bool {
	for _, p := range w.parts {
		if p.kind != partLit || !p.quoted && strings.ContainsAny(p.text, "*?[") {
			return false
		}
	}
	return len(w.parts) > 0
}

// maySplit reports whether the shell may cut a word into several: it holds
// an unquoted $NAME or $( ), or an expansion belt does not read ("$@"
// splits even in quotes).
func maySplit(w shWord) bool {
	for _, p := range w.parts {
		switch p.kind {
		case partVar, partSubst:
			if !p.quoted {
				return true
			}
		case partUnknown:
			return true
		}
	}
	return false
}

// appendSplit adds w to words when the shell may cut it into several
// (maySplit).
func appendSplit(words []shWord, w shWord) []shWord {
	if maySplit(w) {
		return append(words, w)
	}
	return words
}

// keepsBackup reports whether an in-place suffix or extension keeps the
// old text under another name. GNU and BSD sed and perl add one without a
// `*` to the file's name. GNU sed and perl put the file's name, as the
// command gives it, in place of each `*`, so `*` and `./*` name the file
// itself and keep nothing. A suffix with a `*` counts only when the name it
// makes can't be the file: it holds text other than `*`, `.` and `/`, and
// it neither starts at the root nor holds a `..` part, either of which may
// lead back to the file's own folder.
func keepsBackup(suffix string) bool {
	if !strings.Contains(suffix, "*") {
		return suffix != ""
	}
	if strings.HasPrefix(suffix, "/") || slices.Contains(strings.Split(suffix, "/"), "..") {
		return false
	}
	other := func(r rune) bool { return !strings.ContainsRune("*./", r) }
	return strings.ContainsFunc(suffix, other)
}

// sedSpec is how sed reads its options, GNU's and BSD's together. -i and
// -I take a backup suffix only in the same word: GNU reads a separate word
// as the script or a file, and BSD as the suffix, so a separate word stays
// an operand and makes no backup.
var sedSpec = wrapperSpec{
	short: "abEe:f:i::I::l:nrsuz",
	long: "binary debug expression: file: follow-symlinks help in-place:: line-length: null-data " +
		"posix quiet regexp-extended sandbox separate silent unbuffered version zero-terminated",
}

// sedOperands reads a sed's arguments into the files it edits in place:
// none without -i, -I or --in-place, and none when the last of them gives a
// suffix both GNU and BSD sed keep a backup under (keepsBackup). An option
// counts after an operand too, as GNU getopt reads it, and `--` ends them.
// The first operand is the script unless -e, -f or their long forms give
// one. A word belt cannot read is an operand, unless its text starts as an
// in-place option does: that one is read as -i with no suffix. A word the
// shell may cut into several (maySplit) may hold files, so as the script,
// an option's value or that -i it is judged as a file too.
func sedOperands(args []shWord) []lossOperand {
	var words, split []shWord
	inPlace, backup, script, follows, endOpts := false, false, false, false, false
	for i := 0; i < len(args); i++ {
		v, lit := args[i].literal()
		switch {
		case !lit && !endOpts && sedInPlaceHead(literalHead(args[i])):
			inPlace, backup = true, false
			split = appendSplit(split, args[i])
			continue
		case endOpts || !lit || len(v) < 2 || v[0] != '-':
			words = append(words, args[i])
			continue
		case v == "--":
			endOpts = true
			continue
		}
		opts, ok := sedSpec.options(sedLong(v))
		if !ok {
			continue
		}
		for _, o := range opts {
			switch o.name {
			case "i", "I", "in-place":
				inPlace, backup = true, keepsBackup(o.value)
			case "e", "f", "expression", "file":
				script = true
			case "follow-symlinks":
				follows = true
			}
			if o.arg == argNeeded && !o.attached && i+1 < len(args) {
				i++
				split = appendSplit(split, args[i])
			}
		}
	}
	if !inPlace || backup {
		return nil
	}
	if !script && len(words) > 0 && !maySplit(words[0]) {
		words = words[1:]
	}
	words = append(words, split...)
	ops := make([]lossOperand, len(words))
	for i, w := range words {
		ops[i] = lossOperand{word: w, follows: follows}
	}
	return ops
}

// sedInPlaceHead reports whether the literal text a word starts with reads
// as one of sed's in-place options: -i, -I or a prefix of --in-place.
func sedInPlaceHead(head string) bool {
	return strings.HasPrefix(head, "-i") || strings.HasPrefix(head, "-I") ||
		len(head) > 3 && strings.HasPrefix("--in-place", strings.TrimSuffix(head, "="))
}

// sedLong spells a long option by the full name of the one sed reads it
// as, an exact name or a unique prefix (longOption); any other word as it
// is.
func sedLong(v string) string {
	rest, ok := strings.CutPrefix(v, "--")
	if !ok {
		return v
	}
	name, value, hasValue := strings.Cut(rest, "=")
	if full := longOption(strings.Fields(strings.ReplaceAll(sedSpec.long, ":", "")), name); full != "" {
		name = full
	}
	if hasValue {
		return "--" + name + "=" + value
	}
	return "--" + name
}

// perlOperands reads a perl's arguments into the files -i edits in place:
// none without -i, and none when the last -i gives an extension under
// which perl keeps a backup (keepsBackup). perl reads switches up to the
// first word that is none, or `--` (perlSwitches). With no -e or -E the
// first word after them is the program file, and the rest are the files.
// A program file or switch value the shell may cut into several words
// (maySplit) may hold files, so it is judged as a file too.
func perlOperands(args []shWord) []lossOperand {
	var split []shWord
	inPlace, backup, code := false, false, false
	i := 0
	for ; i < len(args); i++ {
		v, lit := args[i].literal()
		if !lit || len(v) < 2 || v[0] != '-' {
			break
		}
		if v == "--" {
			i++
			break
		}
		sw := perlSwitches(v[1:])
		if sw.inPlace {
			inPlace, backup = true, keepsBackup(sw.ext)
		}
		code = code || sw.code
		if sw.takesNext && i+1 < len(args) {
			i++
			split = appendSplit(split, args[i])
		}
	}
	if !inPlace || backup {
		return nil
	}
	files := args[i:]
	if !code && len(files) > 0 && !maySplit(files[0]) {
		files = files[1:]
	}
	files = append(slices.Clone(files), split...)
	ops := make([]lossOperand, len(files))
	for k, w := range files {
		ops[k] = lossOperand{word: w}
	}
	return ops
}

// perlSwitch is what one word of perl switches gives: -i and the extension
// after it, code from -e or -E, and whether the next word is a switch's
// value (takesNext).
type perlSwitch struct {
	inPlace, code, takesNext bool
	ext                      string
}

// perlValueRest are the switches that take the rest of their word as a
// value, which may be empty.
const perlValueRest = "CdDFmMVx"

// perlSwitches reads one word of perl switches, its dash cut off, as perl
// reads a cluster: -i takes the rest of the word as its extension; -e, -E
// and -I take the rest, or else the next word; -0 and -l take the digits
// after them and go on; the perlValueRest switches take the rest. Any
// other letter is a switch of its own.
func perlSwitches(s string) perlSwitch {
	var sw perlSwitch
	for k := 0; k < len(s); k++ {
		switch c := s[k]; {
		case c == 'i':
			sw.inPlace, sw.ext = true, s[k+1:]
			return sw
		case c == 'e' || c == 'E':
			sw.code, sw.takesNext = true, k+1 == len(s)
			return sw
		case c == 'I':
			sw.takesNext = k+1 == len(s)
			return sw
		case c == '0':
			k = perlNumberEnd(s, k+1) - 1
		case c == 'l':
			k = digitsEnd(s, k+1, "01234567") - 1
		case strings.IndexByte(perlValueRest, c) >= 0:
			return sw
		}
	}
	return sw
}

// perlNumberEnd is where -0's number ends in s, from at: hex digits after
// an x, octal ones otherwise.
func perlNumberEnd(s string, at int) int {
	if at < len(s) && (s[at] == 'x' || s[at] == 'X') {
		return digitsEnd(s, at+1, "0123456789abcdefABCDEF")
	}
	return digitsEnd(s, at, "01234567")
}

// digitsEnd is the index of the first byte in s from at that is not in
// digits.
func digitsEnd(s string, at int, digits string) int {
	for at < len(s) && strings.IndexByte(digits, s[at]) >= 0 {
		at++
	}
	return at
}
