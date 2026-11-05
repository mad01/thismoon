// Package lifecycle is the note status model: the statuses a note moves
// through, the frontmatter fields each one needs, and the defaults a write
// fills in. Every lifecycle field but status lives among the frontmatter
// keys brain does not type (markdown.Frontmatter.Extra), so a note written
// before the model existed parses as it always did. The package is pure: it
// reads and changes a markdown.Frontmatter and never touches disk.
package lifecycle

import (
	"slices"
	"strings"
	"time"
)

// Statuses a note may store.
const (
	// Raw is an inbox capture, unverified.
	Raw = "raw"
	// Validated is first-class knowledge, due for review on review_by.
	Validated = "validated"
	// Promoting is a note on its way into a shared vault.
	Promoting = "promoting"
	// Disputed is a note whose claims someone has challenged.
	Disputed = "disputed"
	// Superseded is a note another note replaces; superseded_by names it.
	Superseded = "superseded"
	// Retracted is a note found wrong; the body stays, read shows a banner.
	Retracted = "retracted"
	// Withdrawn is a tombstone: only id, status and status_at remain.
	Withdrawn = "withdrawn"
)

// Expired is the status a validated note past its review_by reads as. It is
// computed, never stored.
const Expired = "expired"

// Statuses lists every status a note may store, in lifecycle order.
var Statuses = []string{Raw, Validated, Promoting, Disputed, Superseded, Retracted, Withdrawn}

// Retraction reasons, the values retracted_reason takes.
const (
	ReasonError           = "error"
	ReasonSourceRetracted = "source-retracted"
	ReasonUnverifiable    = "unverifiable"
)

// RetractReasons lists the values retracted_reason takes.
var RetractReasons = []string{ReasonError, ReasonSourceRetracted, ReasonUnverifiable}

// Volatility values: how fast a note's subject changes.
const (
	VolatilityHigh = "high"
	VolatilityLow  = "low"
)

// Volatilities lists the values volatility takes.
var Volatilities = []string{VolatilityHigh, VolatilityLow}

// Classifications lists the values classification takes, least sensitive
// first. A vault policy narrows which of them the vault accepts.
var Classifications = []string{"public", "internal", "confidential", "restricted"}

// The windows the defaults count from the day of the write.
const (
	// RawLifetimeDays is how long a raw capture lives before it expires.
	RawLifetimeDays = 30
	// HighVolatilityReviewDays is the review window of a fast-moving note.
	HighVolatilityReviewDays = 90
	// LowVolatilityReviewDays is the review window of a slow-moving note,
	// and of a note that names no volatility.
	LowVolatilityReviewDays = 365
)

// Frontmatter keys of the lifecycle fields.
const (
	KeyStatus          = "status"
	KeyID              = "id"
	KeyExpiresAt       = "expires_at"
	KeyReviewBy        = "review_by"
	KeyStatusAt        = "status_at"
	KeyStatusReason    = "status_reason"
	KeySupersededBy    = "superseded_by"
	KeyRetractedReason = "retracted_reason"
	KeyRetractedAt     = "retracted_at"
	KeyVolatility      = "volatility"
	KeyCapturedBy      = "captured_by"
	KeyDistinctFrom    = "distinct_from"
	KeySteward         = "steward"
	KeySources         = "sources"
	KeyValidFrom       = "valid_from"
	KeyInvalidAt       = "invalid_at"
	KeyClassification  = "classification"
)

// DateLayout is the layout lifecycle dates are written in.
const DateLayout = "2006-01-02"

// Normalize returns status the way the model compares it: trimmed and
// lowercased.
func Normalize(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}

// Known reports whether status is one a note may store.
func Known(status string) bool {
	return slices.Contains(Statuses, Normalize(status))
}

// Revoked reports whether status takes a note out of default search:
// superseded, retracted or withdrawn.
func Revoked(status string) bool {
	switch Normalize(status) {
	case Superseded, Retracted, Withdrawn:
		return true
	}
	return false
}

// Date formats t as a lifecycle date, in UTC.
func Date(t time.Time) string { return t.UTC().Format(DateLayout) }

// ParseDate reads a lifecycle date: YYYY-MM-DD, or an RFC 3339 timestamp.
func ParseDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if t, err := time.Parse(DateLayout, s); err == nil {
		return t, true
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, true
	}
	return time.Time{}, false
}

// PastReview reports whether the note's review_by date is before now's day.
// A note without a readable review_by is never past review.
func PastReview(f Fields, now time.Time) bool {
	return pastDate(f.ReviewBy, now)
}

// ReviewDays returns the review window of a volatility; an unknown or empty
// volatility gets the low window.
func ReviewDays(volatility string) int {
	if Normalize(volatility) == VolatilityHigh {
		return HighVolatilityReviewDays
	}
	return LowVolatilityReviewDays
}

// Standing describes how a note stands as a citation target: "" when it is
// validated and not past review, otherwise the reason a citation of it
// deserves a second look ("raw", "retracted", "expired", ...). A validated
// note past review_by and a raw note past expires_at are expired. status is
// the note's effective status, its tier default when the field is empty.
func Standing(status string, f Fields, now time.Time) string {
	switch s := Normalize(status); {
	case s == Validated && PastReview(f, now), s == Raw && pastDate(f.ExpiresAt, now):
		return Expired
	case s == Validated:
		return ""
	default:
		return s
	}
}

// pastDate reports whether the date value lies before now's day.
func pastDate(value string, now time.Time) bool {
	t, ok := ParseDate(value)
	return ok && Date(t) < Date(now)
}
