package guard

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// This file holds file-discard's copies: cp, mv, install and ln -f each put
// a file in place of the destination, or of the file with the source's
// name in it when the destination is a folder. A copy that keeps the old
// file (-n, --backup) passes. A folder a cp -r source lands on is merged
// into, which file-discard leaves alone (ADR-0008), so a folder in the
// destination's place passes.

// copyCommands are the commands that put a file in place of another, by the
// name each runs as: GNU coreutils from Homebrew installs them with a g in
// front.
var copyCommands = map[string]string{
	"cp": "cp", "gcp": "cp", "mv": "mv", "gmv": "mv", "install": "install", "ginstall": "install",
	"ln": "ln", "gln": "ln",
}

// isCopyFamily reports whether a loss command name is cp, mv, install or ln.
func isCopyFamily(name string) bool {
	return name == "cp" || name == "mv" || name == "install" || name == "ln"
}

// copySpec is how one copy command reads its options, marked as
// wrapperSpec marks them: GNU coreutils 9's and BSD's from the macOS man
// pages. A letter the two read apart is read the way that judges more:
// cp's and install's -S as BSD's flag, not GNU's backup suffix. backup
// names the options that keep the old file under another name, noClobber
// those that leave it alone and forces those that undo them; noRun names
// the options after which the command writes no file, forces for ln the
// ones that make it replace one, file the ones that take the destination
// as a file even when it is a folder, and noDeref those that take a
// symlink there as a file.
type copySpec struct {
	wrapperSpec
	backup, noClobber, forces, noRun, file, noDeref string
}

var copySpecs = map[string]copySpec{
	"cp": {
		wrapperSpec: wrapperSpec{
			short: "abcdfHilLnpPRrsSt:TuvxXZ",
			long: "archive attributes-only backup:: copy-contents debug force interactive dereference " +
				"link no-clobber no-dereference preserve:: no-preserve: parents recursive reflink:: " +
				"remove-destination sparse: strip-trailing-slashes symbolic-link suffix: " +
				"target-directory: no-target-directory update:: verbose keep-directory-symlink " +
				"one-file-system context:: help version",
		},
		backup: "b backup suffix", noClobber: "n no-clobber", forces: "f i force interactive",
		noRun: "help version", file: "T no-target-directory",
	},
	"mv": {
		wrapperSpec: wrapperSpec{
			short: "bfhinS:t:TuvZ",
			long: "backup:: debug exchange force interactive no-clobber no-copy strip-trailing-slashes " +
				"suffix: target-directory: no-target-directory update:: verbose context:: help version",
		},
		backup: "b S backup suffix", noClobber: "n no-clobber", forces: "f i force interactive",
		noRun: "exchange help version", file: "T no-target-directory", noDeref: "h",
	},
	"install": {
		wrapperSpec: wrapperSpec{
			short: "bB:cCdDf:g:h:l:Mm:N:o:pSst:TUvZ",
			long: "backup:: compare debug directory group: mode: owner: preserve-timestamps strip " +
				"strip-program: suffix: target-directory: no-target-directory verbose " +
				"preserve-context context:: help version",
		},
		backup: "b backup suffix", noRun: "d directory help version", file: "T no-target-directory",
	},
	"ln": {
		wrapperSpec: wrapperSpec{
			short: "bdFfhiLnPrsS:t:Tvw",
			long: "backup:: directory force interactive logical no-dereference physical relative " +
				"symbolic suffix: target-directory: no-target-directory verbose help version",
		},
		backup: "b S backup suffix", forces: "f F i force interactive", noRun: "help version",
		file: "T no-target-directory", noDeref: "h n no-dereference",
	},
}

// copyCall is a copy command read for file-discard (readCopy): its name,
// its operands, -t's folder (nil for none), and what its options say.
// parents marks cp --parents, which keeps each source's path below the
// folder. control is the value of the last --backup=, GNU's version control
// (makesBackup).
type copyCall struct {
	name, control                          string
	ops                                    []shWord
	target                                 *shWord
	backup, noClobber, forced, noRun, file bool
	noDeref, parents                       bool
}

