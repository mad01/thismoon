package settings

import (
	"fmt"
	"io"

	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
)

// The report reproduces sync_settings.py's output byte for byte, and that
// output carries an em dash and an arrow. They are spelled as code points so
// the source stays free of the characters themselves.
const (
	emDash = string(rune(0x2014))
	arrow  = string(rune(0x2192))
)

// Report lines that are fixed text.
const (
	inSyncLine     = "settings already in sync"
	planFooter     = "\n(dry-run " + emDash + " no changes written)"
	breakLinkLine  = "breaking symlink " + emDash + " file will be a regular file after merge"
	localOnlyTitle = "local-only keys preserved (not in template " + emDash +
		" add to template to persist):"
)

// driftSummary is check's first line: the one `ralph doctor` shows.
func driftSummary(n int) string {
	keys, verb := "keys", "differ"
	if n == 1 {
		keys, verb = "key", "differs"
	}
	return fmt.Sprintf("drift: %d %s %s from template (run 'ralph up' to merge)", n, keys, verb)
}

// fmtVal renders a value in the report: objects and arrays as
// json.dumps(v, indent=2), scalars as json.dumps(v).
func fmtVal(v any) string {
	switch v.(type) {
	case *pyjson.Object, []any:
		return pyjson.Indent(v, 2)
	default:
		return pyjson.Compact(v)
	}
}

// writeSections prints the diff under its headings: overwritten keys, added
// keys, pruned keys, removed keys, then the local-only keys the merge kept.
// A heading with nothing under it is left out.
func writeSections(w io.Writer, diffs []diff, prunePaths []KeyPath, kept []localOnly) {
	var overwritten, added, pruned, removed []diff
	for _, d := range diffs {
		switch {
		case d.kind == diffChanged:
			overwritten = append(overwritten, d)
		case d.kind == diffAdded:
			added = append(added, d)
		case isUnder(d.path, prunePaths):
			pruned = append(pruned, d)
		default:
			removed = append(removed, d)
		}
	}
	if len(overwritten) > 0 {
		fmt.Fprintln(w, "overwritten (template wins):")
		for _, d := range overwritten {
			writeOverwritten(w, d)
		}
	}
	if len(added) > 0 {
		fmt.Fprintln(w, "added from template:")
		for _, d := range added {
			fmt.Fprintf(w, "  + %s: %s\n", d.path, fmtVal(d.after))
		}
	}
	if len(pruned) > 0 {
		fmt.Fprintln(w, "pruned (retired keys):")
		for _, d := range pruned {
			fmt.Fprintf(w, "  - %s: %s\n", d.path, fmtVal(d.before))
		}
	}
	if len(removed) > 0 {
		fmt.Fprintln(w, "removed (was in live, not in merged):")
		for _, d := range removed {
			fmt.Fprintf(w, "  - %s: %s\n", d.path, fmtVal(d.before))
		}
	}
	if len(kept) > 0 {
		fmt.Fprintln(w, localOnlyTitle)
		for _, k := range kept {
			fmt.Fprintf(w, "  ~ %s: %s\n", k.path, fmtVal(k.val))
		}
	}
}

// writeOverwritten prints one changed key: an array pair as the entries it
// lost and gained, anything else as the old and new value.
func writeOverwritten(w io.Writer, d diff) {
	before, beforeIsList := d.before.([]any)
	after, afterIsList := d.after.([]any)
	if !beforeIsList || !afterIsList {
		fmt.Fprintf(w, "  %s\n", d.path)
		fmt.Fprintf(w, "    - %s\n", fmtVal(d.before))
		fmt.Fprintf(w, "    + %s\n", fmtVal(d.after))
		return
	}
	fmt.Fprintf(w, "  %s (array)\n", d.path)
	lost, gained := listMinus(before, after), listMinus(after, before)
	for _, x := range lost {
		fmt.Fprintf(w, "    - %s\n", pyjson.Compact(x))
	}
	for _, x := range gained {
		fmt.Fprintf(w, "    + %s\n", pyjson.Compact(x))
	}
	if len(lost) == 0 && len(gained) == 0 {
		fmt.Fprintln(w, "    (order changed)")
	}
}

// listMinus returns a's entries that b lacks, by Python ==.
func listMinus(a, b []any) []any {
	var out []any
	for _, x := range a {
		if !pyjson.Contains(b, x) {
			out = append(out, x)
		}
	}
	return out
}
