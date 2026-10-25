package trace

import "github.com/mad01/thismoon/services/moondb/internal/store"

// TotalsVersion is the trace build Stored stamps on a turn's totals. Raise
// it when a change to Build changes what a turn's totals hold: serve's
// totals backfill then computes every stored turn's totals again. Raise it
// too when a LongPromptTokens threshold in moondb.DefaultPrices changes: a
// usage entry's Long flag is fixed when the totals are computed, so stored
// totals keep the old side of the threshold until they are computed again.
const TotalsVersion = 1

// Stored is a turn's totals as the Turn keeps them: the time split, the
// usage and the call counts, without the cost, which a reader prices from
// the usage with the table it has (Prices.UsageCost).
func Stored(t Totals) store.TurnTotals {
	return store.TurnTotals{
		Version:    TotalsVersion,
		WallMS:     t.WallMS,
		ModelMS:    t.ModelMS,
		ToolMS:     t.ToolMS,
		HarnessMS:  t.HarnessMS,
		UserMS:     t.UserMS,
		Usage:      t.Usage,
		ModelCalls: t.ModelCalls,
		ToolCalls:  t.ToolCalls,
		ToolErrors: t.ToolErrors,
	}
}
