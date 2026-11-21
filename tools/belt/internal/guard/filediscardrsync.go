package guard

import (
	"regexp"
	"strconv"
	"strings"
)

// This file holds rsync's --delete and --remove-source-files, and xargs,
// which runs a command over the paths a feeder prints.

// rsyncCall is an rsync command read for file-discard.
type rsyncCall struct {
	operands                                                      []shWord
	deletes, removesSources, dryRun, backup, recursive, filesFrom bool
}

// The rsync options that take a value, long and short.
var rsyncValues = wordSet("info debug stderr backup-dir suffix chmod block-size rsh rsync-path " +
	"max-delete max-size min-size max-alloc partial-dir timeout contimeout modify-window temp-dir " +
	"compare-dest copy-dest link-dest compress-choice zc compress-level zl skip-compress filter " +
	"exclude exclude-from include include-from files-from address port sockopts out-format " +
	"log-format log-file log-file-format password-file early-input bwlimit stop-after stop-at " +
	"time-limit write-batch only-write-batch read-batch protocol iconv checksum-seed " +
	"checksum-choice cc usermap groupmap chown outbuf remote-option copy-as config dparam")

const rsyncValueShorts = "efTBM@"

// readRsync reads rsync's words: the operands, past every option and its
// value, and the options file-discard judges by.
func readRsync(args []shWord) rsyncCall {
	var r rsyncCall
	endOpts := false
	for i := 0; i < len(args); i++ {
		v, lit := args[i].literal()
		switch {
		case endOpts || !lit || len(v) < 2 || v[0] != '-':
			r.operands = append(r.operands, args[i])
		case v == "--":
			endOpts = true
		case strings.HasPrefix(v, "--"):
			r.readLong(v, &i)
		default:
			r.readShorts(v, &i)
		}
	}
	return r
}

// readLong reads one long rsync option at args[*i].
func (r *rsyncCall) readLong(v string, i *int) {
	name, _, inline := strings.Cut(v[2:], "=")
	switch {
	case name == "del" || strings.HasPrefix(name, "delete"):
		r.deletes = true
	case name == "remove-source-files" || name == "remove-sent-files":
		r.removesSources = true
	case name == "dry-run" || name == "list-only":
		r.dryRun = true
	case name == "backup":
		r.backup = true
	case name == "recursive" || name == "archive":
		r.recursive = true
	}
	r.filesFrom = r.filesFrom || name == "files-from"
	if rsyncValues[name] && !inline {
		*i++
	}
}

// readShorts reads one cluster of short rsync options at args[*i]: a
// letter that takes a value takes the rest of the cluster or the next word.
func (r *rsyncCall) readShorts(v string, i *int) {
	for k := 1; k < len(v); k++ {
		switch c := v[k]; {
		case strings.IndexByte(rsyncValueShorts, c) >= 0:
			if k == len(v)-1 {
				*i++
			}
			return
		case c == 'n':
			r.dryRun = true
		case c == 'b':
			r.backup = true
		case c == 'r' || c == 'a':
			r.recursive = true
		}
	}
}

// remotePath reports whether an rsync operand names another host: rsync://
// or a colon before any slash (host:path, host::module).
func remotePath(raw string) bool {
	if strings.HasPrefix(raw, "rsync://") {
		return true
	}
	i := strings.IndexByte(raw, ':')
	return i > 0 && !strings.Contains(raw[:i], "/")
}

// rsync judges an rsync that deletes: with a --delete option the local
// destination is a tree drop, unless --backup keeps what it deletes; with
// --remove-source-files each local source is dropped, a tree with -r or
// -a. A dry run, a listing (one operand) and a remote path change nothing
// here.
func (j *discardJudge) rsync(args []shWord, s walkStep) *fileLoss {
	r := readRsync(args)
	if r.dryRun || len(r.operands) < 2 {
		return nil
	}
	dest := r.operands[len(r.operands)-1]
	if r.deletes && !r.backup && !remotePath(dest.raw) {
		call := &lossCall{keep: "To keep what it deletes, add --backup, so rsync keeps each file " +
			"it deletes as a backup copy, or run it without --delete."}
		if loss := j.operand(call, lossOperand{word: dest, tree: true, follows: true}, s); loss != nil {
			return loss
		}
	}
	if !r.removesSources {
		return nil
	}
	call := &lossCall{keep: "To keep the sources, run it without --remove-source-files."}
	if r.filesFrom {
		return &fileLoss{call: call, why: "rsync takes the sources it removes from a --files-from " +
			"list belt cannot see"}
	}
	for _, src := range r.operands[:len(r.operands)-1] {
		if remotePath(src.raw) {
			continue
		}
		if loss := j.operand(call, lossOperand{word: src, tree: r.recursive}, s); loss != nil {
			return loss
		}
	}
	return nil
}

// xargsDrop is an xargs command read for file-discard: the command it
// runs, the text -I or -J replaces with each path ("" when it appends
// them), whether it reads the paths from a file (-a), and how it cuts its
// input into paths: at a NUL with -0, at the -d byte (delim), else at
// blanks and newlines, reading quotes and backslashes as its own. why says
// why belt cannot read it.
type xargsDrop struct {
	command  []shWord
	replace  string
	fromFile bool
	nul      bool
	delim    string
	why      string
}