// readCopy reads a copy command's arguments with its copySpec. An option
// counts after an operand too, as GNU getopt reads it, and `--` ends them.
// An option the command does not know is read as a flag. -n and -f or -i
// undo each other, the last one counting, as mv and BSD cp read them.
func readCopy(name string, args []shWord) copyCall {
	spec := copySpecs[name]
	c := copyCall{name: name}
	endOpts := false
	for i := 0; i < len(args); i++ {
		v, lit := args[i].literal()
		switch {
		case endOpts || !lit || len(v) < 2 || v[0] != '-':
			c.ops = append(c.ops, args[i])
			continue
		case v == "--":
			endOpts = true
			continue
		}
		opts, ok := spec.options(v)
		if !ok || len(opts) == 0 {
			continue
		}
		last := &opts[len(opts)-1]
		value := dropHead(args[i], len(v)-len(last.value))
		if last.arg == argNeeded && !last.attached {
			if i++; i == len(args) {
				break
			}
			value = args[i]
			last.value, _ = value.literal()
		}
		for _, o := range opts {
			c.note(spec, o, value)
		}
	}
	return c
}

// note records what one option says; value is the word that holds its
// value.
func (c *copyCall) note(spec copySpec, o option, value shWord) {
	switch {
	case o.name == "t" || o.name == "target-directory":
		c.target = &value
	case o.name == "parents":
		c.parents = true
	case o.name == "update":
		c.noClobber = o.value == "none" || o.value == "none-fail"
	case hasWord(spec.noClobber, o.name):
		c.noClobber = true
	case hasWord(spec.forces, o.name):
		c.noClobber, c.forced = false, true
	}
	if o.name == "backup" && o.attached {
		c.control = o.value
	}
	c.backup = c.backup || hasWord(spec.backup, o.name)
	c.noRun = c.noRun || hasWord(spec.noRun, o.name)
	c.file = c.file || hasWord(spec.file, o.name)
	c.noDeref = c.noDeref || hasWord(spec.noDeref, o.name)
}

// keeps reports whether the copy replaces no file: it writes none, keeps
// the old one under another name (makesBackup), leaves an existing one
// alone, or is an ln with no -f, which fails on one.
func (c copyCall) keeps() bool {
	return c.noRun || c.makesBackup() || c.noClobber || c.name == "ln" && !c.forced
}

// makesBackup reports whether a backup option asks for a backup and the
// last --backup= value lets it: GNU's none and off, and any prefix GNU
// reads as one of them, make none. -b, a bare --backup and the suffix
// options keep that value, as GNU reads them.
func (c copyCall) makesBackup() bool {
	v := c.control
	none := len(v) >= 2 && strings.HasPrefix("none", v) || v != "" && strings.HasPrefix("off", v)
	return c.backup && !none
}

// follows reports whether the copy writes through a symlink in the
// destination's last part: cp opens the file there, and belt reads install
// the same way. mv and ln replace the name.
func (c copyCall) follows() bool { return c.name == "cp" || c.name == "install" }

// operands returns a copy's sources and destination: with -t every operand
// is a source, else the last operand is the destination, and an ln of one
// operand links into the shell's folder. ok is false for a copy with too
// few operands to write a file.
func (c copyCall) operands() (sources []shWord, dest shWord, ok bool) {
	switch n := len(c.ops); {
	case c.target != nil:
		return c.ops, *c.target, n > 0
	case n >= 2:
		return c.ops[:n-1], c.ops[n-1], true
	case n == 1 && c.name == "ln":
		return c.ops, literalWord("."), true
	}
	return nil, shWord{}, false
}

// copyInput is what a consumer hands the copy it runs: the feed of its
// inputs, whose mark stands for one, how to tell a word that holds a mark
// in part, and whether the consumer appends the inputs, so the last of
// them is the destination.
type copyInput struct {
	fd       *pathFeed
	holds    func(shWord) bool
	appended bool
}

// marks reports whether a word is the input's mark (whole) or holds a mark
// in part. A nil input marks nothing.
func (in *copyInput) marks(w shWord) (whole, part bool) {
	if in == nil {
		return false, false
	}
	if v, ok := w.literal(); ok && v == in.fd.mark {
		return true, false
	}
	return false, in.holds(w)
}

// copy judges a cp, mv, install or ln the shell runs (copyInto).
func (j *discardJudge) copy(name string, args []shWord, s walkStep) *fileLoss {
	return j.copyInto(readCopy(name, args), s, nil)
}

