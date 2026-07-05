package crashloop

import (
	"testing"
	"time"
)

func TestObserveFiresAtThreshold(t *testing.T) {
	tr := New(time.Hour, 50, 6*time.Hour)
	base := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	// 60 restarts over 30 minutes of one-minute samples: 2 per cycle.
	var fired *Alert
	for i := 0; i <= 30; i++ {
		a, ok := tr.Observe("present", base.Add(time.Duration(i)*time.Minute), 100+i*2)
		if ok {
			fired = &a
			break
		}
	}
	if fired == nil {
		t.Fatal("expected an alert once restarts crossed the threshold")
	}
	if fired.Restarts < 50 {
		t.Fatalf("alert restarts = %d, want >= 50", fired.Restarts)
	}
	if fired.Label != "present" {
		t.Fatalf("alert label = %q, want present", fired.Label)
	}
}

func TestObserveHealthyServiceNeverFires(t *testing.T) {
	tr := New(time.Hour, 50, 6*time.Hour)
	base := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	// A stable service: runs counter never moves.
	for i := 0; i < 120; i++ {
		if _, ok := tr.Observe("events", base.Add(time.Duration(i)*time.Minute), 3); ok {
			t.Fatal("healthy service must not alert")
		}
	}
}

func TestObserveCooldownSuppressesRepeatAlerts(t *testing.T) {
	tr := New(time.Hour, 10, 6*time.Hour)
	base := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	fires := 0
	// Continuous crash loop for 3 hours: only the first crossing alerts.
	for i := 0; i <= 180; i++ {
		if _, ok := tr.Observe("pr", base.Add(time.Duration(i)*time.Minute), i*2); ok {
			fires++
		}
	}
	if fires != 1 {
		t.Fatalf("fires = %d, want 1 (cooldown must suppress repeats)", fires)
	}
}

func TestObserveReAlertsAfterCooldown(t *testing.T) {
	tr := New(time.Hour, 10, 2*time.Hour)
	base := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	fires := 0
	for i := 0; i <= 300; i++ {
		if _, ok := tr.Observe("pr", base.Add(time.Duration(i)*time.Minute), i*2); ok {
			fires++
		}
	}
	if fires < 2 {
		t.Fatalf("fires = %d, want >= 2 (loop persisting past cooldown re-alerts)", fires)
	}
}

func TestObserveCounterResetDiscardsWindow(t *testing.T) {
	tr := New(time.Hour, 50, 6*time.Hour)
	base := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	// Counter climbs to 40 restarts, then the job is re-registered
	// (bootout/bootstrap) and the counter restarts near zero.
	for i := 0; i <= 20; i++ {
		tr.Observe("speak-web", base.Add(time.Duration(i)*time.Minute), 100+i*2)
	}
	a, ok := tr.Observe("speak-web", base.Add(21*time.Minute), 1)
	if ok {
		t.Fatalf("counter reset must not alert, got %+v", a)
	}
	// After the reset the baseline is fresh: small increases stay quiet.
	if _, ok := tr.Observe("speak-web", base.Add(22*time.Minute), 5); ok {
		t.Fatal("post-reset small delta must not alert")
	}
}

func TestForgetDropsRemovedServices(t *testing.T) {
	tr := New(time.Hour, 50, 6*time.Hour)
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	tr.Observe("gone", now, 10)
	tr.Observe("kept", now, 10)
	tr.Forget(map[string]bool{"kept": true})

	if _, ok := tr.samples["gone"]; ok {
		t.Fatal("Forget must drop services absent from discovery")
	}
	if _, ok := tr.samples["kept"]; !ok {
		t.Fatal("Forget must keep active services")
	}
}
