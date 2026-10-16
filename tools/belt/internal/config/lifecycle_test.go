package config

import (
	"strings"
	"testing"
	"time"
)

func TestLifecycleDefaults(t *testing.T) {
	cfg := mustLoad(t, Paths{BeltYAML: writeFile(t, t.TempDir(), "config.yaml", "hints: {}\n")})
	if !cfg.Ledger.On() || cfg.Ledger.SourceName() != DefaultLedgerSource {
		t.Errorf("ledger defaults = on %v, source %q", cfg.Ledger.On(), cfg.Ledger.SourceName())
	}
	w := cfg.Watch
	if w.Enabled {
		t.Error("watch must be off until a config turns it on")
	}
	if w.PollInterval() != DefaultWatchPollInterval || w.MaxWait() != DefaultWatchMaxWait ||
		w.SourceName() != DefaultWatchSource {
		t.Errorf("watch defaults = %s, %s, %q", w.PollInterval(), w.MaxWait(), w.SourceName())
	}
}

func TestLoadLifecycleSections(t *testing.T) {
	cfg := mustLoad(t, Paths{BeltYAML: writeFile(t, t.TempDir(), "config.yaml", `
ledger:
  enabled: false
  source: factory-ledger
watch:
  enabled: true
  poll_interval_ms: 500
  max_wait_s: 120
  source: factory-inbox
`)})
	if cfg.Ledger.On() || cfg.Ledger.SourceName() != "factory-ledger" {
		t.Errorf("ledger = on %v, source %q", cfg.Ledger.On(), cfg.Ledger.SourceName())
	}
	w := cfg.Watch
	if !w.Enabled || w.PollInterval() != 500*time.Millisecond ||
		w.MaxWait() != 120*time.Second || w.SourceName() != "factory-inbox" {
		t.Errorf("watch = %+v (%s, %s, %q)", w, w.PollInterval(), w.MaxWait(), w.SourceName())
	}
}

// TestLifecycleValuesAreClampedNotRejected pins the rule that keeps a typo
// in an optional feature from denying every tool call: each value loads
// without error, resolves to a usable value, and is named as an adjustment
// for doctor to warn about.
func TestLifecycleValuesAreClampedNotRejected(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		check      func(Config) bool
		adjustment string
	}{
		{
			"poll interval below the floor is clamped to it",
			"watch:\n  enabled: false\n  poll_interval_ms: 50\n",
			func(c Config) bool { return c.Watch.PollInterval() == MinWatchPollInterval },
			"watch.poll_interval_ms is 50, using the 100ms floor",
		},
		{
			"negative poll interval uses the default",
			"watch:\n  poll_interval_ms: -1\n",
			func(c Config) bool { return c.Watch.PollInterval() == DefaultWatchPollInterval },
			"watch.poll_interval_ms is -1, using the default 2s",
		},
		{
			"max wait past the cap is clamped to it",
			"watch:\n  max_wait_s: 86400\n",
			func(c Config) bool { return c.Watch.MaxWait() == MaxWatchMaxWait },
			"watch.max_wait_s is 86400, using the 1h0m0s cap",
		},
		{
			// 1e10 seconds overflows the conversion to time.Duration and
			// wraps negative; the cap check must run on the raw count.
			"absurd max wait is clamped, not wrapped",
			"watch:\n  max_wait_s: 10000000000\n",
			func(c Config) bool { return c.Watch.MaxWait() == MaxWatchMaxWait },
			"watch.max_wait_s is 10000000000, using the 1h0m0s cap",
		},
		{
			// 1e13 milliseconds overflows the conversion the same way.
			"absurd poll interval is clamped to the cap, not wrapped",
			"watch:\n  poll_interval_ms: 10000000000000\n",
			func(c Config) bool { return c.Watch.PollInterval() == MaxWatchPollInterval },
			"watch.poll_interval_ms is 10000000000000, using the 1h0m0s cap",
		},
		{
			"negative max wait uses the default",
			"watch:\n  max_wait_s: -5\n",
			func(c Config) bool { return c.Watch.MaxWait() == DefaultWatchMaxWait },
			"watch.max_wait_s is -5, using the default 9m0s",
		},
		{
			"ledger source is normalized the way the events service stores it",
			"ledger:\n  enabled: false\n  source: Belt_Ledger\n",
			func(c Config) bool { return c.Ledger.SourceName() == "belt_ledger" },
			`ledger.source is "Belt_Ledger", using "belt_ledger"`,
		},
		{
			"watch source with nothing usable falls back to the default",
			"watch:\n  source: '///'\n",
			func(c Config) bool { return c.Watch.SourceName() == DefaultWatchSource },
			`watch.source is "///", using "belt-inbox"`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Paths{BeltYAML: writeFile(t, t.TempDir(), "config.yaml", tt.content)}
			cfg, err := LoadFrom(p)
			if err != nil {
				t.Fatalf(
					"LoadFrom() error = %v, want none: lifecycle values never fail the file",
					err,
				)
			}
			if !tt.check(cfg) {
				t.Errorf("effective values = ledger %+v / watch %+v", cfg.Ledger, cfg.Watch)
			}
			got := strings.Join(append(cfg.Ledger.Adjustments(), cfg.Watch.Adjustments()...), "\n")
			if !strings.Contains(got, tt.adjustment) {
				t.Errorf("adjustments = %q, want one containing %q", got, tt.adjustment)
			}
		})
	}
}

// TestLifecycleTypeErrorsStillFailTheFile: clamping covers values, not
// shapes. A value of the wrong YAML type fails the file as it does in every
// other section.
func TestLifecycleTypeErrorsStillFailTheFile(t *testing.T) {
	p := Paths{BeltYAML: writeFile(t, t.TempDir(), "config.yaml", "watch:\n  max_wait_s: soon\n")}
	if _, err := LoadFrom(p); err == nil {
		t.Error("LoadFrom() = nil error for a string max_wait_s, want a parse error")
	}
}

func TestSanitizeSource(t *testing.T) {
	tests := []struct{ in, want string }{
		{"belt-ledger", "belt-ledger"},
		{"Belt Ledger", "belt-ledger"},
		{"  inbox//main  ", "inbox-main"},
		{"--edge--", "edge"},
		{"///", ""},
		{strings.Repeat("a", 70), strings.Repeat("a", maxSourceLen)},
	}
	for _, tt := range tests {
		if got := sanitizeSource(tt.in); got != tt.want {
			t.Errorf("sanitizeSource(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNoAdjustmentsForCleanValues(t *testing.T) {
	cfg := mustLoad(t, Paths{BeltYAML: writeFile(t, t.TempDir(), "config.yaml",
		"ledger:\n  source: belt-ledger\nwatch:\n  poll_interval_ms: 500\n  max_wait_s: 120\n")})
	if got := append(cfg.Ledger.Adjustments(), cfg.Watch.Adjustments()...); len(got) != 0 {
		t.Errorf("adjustments = %v, want none", got)
	}
}

func TestCustomHintAcceptsSubagentStart(t *testing.T) {
	cfg := mustLoad(t, Paths{BeltYAML: writeFile(t, t.TempDir(), "config.yaml", `
custom_hints:
  worker-brief:
    event: subagent-start
    command: [brief]
`)})
	if got := cfg.CustomHints["worker-brief"].Event; got != HintEventSubagentStart {
		t.Errorf("event = %q, want %q", got, HintEventSubagentStart)
	}
}
