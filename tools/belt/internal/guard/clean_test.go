package guard

import (
	"reflect"
	"testing"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// TestRunCleanReportsSoftPasses: a built-in guard in soft mode that would
// have denied lets the call through with a warning, which RunClean reports
// as a pass that is not clean, so no allow acts on it. A call no guard
// objects to is clean, and a denial is never clean.
func TestRunCleanReportsSoftPasses(t *testing.T) {
	script := writeScript(t, t.TempDir(), "x.sh", "kubectl delete pod x\n")
	hard := scriptDenyConfig([]string{"kubectl delete"}, nil)
	soft := scriptDenyConfig([]string{"kubectl delete"}, nil)
	toggle := soft.Guards[ScriptDenyListID]
	toggle.Mode = config.ModeSoft
	soft.Guards[ScriptDenyListID] = toggle
	tests := []struct {
		name, command string
		cfg           config.Config
		wantDeny      bool
		wantClean     bool
	}{
		{"nothing to deny", "git -C . status", soft, false, true},
		{"soft mode would have denied", "bash " + script, soft, false, false},
		{"hard mode denies", "bash " + script, hard, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, clean := RunClean(Input{Event: EventBash, Command: tt.command}, tt.cfg)
			if (d != nil) != tt.wantDeny || clean != tt.wantClean {
				t.Errorf("RunClean(%q) = %+v, %v; want a denial: %v, clean: %v",
					tt.command, d, clean, tt.wantDeny, tt.wantClean)
			}
		})
	}
}

// TestEveryEmittingGuardIsAWarner: a guard emits an event only to let a
// call through with a warning, so every guard type with an emit field must
// report those warnings to RunClean. A new guard that adds one without
// noteWarnings would let an allow approve a call it only warned about.
func TestEveryEmittingGuardIsAWarner(t *testing.T) {
	cfg := config.Config{CustomGuards: map[string]config.CustomGuard{
		"external": {Event: EventBash, Command: []string{"true"}},
	}}
	for _, g := range All(cfg) {
		v := reflect.Indirect(reflect.ValueOf(g))
		if _, emits := v.Type().FieldByName("emit"); !emits {
			continue
		}
		if _, ok := g.(warner); !ok {
			t.Errorf("%T holds an emit func but has no noteWarnings method", g)
		}
	}
}
