// Package reply turns the model's reply into the captures harvest writes.
// The reply is untrusted: the model read a session's text, and that text
// can say anything. So Parse checks every field and keeps only what brain
// can take as a value, never as a flag or a path.
//
// The contract, which the skill file gives the model, is one JSON object:
//
//	{"captures": [{"title": "...", "body": "...", "tags": ["..."], "source": "https://..."}],
//	 "dropped": [{"candidate": "...", "reason": "..."}]}
//
// The candidates the model dropped are never kept. The skill tells it to
// drop a secret, so a candidate can hold one, and the plan ends up in the
// log.
package reply

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

// The limits Parse applies.
const (
	// MaxCaptures is the cap on captures from one session, as the skill
	// file gives it. The rest are dropped.
	MaxCaptures = 5
	// MaxTopicTags is how many of the model's tags a capture keeps.
	MaxTopicTags = 3
	// MaxTitleBytes is the longest title a capture may have; a capture
	// with a longer one is dropped. A title is a short claim, and brain
	// names the note's file after it.
	MaxTitleBytes = 200
	// MaxBodyBytes is the largest body a capture may have; a capture with
	// a larger one is dropped. A body is a claim and three short lines.
	MaxBodyBytes = 16 << 10
	// MaxReasons is how many of the model's drop reasons a plan keeps.
	MaxReasons = 20
	// MaxReasonBytes is where a drop reason is cut.
	MaxReasonBytes = 160
	// Tag is the tag every capture carries, the one the hint counts.
	Tag = "harvest"
)

// ErrNoObject means the reply holds no JSON object at all.
var ErrNoObject = errors.New("reply: no JSON object in the reply")

// Capture is one note to write.
type Capture struct {
	Title  string
	Body   string
	Tags   []string // lowercased topic tags, then Tag
	Source string   // an http or https URL, or ""
}

// Drop is a capture Parse will not write: its title, cut to MaxTitleBytes,
// and the reason.
type Drop struct {
	Title  string
	Reason string
}

// Declined is what the plan keeps of the candidates the model dropped: how
// many there were and the first MaxReasons reasons, each on one line and cut
// to MaxReasonBytes. The candidates themselves are not kept.
type Declined struct {
	Count   int
	Reasons []string
}

// Plan is what a reply asks for once Parse has checked it.
type Plan struct {
	Captures []Capture
	Dropped  []Drop   // the captures Parse dropped
	Declined Declined // the candidates the model dropped
}

// wire is the JSON shape of a reply.
type wire struct {
	Captures []struct {
		Title  string   `json:"title"`
		Body   string   `json:"body"`
		Tags   []string `json:"tags"`
		Source string   `json:"source"`
	} `json:"captures"`
	Dropped []struct {
		Candidate string `json:"candidate"`
		Reason    string `json:"reason"`
	} `json:"dropped"`
}

// Parse reads the reply. It decodes one JSON object from the first {, and
// ignores what follows it, so the object may stand alone, sit in a code
// fence, or have prose around it. A reply with no {, or an object that does
// not decode, is an error. A capture with no title or no body is dropped,
// and so is one over MaxTitleBytes or MaxBodyBytes and every capture past
// MaxCaptures.
func Parse(text string) (Plan, error) {
	start := strings.Index(text, "{")
	if start < 0 {
		return Plan{}, ErrNoObject
	}
	var w wire
	if err := json.NewDecoder(strings.NewReader(text[start:])).Decode(&w); err != nil {
		return Plan{}, fmt.Errorf("reply: decode: %w", err)
	}

	p := Plan{Declined: Declined{Count: len(w.Dropped)}}
	for _, d := range w.Dropped[:min(len(w.Dropped), MaxReasons)] {
		reason := cut(oneLine(d.Reason), MaxReasonBytes)
		if reason == "" {
			reason = "(no reason)"
		}
		p.Declined.Reasons = append(p.Declined.Reasons, reason)
	}
	for _, c := range w.Captures {
		title := oneLine(c.Title)
		drop := func(reason string) {
			p.Dropped = append(p.Dropped, Drop{Title: cut(title, MaxTitleBytes), Reason: reason})
		}
		switch {
		case title == "":
			p.Dropped = append(p.Dropped, Drop{Title: "(untitled)", Reason: "no title"})
		case len(title) > MaxTitleBytes:
			drop(fmt.Sprintf("title over %d bytes", MaxTitleBytes))
		case strings.TrimSpace(c.Body) == "":
			drop("no body")
		case len(c.Body) > MaxBodyBytes:
			drop(fmt.Sprintf("body over %d bytes", MaxBodyBytes))
		case len(p.Captures) == MaxCaptures:
			drop(fmt.Sprintf("over the cap of %d", MaxCaptures))
		default:
			p.Captures = append(p.Captures, Capture{
				Title:  title,
				Body:   c.Body,
				Tags:   tags(c.Tags),
				Source: source(c.Source),
			})
		}
	}
	return p, nil
}

// tags lowercases the model's tags and keeps the first MaxTopicTags that
// use only a-z, 0-9, / and -, then adds Tag. A tag with any other character
// is dropped whole rather than rewritten into another word.
func tags(raw []string) []string {
	var out []string
	seen := map[string]bool{Tag: true}
	for _, t := range raw {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] || !validTag(t) {
			continue
		}
		seen[t] = true
		out = append(out, t)
		if len(out) == MaxTopicTags {
			break
		}
	}
	return append(out, Tag)
}

func validTag(t string) bool {
	for _, r := range t {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '/' && r != '-' {
			return false
		}
	}
	return true
}

// source returns raw when it parses as an http or https URL with a host,
// and "" otherwise.
func source(raw string) string {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return raw
}

// oneLine collapses every run of whitespace, newlines included, into one
// space, so a value reads as one line in the log.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// cut returns s when it fits in n bytes, and otherwise its first n bytes or
// fewer, on a rune boundary, with "..." after them.
func cut(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "..."
}
