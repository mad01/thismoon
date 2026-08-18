package store

import (
	"time"

	"github.com/mad01/thismoon/services/keeper-of-facts/internal/pin"
)

// Assertion is a single derived claim about a codebase or machine, backed by at
// least one evidence pin. Times are stored in UTC.
type Assertion struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Subject     string     `json:"subject"`
	Statement   string     `json:"statement"`
	Pins        []pin.Pin  `json:"pins"`
	Confidence  string     `json:"confidence"`
	Provenance  Provenance `json:"provenance"`
	Status      string     `json:"status"`
	StaleReason string     `json:"stale_reason,omitempty"`
	RetractNote string     `json:"retract_note,omitempty"`
	CheckedAt   *time.Time `json:"checked_at,omitempty"`
	Links       []string   `json:"links,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	RetractedAt *time.Time `json:"retracted_at,omitempty"`
}

// Provenance records where an assertion came from: who asserted it, the
// session that derived it, when, and its token cost. Author is stamped by the
// serve process, never taken from the request.
type Provenance struct {
	Author     string    `json:"author,omitempty"`
	SessionID  string    `json:"session_id"`
	DerivedAt  time.Time `json:"derived_at"`
	CostTokens int       `json:"cost_tokens,omitempty"`
}

// Assertion kind values: what sort of claim the statement makes.
const (
	KindCodeBehavior = "code-behavior" // how the code behaves
	KindDeadEnd      = "dead-end"      // an approach that was tried and failed
	KindPreference   = "preference"    // a stated way of working
	KindDecision     = "decision"      // a settled choice
	KindMachineState = "machine-state" // a fact about the local machine
	KindOpenThread   = "open-thread"   // unfinished work worth resuming
)

// Confidence values: how strongly the assertion is believed.
const (
	ConfidenceVerified = "verified" // checked against a primary source
	ConfidenceDerived  = "derived"  // inferred from evidence
	ConfidenceHint     = "hint"     // a weak signal
)

// Assertion status values.
const (
	StatusFresh     = "fresh"     // evidence last checked and still holding
	StatusStale     = "stale"     // at least one pin no longer holds
	StatusRetracted = "retracted" // withdrawn by the user (terminal)
)

// ValidKind reports whether k is a known assertion kind.
func ValidKind(k string) bool {
	switch k {
	case KindCodeBehavior, KindDeadEnd, KindPreference,
		KindDecision, KindMachineState, KindOpenThread:
		return true
	}
	return false
}

// ValidConfidence reports whether c is a known confidence level.
func ValidConfidence(c string) bool {
	switch c {
	case ConfidenceVerified, ConfidenceDerived, ConfidenceHint:
		return true
	}
	return false
}

// ValidStatus reports whether s is a known assertion status.
func ValidStatus(s string) bool {
	switch s {
	case StatusFresh, StatusStale, StatusRetracted:
		return true
	}
	return false
}
