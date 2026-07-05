package history

import (
	"path/filepath"
	"testing"
	"time"
)

func date(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, time.Local)
	if err != nil {
		panic(err)
	}
	return t
}

func TestRecordDaysUptime(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := date("2026-06-10 12:00")

	for range 99 {
		s.Record("svc", now, true)
	}
	s.Record("svc", now, false)
	s.Record("svc", now.AddDate(0, 0, -1), true)

	days := s.Days("svc", now, 3)
	if len(days) != 3 {
		t.Fatalf("got %d days, want 3", len(days))
	}
	if days[0].HasData {
		t.Errorf("oldest day should have no data: %+v", days[0])
	}
	if !days[1].HasData || days[1].Pct != 100 {
		t.Errorf("yesterday = %+v, want 100%%", days[1])
	}
	today := days[2]
	if !today.HasData || today.OK != 99 || today.Fail != 1 || today.Pct != 99 {
		t.Errorf("today = %+v, want 99 ok / 1 fail / 99%%", today)
	}

	pct, ok := s.Uptime("svc", now, 30)
	if !ok {
		t.Fatal("expected uptime data")
	}
	want := 100 * 100.0 / 101.0
	if pct < want-0.01 || pct > want+0.01 {
		t.Errorf("uptime = %f, want ~%f", pct, want)
	}

	if _, ok := s.Uptime("unknown", now, 30); ok {
		t.Error("unknown service must report no uptime data")
	}
}

func TestPrune(t *testing.T) {
	s, err := Load(filepath.Join(t.TempDir(), "history.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := date("2026-06-10 12:00")
	s.Record("old", now.AddDate(0, 0, -91), true)
	s.Record("svc", now.AddDate(0, 0, -91), true)
	s.Record("svc", now, true)

	s.Prune(now, 90)

	if _, ok := s.Uptime("old", now, 365); ok {
		t.Error("service with only stale buckets should be dropped")
	}
	days := s.Days("svc", now, 1)
	if !days[0].HasData {
		t.Error("recent bucket must survive pruning")
	}
	if got := s.Days("svc", now.AddDate(0, 0, -91), 1); got[0].HasData {
		t.Error("stale bucket must be pruned")
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	now := date("2026-06-10 12:00")
	s.Record("svc", now, true)
	s.Record("svc", now, false)
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	s2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	days := s2.Days("svc", now, 1)
	if days[0].OK != 1 || days[0].Fail != 1 {
		t.Errorf("roundtrip lost data: %+v", days[0])
	}
}
