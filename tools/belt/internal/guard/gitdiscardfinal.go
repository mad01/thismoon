package guard

import (
	"slices"
	"strings"
	"time"
)

// What finalLoss says each form does, for finalLossReason.
const (
	configFinalLoss = "sets a gc expiry (gc.pruneExpire, gc.reflogExpire or " +
		"gc.reflogExpireUnreachable) sooner than git's default, so every later gc or " +
		"reflog expire in the repository deletes before git would"
	callConfigFinalLoss = "runs git with a gc expiry (gc.pruneExpire, gc.reflogExpire or " +
		"gc.reflogExpireUnreachable, from -c, --config-env or a GIT_CONFIG_* variable) " +
		"sooner than git's default, so a gc it runs or starts deletes before git would: " +
		"a commit, merge, rebase or fetch may start one"
	gcFinalLoss = "prunes objects no ref or reflog holds sooner than git's 2-week default. A " +
		"commit off a deleted or moved branch that no reflog holds is then gone for good, " +
		"and `git fsck` finds nothing"
	pruneFinalLoss = "deletes loose objects no ref or reflog holds sooner than git's 2-week " +
		"default: `git prune` deletes every one of them without an --expire date at least " +
		"that old. A commit off a deleted or moved branch that no reflog holds is then gone " +
		"for good, and `git fsck` finds nothing"
	reflogFinalLoss = "expires reflog entries sooner than git's default (90 days, 30 for " +
		"entries no branch holds). A commit off its branch after a `branch -D`, `branch -f` " +
		"or reset is then on no reflog, and the next prune deletes it for good"
	reflogDeleteFinalLoss = "deletes reflog entries. A commit off its branch after a " +
		"`branch -D`, `branch -f` or reset may be on no other reflog, and the next prune " +
		"deletes it for good"
	repackFinalLoss = "repacks everything and deletes the packed objects no ref or reflog " +
		"holds: `-a -d` without `-A`, `-k`, `--cruft` or an --unpack-unreachable date, or " +
		"with an --unpack-unreachable or --cruft-expiration date sooner than git's 2-week " +
		"default. A commit off a deleted or moved branch is then gone for good"
)

// finalLossReason is what the model reads when a command makes a loss final
// (finalLoss); what names what it does.
func finalLossReason(what string) string {
	return "this " + what + ". " + gitGoAhead +
		": keep what you need on a branch first, or let the user run it with `! <command>`."
}

// finalLoss names what a git call does that makes the loss of a commit off
// its branch final, "" for none: a reflog expire, gc, prune or repack that
// deletes sooner than git's default, a reflog delete or drop, and a config
// write that sets such an expiry up for later (configSetsFinal). Past its
// own words, a call that runs with such an expiry in its settings (a -c,
// --config-env or GIT_CONFIG_* one, finalSetting) counts unless it applies
// none (appliesNoExpiry): gc runs on its own after a commit, merge, rebase
// or fetch, and a hook or alias may run any git command. fed says a command
// in front of git may hand it words. x reads the dates.
func finalLoss(c gitInvocation, settings []callSetting, fed bool, x expiryReader) string {
	if what := ownFinalLoss(c, fed, x); what != "" {
		return what
	}
	if appliesNoExpiry(c, fed) || !slices.ContainsFunc(settings, x.finalSetting) {
		return ""
	}
	return callConfigFinalLoss
}

// ownFinalLoss names what a git call's own words make final, "" for none
// (finalLoss).
func ownFinalLoss(c gitInvocation, fed bool, x expiryReader) string {
	switch c.sub {
	case "config":
		if configSetsFinal(c.args, fed, x) {
			return configFinalLoss
		}
	case "gc":
		if gcPrunesSoon(c.args, fed, x) {
			return gcFinalLoss
		}
	case "prune":
		if prunesSoon(c.args, fed, x) {
			return pruneFinalLoss
		}
	case "reflog":
		return reflogLoss(c.args, fed, x)
	case "repack":
		if repackDrops(c.args, fed, x) {
			return repackFinalLoss
		}
	}
	return ""
}

