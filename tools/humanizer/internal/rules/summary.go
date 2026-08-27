package rules

// Summary aggregates detect findings the way the humanizer_detect MCP
// payload reports them: a total plus per-severity, per-category, and
// per-rule counts. The CLI --json output and both MCP detect tools share
// this shape so the two entrypoints stay interchangeable.
type Summary struct {
	Total      int            `json:"total"`
	BySeverity map[string]int `json:"by_severity"`
	ByCategory map[string]int `json:"by_category"`
	ByRule     map[string]int `json:"by_rule"`
}

// NewSummary returns a Summary with every counter map allocated.
func NewSummary() Summary {
	return Summary{
		BySeverity: map[string]int{},
		ByCategory: map[string]int{},
		ByRule:     map[string]int{},
	}
}

// Add folds one finding's severity, category, and rule into the summary.
// Category is only counted when the finding carries one.
func (s *Summary) Add(severity, category, ruleID string) {
	s.Total++
	s.BySeverity[severity]++
	if category != "" {
		s.ByCategory[category]++
	}
	s.ByRule[ruleID]++
}

// Summarize aggregates vale findings into a Summary.
func Summarize(findings []Finding) Summary {
	s := NewSummary()
	for _, f := range findings {
		s.Add(f.Severity, f.Category, f.RuleID)
	}
	return s
}
