package trace

import (
	"slices"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// TestUsageReprices: stored usage priced again with the same table costs
// what the calls cost one by one. The long-prompt tier, fast mode and
// US-only inference each keep their own entry, so no call moves to
// another rate when it is summed.
func TestUsageReprices(t *testing.T) {
	calls := []Call{
		{Model: "claude-haiku-5-5", Tokens: Tokens{Input: 50_000, Output: 1000}},
		{Model: "claude-haiku-5-5", Tokens: Tokens{Input: 1000, CacheRead: 199_000, Output: 1000}},
		{Model: "claude-haiku-5-5", Tokens: Tokens{Input: 20_000}},
		{Model: "claude-opus-5-5", Speed: "fast", Tokens: Tokens{Input: 1_000_000}},
		{Model: "claude-opus-5-5", Speed: "fast", Geo: "us", Tokens: Tokens{Output: 1000}},
		{Model: "claude-opus-5-5", Speed: "standard", Tokens: Tokens{Output: 1000}},
		{Model: "claude-sonnet-5-5", Geo: "us", Tokens: Tokens{CacheRead: 1_000_000}},
		{Model: "claude-sonnet-5-5", Speed: "fast", Tokens: Tokens{Input: 1}},
		{Model: "claude-nova-9", Tokens: Tokens{Input: 1}},
		{Model: "claude-nova-9"},
	}
	var p Prices
	usage := p.Usage(calls)

	want := 0.0
	var unpriced []string
	for _, c := range calls {
		cost := p.Cost(c)
		if cost == nil {
			if !slices.Contains(unpriced, c.Model) {
				unpriced = append(unpriced, c.Model)
			}
			continue
		}
		want += *cost
	}
	slices.Sort(unpriced)
	got, gotUnpriced := p.UsageCost(usage)
	if got == nil || !near(*got, want) || !slices.Equal(gotUnpriced, unpriced) {
		t.Errorf("UsageCost = %v, %q; want %v, %q", deref(got), gotUnpriced, want, unpriced)
	}

	type rate struct {
		model, speed, geo string
		long              bool
	}
	var rates []rate
	for _, u := range usage {
		rates = append(rates, rate{u.Model, u.Speed, u.Geo, u.Long})
	}
	wantRates := []rate{
		{"claude-haiku-5-5", "", "", false},
		{"claude-haiku-5-5", "", "", true},
		{"claude-nova-9", "", "", false},
		{"claude-opus-5-5", "", "", false},
		{"claude-opus-5-5", "fast", "", false},
		{"claude-opus-5-5", "fast", "us", false},
		{"claude-sonnet-5-5", "", "us", false},
		{"claude-sonnet-5-5", "fast", "", false},
	}
	if !slices.Equal(rates, wantRates) {
		t.Errorf("usage rates = %v\nwant %v", rates, wantRates)
	}
	if u := usage[0]; u.Tokens.Input != 70_000 || u.Tokens.Output != 1000 {
		t.Errorf("haiku under the threshold = %+v, want its two calls summed", u.Tokens)
	}
}

// TestUsageCostEmpty: usage without a billed token has no cost, never a
// zero one, and a long entry of a model whose row lost its tier bills its
// standard rates.
func TestUsageCostEmpty(t *testing.T) {
	var p Prices
	if cost, unpriced := p.UsageCost(nil); cost != nil || unpriced != nil {
		t.Errorf("UsageCost(nil) = %v, %q; want nil, nil", deref(cost), unpriced)
	}
	long := []store.ModelUsage{{
		Model: "claude-opus-5-5", Long: true, Tokens: Tokens{Input: 1_000_000},
	}}
	if cost, _ := p.UsageCost(long); cost == nil || !near(*cost, 4.0) {
		t.Errorf("long opus entry = %v, want the standard 4.0", deref(cost))
	}
}

// TestBuildSessionMergesUsage: a session's totals carry its turns' usage
// merged by rate.
func TestBuildSessionMergesUsage(t *testing.T) {
	a := TurnTrace{Seq: 1, Totals: Totals{Usage: []store.ModelUsage{
		{Model: "m", Tokens: Tokens{Input: 1}},
	}}}
	b := TurnTrace{Seq: 2, Totals: Totals{Usage: []store.ModelUsage{
		{Model: "m", Tokens: Tokens{Input: 2}}, {Model: "k", Tokens: Tokens{Output: 3}},
	}}}
	got := BuildSession("s", []TurnTrace{a, b}).Totals.Usage
	want := []store.ModelUsage{
		{Model: "k", Tokens: Tokens{Output: 3}}, {Model: "m", Tokens: Tokens{Input: 3}},
	}
	if !slices.Equal(got, want) {
		t.Errorf("session usage = %+v, want %+v", got, want)
	}
}