// copyInto judges the files a copy writes over. A destination that is a
// folder (followed through a symlink, unless -T or ln -n says otherwise)
// takes a file of each source's name (copySource); any other destination
// is the one file written. A destination that is a consumer's input is
// each input (fedCopy), and the last of the inputs a consumer appends with
// no -t is one belt cannot tell. A destination a mktemp variable starts
// passes, and one belt cannot place is judged as an operand, which names
// why.
func (j *discardJudge) copyInto(c copyCall, s walkStep, in *copyInput) *fileLoss {
	if c.keeps() {
		return nil
	}
	if in != nil && in.appended && c.target == nil {
		return &fileLoss{call: copyLoss("", s), why: "the last path the feeder hands `" + c.name +
			"` is where it writes, so belt cannot tell which path that is: give it a -t folder"}
	}
	sources, dest, ok := c.operands()
	if !ok {
		return nil
	}
	if whole, part := in.marks(dest); whole || part {
		return j.fedCopy(c, dest, whole, s, in)
	}
	follows := c.follows()
	if v, lit := dest.literal(); in != nil && in.fd.inMatchDir && (!lit || !path.IsAbs(v)) {
		return &fileLoss{call: copyLoss("", s), why: "`" + dest.raw + "` is relative to the folder " +
			"of each match, so belt cannot tell which path it names"}
	}
	p, temp, placed := j.placeDest(dest, s)
	switch {
	case temp:
		return nil
	case !placed:
		if _, active, ok := j.scan.staticGlobs(dest); ok && slices.Contains(active, true) {
			return unresolvable(copyLoss("", s), dest)
		}
		return j.operand(copyLoss(dest.raw, s), lossOperand{word: dest, follows: follows}, s)
	}
	if !c.folder(p) {
		if c.target != nil || len(sources) > 1 || strings.HasSuffix(p, "/") {
			return nil
		}
		return j.drop(copyLoss(dest.raw, s), false, nil, kernelPath(p, follows))
	}
	for _, src := range sources {
		if loss := j.copySource(c, p, strings.TrimRight(dest.raw, "/"), src, s, in); loss != nil {
			return loss
		}
	}
	return nil
}

// folder reports whether a copy puts its sources in the destination p
// rather than in its place: p is a folder, or a symlink to one that the
// copy follows (no -T, no ln -n or mv -h).
func (c copyCall) folder(p string) bool {
	if c.file || !isDir(p) {
		return false
	}
	fi, err := os.Lstat(strings.TrimRight(p, "/"))
	return !c.noDeref || err != nil || fi.Mode()&fs.ModeSymlink == 0 || strings.HasSuffix(p, "/")
}

// copySource judges the file one source lands on in the folder dir,
// written destRaw: the file of the source's name (baseWord), or for cp
// --parents of its whole path. A glob source lands on each entry of dir
// its last part matches. A source that is the consumer's input, or a whole
// $( ) of a feeder, may land on any entry of dir (intoFolder).
func (j *discardJudge) copySource(c copyCall, dir, destRaw string, src shWord, s walkStep, in *copyInput) *fileLoss {
	whole, part := in.marks(src)
	body, subst := substBody(src)
	switch {
	case part:
		return unresolvable(copyLoss("", s), src)
	case (whole || subst) && c.parents:
		return &fileLoss{call: copyLoss("", s), why: "belt does not read a cp --parents of paths a feeder hands it"}
	case whole:
		return j.intoFolder(dir, destRaw, c.follows(), in.fd, s)
	case subst:
		return j.intoFolder(dir, destRaw, c.follows(), substFeed(src, body), s)
	}
	name, ok := baseWord(src)
	if c.parents {
		name, ok = src, true
	}
	if !ok {
		return unresolvable(copyLoss("", s), src)
	}
	if v, lit := name.literal(); lit && (v == "." || v == "..") {
		return nil
	}
	w := shWord{
		raw:   destRaw + "/" + name.raw,
		parts: append([]wordPart{{kind: partLit, text: strings.TrimRight(dir, "/") + "/", quoted: true}}, name.parts...),
	}
	return j.pathOperand(copyLoss(w.raw, s), lossOperand{word: w, follows: c.follows()}, w, s)
}

// baseWord is the last part of a path word, its trailing slashes dropped:
// the name a copy gives the source in a folder. ok is false when that part
// holds an expansion or is empty.
func baseWord(w shWord) (shWord, bool) {
	parts := slices.Clone(w.parts)
	for len(parts) > 0 {
		last := len(parts) - 1
		if parts[last].kind != partLit {
			break
		}
		if t := strings.TrimRight(parts[last].text, "/"); t != "" {
			parts[last].text = t
			break
		}
		parts = parts[:last]
	}
	start := 0
	for i := len(parts) - 1; i >= 0; i-- {
		if k := strings.LastIndexByte(parts[i].text, '/'); parts[i].kind == partLit && k >= 0 {
			parts[i].text = parts[i].text[k+1:]
			start = i
			break
		}
	}
	base := shWord{parts: parts[start:]}
	text, ok := base.literal()
	base.raw = text
	return base, ok && text != ""
}

