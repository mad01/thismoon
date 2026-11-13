package limits

import (
	"regexp"
	"slices"
	"strconv"
	"testing"
)

var wholeNumber = regexp.MustCompile(`\d+`)

// TestTableIsWellFormed holds every entry to the shape the reference and
// the lint rely on: a unique id, a known level and source, a scope, a rule,
// and a reason, a basis for the two sources that cite one, and a unit
// exactly when there is a number.
func TestTableIsWellFormed(t *testing.T) {
	seen := map[ID]bool{}
	for _, l := range All() {
		if seen[l.ID] {
			t.Errorf("%s: id repeats", l.ID)
		}
		seen[l.ID] = true
		if l.Level != Warn && l.Level != Refuse {
			t.Errorf("%s: unknown level %q", l.ID, l.Level)
		}
		if !slices.Contains([]Source{Study, Guideline, House, Renderer}, l.Source) {
			t.Errorf("%s: unknown source %q", l.ID, l.Source)
		}
		if l.Scope == "" || l.Rule == "" || l.Reason == "" {
			t.Errorf("%s: scope, rule, and reason are required", l.ID)
		}
		cites := l.Source == Study || l.Source == Guideline
		if cites != (l.Basis != "") {
			t.Errorf("%s: a study or guideline names its basis, and only those do", l.ID)
		}
		if (l.Number > 0) != (l.Unit != "") {
			t.Errorf("%s: a number needs a unit and a unit needs a number", l.ID)
		}
	}
}

// TestRuleLineStatesTheNumber pins the rule line to the entry's number, so
// the one line the reference prints cannot keep an old number after the
// number moves. A rule without a number holds no digits at all.
func TestRuleLineStatesTheNumber(t *testing.T) {
	for _, l := range All() {
		nums := wholeNumber.FindAllString(l.Rule, -1)
		if l.Number == 0 {
			if len(nums) > 0 {
				t.Errorf("%s: rule has no number but its line says %v", l.ID, nums)
			}
			continue
		}
		if !slices.Contains(nums, strconv.Itoa(l.Number)) {
			t.Errorf("%s: rule line %q does not state %d", l.ID, l.Rule, l.Number)
		}
	}
}

// TestWarningsComeFirst pins the table's order: the lint reports its rules
// in table order, so a refusal between two warnings would be a hole in it.
func TestWarningsComeFirst(t *testing.T) {
	all := All()
	warn := Warnings()
	if len(warn) == 0 {
		t.Fatal("no warnings in the table")
	}
	for i, l := range warn {
		if all[i].ID != l.ID {
			t.Fatalf("entry %d is %s, want the warning %s", i, all[i].ID, l.ID)
		}
	}
	for _, l := range all[len(warn):] {
		if l.Level != Refuse {
			t.Errorf("%s: a warning after the first refusal", l.ID)
		}
	}
}

func TestGetAndNumber(t *testing.T) {
	l, ok := Get(SummaryWords)
	if !ok || l.ID != SummaryWords || Number(SummaryWords) != l.Number {
		t.Fatalf("Get(%s) = %+v, %v", SummaryWords, l, ok)
	}
	if _, ok := Get("no.such"); ok {
		t.Error("Get of an unknown id reported a limit")
	}
	defer func() {
		if recover() == nil {
			t.Error("Number of an unknown id did not panic")
		}
	}()
	Number("no.such")
}
