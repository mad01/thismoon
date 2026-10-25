package trace

import (
	"maps"
	"regexp"
	"strings"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// tokensPerMillion is the unit prices are quoted in.
const tokensPerMillion = 1_000_000

// speedFast is the usage speed of a call made in fast mode, speedStandard
// the speed of every other call.
const (
	speedFast     = "fast"
	speedStandard = "standard"
)

// geoUS is the usage inference_geo of a call made with US-only inference.
const geoUS = "us"

// Call is what one model call billed, with the parts of its usage that set
// its price.
type Call struct {
	Model  string
	Tokens Tokens
	// Speed is the usage's speed: "fast" for fast mode.
	Speed string
	// Geo is the usage's inference_geo: "us" for US-only inference.
	Geo string
}

// Prices answers what a model call cost. The zero value prices with
// moondb.DefaultPrices.
type Prices struct {
	byModel map[string]moondb.Price
}

// NewPrices returns the default table with overrides laid over it. An
// override sets the rates of its model id and keeps the rest of the
// default row that id matches by Lookup: the long-prompt tier and the
// fast-mode and US-only multipliers, so new rates for a model never drop
// what else its row knows. An id no default row matches is added with its
// rates alone.
func NewPrices(overrides map[string]moondb.Rates) Prices {
	table := maps.Clone(moondb.DefaultPrices)
	for id, r := range overrides {
		price, _ := Prices{}.Lookup(id)
		price.Rates = r
		table[id] = price
	}
	return Prices{byModel: table}
}

func (p Prices) table() map[string]moondb.Price {
	if p.byModel == nil {
		return moondb.DefaultPrices
	}
	return p.byModel
}

// Lookup finds a model id's price: the exact id first, then the longest id
// in the table that is the model's family, so a dated
// "claude-opus-4-5-20251101" takes the "claude-opus-4-5" row. A provider
// prefix before "claude-" (us.anthropic.) is dropped first. A new version
// ("claude-opus-5-6") is not in "claude-opus-5"'s family: versions differ
// in price, so it has none until the table or the config names it.
func (p Prices) Lookup(model string) (moondb.Price, bool) {
	if i := strings.Index(model, "claude-"); i > 0 {
		model = model[i:]
	}
	table := p.table()
	if price, ok := table[model]; ok {
		return price, true
	}
	best := ""
	for id := range table {
		if len(id) > len(best) && familyOf(model, id) {
			best = id
		}
	}
	if best == "" {
		return moondb.Price{}, false
	}
	return table[best], true
}

// familySuffix is what may follow a family's id: a dated snapshot
// ("-20251101"), a provider version ("@20251101") or a context marker
// ("[1m]").
var familySuffix = regexp.MustCompile(`^(-\d{8}|@|\[)`)

// familyOf reports whether model is id with a family suffix.
func familyOf(model, id string) bool {
	rest, ok := strings.CutPrefix(model, id)
	return ok && familySuffix.MatchString(rest)
}

// Cost returns what call c cost in USD. It is nil when the model has no
// price, or ran in fast mode without a fast-mode price: an unknown cost is
// never reported as zero. A call that billed no tokens costs zero whatever
// its model.
func (p Prices) Cost(c Call) *float64 {
	return p.cost(c, p.longPrompt(c))
}

// longPrompt reports whether c's prompt is over its model's long-prompt
// threshold, so it bills at the model's long rates.
func (p Prices) longPrompt(c Call) bool {
	price, ok := p.Lookup(c.Model)
	return ok && price.LongPromptTokens > 0 && c.Tokens.Prompt() > price.LongPromptTokens
}

// Usage groups calls by the rate they bill at, the shape a Turn stores: one
// entry per model, speed, region and long-prompt tier, sorted. A call that
// billed no tokens adds nothing, and a standard speed is left out.
func (p Prices) Usage(calls []Call) []store.ModelUsage {
	entries := make([]store.ModelUsage, 0, len(calls))
	for _, c := range calls {
		if c.Tokens.Total() == 0 {
			continue
		}
		speed := c.Speed
		if speed == speedStandard {
			speed = ""
		}
		entries = append(entries, store.ModelUsage{
			Model: c.Model, Speed: speed, Geo: c.Geo, Long: p.longPrompt(c), Tokens: c.Tokens,
		})
	}
	return store.MergeUsage(entries)
}

// UsageCost prices stored usage with this table: the USD of the entries
// with a price, nil when none has one, and the models of the entries
// without one, sorted. An entry that billed no tokens counts for neither.
func (p Prices) UsageCost(usage []store.ModelUsage) (cost *float64, unpriced []string) {
	var t Totals
	for _, u := range usage {
		if u.Tokens.Total() == 0 {
			continue
		}
		c := Call{Model: u.Model, Tokens: u.Tokens, Speed: u.Speed, Geo: u.Geo}
		t.addCost(p.cost(c, u.Long), u.Model)
	}
	return t.Cost, t.Unpriced
}

// cost prices c at the model's long rates when long is set and the model
// has a long-prompt tier, at its standard rates otherwise.
func (p Prices) cost(c Call, long bool) *float64 {
	t := c.Tokens
	if t.Total() == 0 {
		return new(0.0)
	}
	price, ok := p.Lookup(c.Model)
	if !ok {
		return nil
	}
	scale, ok := multiplier(price, c)
	if !ok {
		return nil
	}
	rates := price.Rates
	if long && price.LongPromptTokens > 0 {
		rates = price.Long
	}
	usd := (float64(t.Input)*rates.Input +
		float64(t.CacheWrite5m)*rates.CacheWrite5m +
		float64(t.CacheWrite1h)*rates.CacheWrite1h +
		float64(t.CacheRead)*rates.CacheRead +
		float64(t.Output)*rates.Output) / tokensPerMillion * scale
	return &usd
}

// multiplier is what scales every rate of c: fast mode, then US-only
// inference on top. ok is false for fast mode on a model without a
// fast-mode price.
func multiplier(price moondb.Price, c Call) (scale float64, ok bool) {
	scale = 1.0
	if c.Speed == speedFast {
		if price.FastMultiplier == 0 {
			return 0, false
		}
		scale = price.FastMultiplier
	}
	if c.Geo == geoUS && price.USMultiplier > 0 {
		scale *= price.USMultiplier
	}
	return scale, true
}
