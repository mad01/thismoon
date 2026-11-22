package guard

import (
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

// xargs judges the command an xargs runs (discardJudge.consumer), read
// with readXargsWords: its input is the paths the command in front of it
// prints (stdinFeed), each cut where xargs cuts its input (xargsCall.cut),
// at the -I or -J text or after the command's own words. With no command
// it runs echo.
func (j *discardJudge) xargs(args []shWord, s walkStep) *fileLoss {
	x, command := readXargsWords(args)
	if len(command) == 0 {
		return nil
	}
	fd := stdinFeed(s, "xargs")
	switch why := x.delimWhy(); {
	case x.itemFile > 0:
		fd = &pathFeed{why: "xargs reads its paths from a file belt cannot see"}
	case why != "":
		fd = &pathFeed{why: why}
	}
	fd.split = x.cut()
	r := consumerRun{who: "xargs", command: command, mark: x.replace, appends: x.replace == "", feeds: []*pathFeed{fd}}
	if x.replace != "" {
		r.marks = x.marks()
	}
	return j.consumer(r, s)
}
