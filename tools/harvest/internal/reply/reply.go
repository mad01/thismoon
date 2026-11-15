// Package reply turns the model's reply into the captures harvest writes.
// The reply is untrusted: the model read a session's text, and that text
// can say anything. So Parse checks every field and keeps only what brain
// can take as a value, never as a flag or a path.
//
// The contract, which the skill file gives the model, is one JSON object:
//
//	{"captures": [{"title": "...", "body": "...", "tags": ["..."], "source": "https://..."}],
//	 "dropped": [{"candidate": "...", "reason": "..."}]}
package reply

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// The limits Parse applies.
const (
	// MaxCaptures is the cap on captures from one session, as the skill
	// file gives it. The rest are dropped.
	MaxCaptures = 5
	// MaxTopicTags is how many of the model's tags a capture keeps.
	MaxTopicTags = 3
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

// Drop is a candidate that is not written, with the reason.
type Drop struct {
	Candidate string
	Reason    string
}

// Plan is what a reply asks for once Parse has checked it.
type Plan struct {
	Captures []Capture
	Dropped  []Drop // the model's own drops, then the ones Parse made
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

// Parse reads the reply. It takes the text from the first { through the
// last }, so the object may stand alone, sit in a code fence, or have prose
// around it. A reply with no object, or one that does not decode, is an
// error. A capture with no title or no body is dropped, and so is every
// capture past MaxCaptures.
func Parse(text string) (Plan, error) {
	start, end := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if start < 0 || end < start {
		return Plan{}, ErrNoObject
	}
	var w wire
	if err := json.Unmarshal([]byte(text[start:end+1]), &w); err != nil {
		return Plan{}, fmt.Errorf("reply: decode: %w", err)
	}

	var p Plan
	for _, d := range w.Dropped {
		p.Dropped = append(
			p.Dropped,
			Drop{Candidate: oneLine(d.Candidate), Reason: oneLine(d.Reason)},
		)
	}
	for _, c := range w.Captures {
		title := oneLine(c.Title)
		switch {
		case title == "":
			p.Dropped = append(p.Dropped, Drop{Candidate: "(untitled)", Reason: "no title"})
		case strings.TrimSpace(c.Body) == "":
			p.Dropped = append(p.Dropped, Drop{Candidate: title, Reason: "no body"})
		case len(p.Captures) == MaxCaptures:
			p.Dropped = append(p.Dropped, Drop{
				Candidate: title, Reason: fmt.Sprintf("over the cap of %d", MaxCaptures),
			})
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