// xargsSplit are the bytes at which xargs without -0 or -d cuts its input,
// and the quotes and backslash it reads as its own.
const xargsSplit = " \t\n\v\f\r'\"\\"

// split returns the bytes at which xargs cuts its input into paths, ""
// for a NUL, which no path holds.
func (x xargsDrop) split() string {
	switch {
	case x.nul || x.delim == "\x00":
		return ""
	case x.delim != "":
		return x.delim
	}
	return xargsSplit
}

// The xargs options that take a value, GNU's and BSD's: long, and short
// in the next word or the rest of the cluster.
var xargsValues = wordSet("arg-file delimiter max-args max-procs max-chars process-slot-var")

const xargsValueShorts = "adEIJLnPRSs"

// readXargsDrop reads xargs' options, up to the command it runs. GNU's -e, -i
// and -l take a value only in the same word.
func readXargsDrop(args []shWord) xargsDrop {
	var x xargsDrop
	i := 0
	for ; i < len(args); i++ {
		v, ok := args[i].literal()
		if !ok || len(v) < 2 || v[0] != '-' {
			break
		}
		if v == "--" {
			i++
			break
		}
		if strings.HasPrefix(v, "--") {
			x.readLong(v, args, &i)
			continue
		}
		x.readShorts(v, args, &i)
	}
	x.command = args[min(i, len(args)):]
	return x
}

// readLong reads one long xargs option at args[*i], and its value.
func (x *xargsDrop) readLong(v string, args []shWord, i *int) {
	name, val, inline := strings.Cut(v[2:], "=")
	known := inline
	if xargsValues[name] && !inline && *i+1 < len(args) {
		*i++
		val, known = args[*i].literal()
	}
	switch {
	case name == "arg-file":
		x.fromFile = true
	case name == "null":
		x.nul = true
	case name == "delimiter":
		x.readDelim(val, known)
	case name == "replace" && inline:
		x.replace = val
	case name == "replace":
		x.replace = "{}"
	}
}

// readShorts reads one cluster of short xargs options at args[*i].
func (x *xargsDrop) readShorts(v string, args []shWord, i *int) {
	for k := 1; k < len(v); k++ {
		c := v[k]
		switch {
		case c == '0':
			x.nul = true
		case c == 'i':
			x.replace = "{}"
			if k+1 < len(v) {
				x.replace = v[k+1:]
			}
			return
		case c == 'e' || c == 'l':
			return
		case strings.IndexByte(xargsValueShorts, c) >= 0:
			val, known := v[k+1:], true
			if val == "" && *i+1 < len(args) {
				*i++
				val, known = args[*i].literal()
			}
			switch c {
			case 'I', 'J':
				x.replace = val
			case 'a':
				x.fromFile = true
			case 'd':
				x.readDelim(val, known)
			}
			return
		}
	}
}

// readDelim reads GNU xargs' -d value: one byte, or a backslash escape (\n,
// \t, \0, \\, an octal \NNN or a hex \xHH).
func (x *xargsDrop) readDelim(v string, known bool) {
	b, ok := delimByte(v)
	if !known || !ok {
		x.why = "belt cannot read the xargs delimiter `" + v + "`"
		return
	}
	x.delim = string(b)
}

// delimByte decodes one xargs -d value into its byte.
func delimByte(v string) (byte, bool) {
	switch {
	case len(v) == 1:
		return v[0], true
	case len(v) < 2 || v[0] != '\\':
		return 0, false
	}
	if c, ok := map[byte]byte{
		'n': '\n', 't': '\t', 'r': '\r', 'v': '\v', 'f': '\f', 'a': '\a', 'b': '\b', '\\': '\\',
	}[v[1]]; ok && len(v) == 2 {
		return c, true
	}
	base, digits := 8, v[1:]
	if v[1] == 'x' {
		base, digits = 16, v[2:]
	}
	n, err := strconv.ParseUint(digits, base, 8)
	return byte(n), err == nil && digits != ""
}

// xargs judges the command an xargs runs (discardJudge.consumer): its
// input is the paths the command in front of it prints (stdinFeed), each
// cut where xargs cuts its input, at the -I or -J text or after the
// command's own words. With no command it runs echo.
func (j *discardJudge) xargs(args []shWord, s walkStep) *fileLoss {
	x := readXargsDrop(args)
	if len(x.command) == 0 {
		return nil
	}
	fd := stdinFeed(s, "xargs")
	switch {
	case x.fromFile:
		fd = &pathFeed{why: "xargs reads its paths from a file belt cannot see"}
	case x.why != "":
		fd = &pathFeed{why: x.why}
	}
	fd.split = x.split()
	r := consumerRun{who: "xargs", command: x.command, mark: x.replace, appends: x.replace == "", feeds: []*pathFeed{fd}}
	if x.replace != "" {
		r.marks = regexp.MustCompile(regexp.QuoteMeta(x.replace))
	}
	return j.consumer(r, s)
}