// expiryFreeSubs are the subcommands that only read, so they apply no gc
// expiry from their settings: none starts a gc, a prune or a reflog expire,
// or runs a hook that may.
var expiryFreeSubs = wordSet(`
	status log show diff rev-parse rev-list ls-files ls-tree cat-file show-ref
	for-each-ref describe blame grep shortlog config help version var
	merge-base name-rev count-objects fsck check-ignore check-attr`)

// appliesNoExpiry reports whether a git call reads no gc expiry from its
// settings: a subcommand that only reads (expiryFreeSubs), a reflog call
// other than an expire, and a dry run of a prune or a reflog expire. Words
// belt cannot read may make it one that does, and so may words a command in
// front of git may hand over (fed).
func appliesNoExpiry(c gitInvocation, fed bool) bool {
	switch {
	case expiryFreeSubs[c.sub]:
		return true
	case fed:
		return false
	case c.sub == "prune":
		w, err := readGitWords(c.args, pruneOptions)
		return err == nil && w.on("dry-run")
	case c.sub == "reflog":
		return reflogReadsNoExpiry(c.args)
	}
	return false
}

// reflogReadsNoExpiry reports whether a reflog call applies no expiry: a
// show (bare, an option or a ref first), list, exists, write, delete or
// drop, which finalLoss judges on its own words, and an expire dry run. A
// first word the shell may still rewrite may be expire.
func reflogReadsNoExpiry(args []string) bool {
	if len(args) == 0 {
		return true
	}
	if unquoteWord(args[0]) == "expire" {
		w, err := readGitWords(args[1:], reflogExpireOptions)
		return err == nil && w.on("dry-run")
	}
	return strings.HasPrefix(args[0], "-") || !mayExpand(args[0])
}

// expiryWindow returns git's default window for a lower-cased gc expiry
// key: gc.pruneExpire, which gc passes to prune, and gc.reflogExpire and
// gc.reflogExpireUnreachable, also in the gc.<pattern>.<key> form that
// picks refs. ok is false for any other key.
func expiryWindow(key string) (keep time.Duration, ok bool) {
	if key == "gc.pruneexpire" {
		return pruneKeep, true
	}
	if !strings.HasPrefix(key, "gc.") {
		return 0, false
	}
	switch key[strings.LastIndex(key, ".")+1:] {
	case "reflogexpire":
		return reflogKeep, true
	case "reflogexpireunreachable":
		return unreachableKeep, true
	}
	return 0, false
}

// configSetsFinal reports whether a git config call writes a gc expiry key
// (expiryWindow) with a value sooner than its default (finalWord), in any
// scope (readConfigSet). A key the shell may still rewrite may be any of
// them, so its value is held to the longest window. Words a command in
// front of git may hand over (fed) may make any call such a write, unless
// its own words make it a read.
func configSetsFinal(args []string, fed bool, x expiryReader) bool {
	if fed {
		return gitConfigWrites(slices.Concat(args, []string{"key", "value"}))
	}
	s, ok := readConfigSet(args)
	if !ok || s.value < 0 {
		return false
	}
	key := args[s.key]
	keep, ok := expiryWindow(strings.ToLower(unquoteWord(key)))
	if mayExpand(key) {
		keep, ok = reflogKeep, true
	}
	return ok && x.finalWord(args[s.value], keep)
}

// gcPrunesSoon reports whether a gc prunes sooner than git's default: a
// --prune date sooner than pruneKeep (finalWord), the last of --prune and
// --no-prune deciding, read as git reads its options (readGitWords). A bare
// --prune takes gc.pruneExpire, which finalLoss reads from the settings.
// Words belt cannot read count, and so do words a command in front of git
// may hand over (fed).
func gcPrunesSoon(args []string, fed bool, x expiryReader) bool {
	w, err := readGitWords(args, gcOptions)
	return err != nil || fed || setsSoon(w, "prune", pruneKeep, x)
}

