package store

import (
	"regexp"
	"slices"
	"strings"
)

// Session tags are short tokens a launcher gives the sessions it starts,
// such as "eval" for an eval run. A tag is lowercase letters, digits and
// dashes, starts with a letter or a digit, and is at most MaxSessionTagLen
// bytes long. A session keeps at most MaxSessionTags of them.
const (
	MaxSessionTags   = 8
	MaxSessionTagLen = 32
)

var tagPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// validTag reports whether tag is a session tag.
func validTag(tag string) bool {
	return len(tag) <= MaxSessionTagLen && tagPattern.MatchString(tag)
}

// ParseEntrypoint is raw when it is a valid entrypoint, else empty. An
// entrypoint is the harness's name for how it was started, such as cli or
// sdk-cli. It follows the session tag rule, since the sessions list shows
// it as a badge and the environment or a transcript can hold any value.
func ParseEntrypoint(raw string) string {
	if !validTag(raw) {
		return ""
	}
	return raw
}

// ParseTags reads a comma-separated tag list, as a launcher sets it in the
// environment. Each tag is trimmed of spaces; an invalid one is dropped. The
// rest are deduplicated, sorted and cut to the first MaxSessionTags. Nil
// when no tag is valid.
func ParseTags(list string) []string {
	return MergeTags(nil, strings.Split(list, ","))
}

// MergeTags is cur, the tags a session holds, with each valid tag of add it
// lacks, in sorted order, while it has fewer than MaxSessionTags. A tag the
// session holds is never dropped. The result is sorted, and nil when empty.
func MergeTags(cur, add []string) []string {
	add = slices.Clone(add)
	for i, t := range add {
		add[i] = strings.TrimSpace(t)
	}
	add = slices.DeleteFunc(add, func(t string) bool { return !validTag(t) })
	slices.Sort(add)
	out := slices.Clone(cur)
	for _, t := range add {
		if len(out) >= MaxSessionTags {
			break
		}
		if !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	slices.Sort(out)
	return out
}