// intoFolder judges the files a copy of the paths a feeder hands it writes
// in the folder dir, written destRaw: the file of each path's name. belt
// cannot list those paths, so every entry of dir that may share a name
// with one is judged (pathFeed.mayName), and every entry when the command
// cuts what the feeder prints. A folder in a temp or cache folder passes
// unread.
func (j *discardJudge) intoFolder(dir, destRaw string, follows bool, fd *pathFeed, s walkStep) *fileLoss {
	if fd.why != "" {
		return &fileLoss{call: copyLoss("", s), why: fd.why}
	}
	if real := physical(dir); j.inTemp(real+"/x") || j.inCache(real) {
		return nil
	}
	entries, err := os.ReadDir(dir)
	switch {
	case err != nil:
		return &fileLoss{call: copyLoss("", s), why: fmt.Sprintf("belt cannot read `%s` (%v)", dir, err)}
	case len(entries) > j.g.maxPaths:
		return &fileLoss{call: copyLoss("", s), why: fmt.Sprintf("belt cannot judge every file the copy "+
			"may overwrite in `%s`: it holds more than %d entries", dir, j.g.maxPaths)}
	}
	cut := fd.split != "" && !j.printsWhole(fd, s)
	for _, e := range entries {
		named := fd.mayName(e.Name())
		if !named && !cut {
			continue
		}
		call := &lossCall{replace: true, toss: "move it to the trash first with `" + TossBin +
			" --safe-mode " + destRaw + "/" + e.Name() + "`, then run the command again"}
		loss := j.drop(call, false, nil, kernelPath(filepath.Join(dir, e.Name()), follows))
		if loss != nil && !named {
			loss.why = "the paths the feeder prints are cut at blanks and quotes before the copy reads " +
				"them, so a piece of one may be named `" + e.Name() + "`, and " + loss.why
			loss.instead = "To hand each path over whole, print the paths with NUL bytes and read them " +
				"with `xargs -0` (`find … -print0`, `git ls-files -z`)."
		}
		if loss != nil {
			return loss
		}
	}
	return nil
}

// printsWhole reports whether every path a feed may print reaches the
// command whole: no root as written, and no name below a root that is a
// folder, holds a byte the command cuts at (pathFeed.split). .git is read
// too unless the feeder never hands a path in it. A root belt cannot place,
// a folder it cannot read and a walk past its cap count as cut.
func (j *discardJudge) printsWhole(fd *pathFeed, s walkStep) bool {
	for _, r := range fd.roots {
		lit, active, ok := j.scan.staticGlobs(r.word)
		if !ok || slices.Contains(active, true) || strings.ContainsAny(lit, fd.split) {
			return false
		}
		if !path.IsAbs(lit) {
			if s.cwd == "" || !s.cwdSure {
				return false
			}
			lit = strings.TrimSuffix(s.cwd, "/") + "/" + lit
		}
		if !isDir(lit) {
			continue
		}
		cut := false
		err := j.walkDirs(lit, walkMode{skipGit: r.filter.skipsGitData()}, func(_ string, entries []fs.DirEntry) walkNext {
			cut = slices.ContainsFunc(entries, func(e fs.DirEntry) bool { return strings.ContainsAny(e.Name(), fd.split) })
			if cut {
				return walkStop
			}
			return walkOn
		})
		if err != nil || cut {
			return false
		}
	}
	return true
}

// mayName reports whether a path the feed hands may have the name: a root
// whose -name tests all match it, or that has none. A command that cuts
// what the feeder prints (pathFeed.split) may get other names too
// (intoFolder).
func (fd *pathFeed) mayName(name string) bool {
	return slices.ContainsFunc(fd.roots, func(r feedRoot) bool {
		return r.filter == nil || !slices.ContainsFunc(r.filter.names, func(n findName) bool {
			return !n.match(name)
		})
	})
}