// prunesSoon reports whether a prune deletes loose objects sooner than git's
// default. Without --expire, git prune expires every unreachable loose
// object whatever its age, so a plain one counts; --no-expire reads as
// never, and a date at least pruneKeep ago keeps git's grace period. A dry
// run (-n) deletes nothing. Words belt cannot read count, and so do words a
// command in front of git may hand over (fed).
func prunesSoon(args []string, fed bool, x expiryReader) bool {
	w, err := readGitWords(args, pruneOptions)
	switch {
	case err != nil || fed:
		return true
	case w.on("dry-run"):
		return false
	}
	o, ok := w.last("expire")
	return !ok || (!o.unset && x.finalWord(o.value, pruneKeep))
}

// reflogLoss names what a reflog call deletes, "" for nothing: an expire
// sooner than git's default (reflogExpiresSoon), and a delete or drop of
// any log, unless it is a dry run (reflogDeletes). The stash forms deny
// before this as stash drops (reflogDrops).
func reflogLoss(args []string, fed bool, x expiryReader) string {
	if len(args) == 0 {
		return ""
	}
	switch unquoteWord(args[0]) {
	case "expire":
		if reflogExpiresSoon(args[1:], fed, x) {
			return reflogFinalLoss
		}
	case "delete", "drop":
		if reflogDeletes(args, fed) {
			return reflogDeleteFinalLoss
		}
	}
	return ""
}

// reflogExpiresSoon reports whether the words after `reflog expire` expire
// entries sooner than git's default: --expire sooner than reflogKeep or
// --expire-unreachable sooner than unreachableKeep, read as git reads its
// options (readGitWords), unless it is a dry run. Without either, the
// expiry comes from the settings or config. Words belt cannot read count,
// and so do words a command in front of git may hand over (fed).
func reflogExpiresSoon(args []string, fed bool, x expiryReader) bool {
	w, err := readGitWords(args, reflogExpireOptions)
	switch {
	case err != nil || fed:
		return true
	case w.on("dry-run"):
		return false
	}
	return setsSoon(w, "expire", reflogKeep, x) ||
		setsSoon(w, "expire-unreachable", unreachableKeep, x)
}

// reflogDeletes reports whether a `reflog delete` or `reflog drop` call
// deletes entries: a delete unless it is a dry run (-n), and every drop,
// which has none. Words belt cannot read count, and so do words a command
// in front of git may hand over (fed).
func reflogDeletes(args []string, fed bool) bool {
	table := reflogDropOptions
	if unquoteWord(args[0]) == "delete" {
		table = reflogDeleteOptions
	}
	w, err := readGitWords(args[1:], table)
	return err != nil || fed || !w.on("dry-run")
}

// repackDrops reports whether a repack deletes packed objects no ref or
// reflog holds: -d with -a, -A or --cruft, which pack everything into one
// pack and delete the old ones. -k keeps those objects in the new pack, and
// --cruft packs them apart unless --cruft-expiration is sooner than
// pruneKeep. -A turns them loose, and so does an --unpack-unreachable date
// with -a or -A, which git hands pack-objects, for the ones newer than it:
// the date must not be sooner than pruneKeep. Words belt cannot read count,
// and so do words a command in front of git may hand over (fed).
func repackDrops(args []string, fed bool, x expiryReader) bool {
	w, err := readGitWords(args, repackOptions)
	switch {
	case err != nil || fed:
		return true
	case !w.on("d") || !packsEverything(w):
		return false
	case w.on("keep-unreachable"):
		return false
	case w.on("cruft"):
		return setsSoon(w, "cruft-expiration", pruneKeep, x)
	}
	if o, ok := w.last("unpack-unreachable"); ok && !o.unset {
		return x.finalWord(o.value, pruneKeep)
	}
	return !w.on("A")
}

// packsEverything reports whether a repack packs every object into one
// pack: -a, -A or --cruft.
func packsEverything(w gitWords) bool {
	return w.on("a") || w.on("A") || w.on("cruft")
}

// setsSoon reports whether the last read of a value option sets a date
// sooner than keep (finalWord). A bare option with no value takes git's
// default.
func setsSoon(w gitWords, name string, keep time.Duration, x expiryReader) bool {
	o, ok := w.last(name)
	return ok && !o.unset && o.value != "" && x.finalWord(o.value, keep)
}