// fedCopy judges a copy whose destination is a consumer's input: each
// input is a file written over (pathFeed through discardJudge.fed), and an
// input with a literal suffix (`{}.bak`) names each input's sibling of
// that name (fedSuffix). A -t folder from the inputs and any other word
// that holds a mark name a path belt cannot tell.
func (j *discardJudge) fedCopy(c copyCall, dest shWord, whole bool, s walkStep, in *copyInput) *fileLoss {
	call := copyLoss("", s)
	switch {
	case c.target != nil:
		return unresolvable(call, dest)
	case whole:
		return j.fed(call, lossOperand{follows: c.follows()}, in.fd, s)
	}
	v, lit := dest.literal()
	suffix := strings.TrimPrefix(v, in.fd.mark)
	if !lit || len(suffix) == len(v) || suffix == "" || strings.Contains(suffix, "/") ||
		in.holds(literalWord(suffix)) {
		return unresolvable(call, dest)
	}
	return j.fedSuffix(call, in.fd, suffix, c.follows(), s)
}

// fedSuffix judges the paths a copy writes by adding suffix to each path a
// feeder hands it (`cp {} {}.bak`): each root with the suffix added, and
// below a root that is a folder each path whose name ends in suffix and
// that the feeder's -name tests match with the suffix added to them, and
// the file each such path that is a symlink leads to, for a copy that
// writes through one (fedLinks). The feeder's -type tests and tracked-only
// filter say nothing about those paths.
func (j *discardJudge) fedSuffix(call *lossCall, fd *pathFeed, suffix string, follows bool, s walkStep) *fileLoss {
	if fd.why != "" {
		return &fileLoss{call: call, why: fd.why}
	}
	pattern := globEscape(suffix, nil)
	for _, r := range fd.roots {
		own := shWord{raw: r.word.raw + suffix, parts: append(slices.Clone(r.word.parts),
			wordPart{kind: partLit, text: suffix, quoted: true})}
		if loss := j.operand(call, lossOperand{word: own, follows: follows || r.follows}, s); loss != nil {
			return loss
		}
		if p, _, placed := j.placeDest(r.word, s); !r.tree || placed && !isDir(p) {
			continue
		}
		f := pathFilter{names: []findName{{pattern: "*" + pattern}}, split: fd.split}
		if r.filter != nil {
			for _, n := range r.filter.names {
				f.names = append(f.names, findName{pattern: n.pattern + pattern, fold: n.fold})
			}
			f.skipsGit, f.skipsIgnored = r.filter.skipsGitData(), r.filter.skipsIgnored
		}
		op := lossOperand{word: r.word, tree: true, follows: r.follows, filter: &f}
		if loss := j.operand(call, op, s); loss != nil {
			return loss
		}
		if follows {
			if loss := j.fedLinks(call, feedRoot{word: r.word, tree: true, filter: &f}, s); loss != nil {
				return loss
			}
		}
	}
	return nil
}

// placeDest resolves a copy's destination word to a path the way
// pathOperand reads an operand: a literal, ~ or $HOME path, a $PWD the
// walk knows, relative to a sure shell directory. temp marks a word a
// mktemp variable starts that stays inside what mktemp made. ok is false
// for any other word, and for a glob.
func (j *discardJudge) placeDest(w shWord, s walkStep) (p string, temp, ok bool) {
	if parts := w.parts; len(parts) > 0 && parts[0].kind == partVar && parts[0].text != "HOME" {
		if parts[0].text != "PWD" {
			_, known := j.scan.vars[parts[0].text]
			_, inside := j.scan.target(w)
			return "", known && inside, false
		}
		if s.pwd == "" || !parts[0].quoted && strings.ContainsAny(s.pwd, " \t\n*?[") {
			return "", false, false
		}
		w = shWord{raw: w.raw, parts: append([]wordPart{{kind: partLit, text: s.pwd, quoted: true}}, parts[1:]...)}
	}
	lit, active, ok := j.scan.staticGlobs(w)
	if !ok || slices.Contains(active, true) {
		return "", false, false
	}
	if !path.IsAbs(lit) {
		if s.cwd == "" || !s.cwdSure {
			return "", false, false
		}
		lit = strings.TrimSuffix(s.cwd, "/") + "/" + lit
	}
	return lit, false, true
}

// copyLoss is the call for a path a copy writes over, as the command
// spells it, for the toss-bin form: move the file to the trash, then run
// the command as written. With no path the form names none.
func copyLoss(written string, s walkStep) *lossCall {
	if written == "" {
		return &lossCall{replace: true, toss: "move each file it overwrites to the trash first with `" +
			TossBin + " --safe-mode <file>`"}
	}
	return replaceCall(written, writtenCommand(s))
}
