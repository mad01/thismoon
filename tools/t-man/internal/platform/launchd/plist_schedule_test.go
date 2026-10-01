package launchd

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/t-man/internal/service"
)

// ip returns a pointer to n for building CalendarEntry literals.
func ip(n int) *int { return &n }

// roundTrip generates a plist for def, parses it back, and returns both the
// parsed plist and the definition rebuilt from it.
func roundTrip(t *testing.T, def *service.Definition) ([]byte, *LaunchdPlist, *service.Definition) {
	t.Helper()
	data, err := GeneratePlist(def, "test")
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}
	parsed, err := ParsePlist(data)
	if err != nil {
		t.Fatalf("ParsePlist() error: %v", err)
	}
	m := &Manager{}
	return data, parsed, m.plistToDefinition(parsed)
}

// assertSameHash pins the idempotency invariant: the round-tripped
// definition must hash identically, or every reconcile would see a phantom
// diff and bounce the job.
func assertSameHash(t *testing.T, want, got *service.Definition) {
	t.Helper()
	wantHash, err := want.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	gotHash, err := got.Hash()
	if err != nil {
		t.Fatalf("round-tripped Hash() error: %v", err)
	}
	if wantHash != gotHash {
		t.Errorf("round-trip hash mismatch: %s != %s\noriginal: %+v\nround-tripped: %+v",
			wantHash, gotHash, want, got)
	}
}

func TestGeneratePlist_CalendarSchedule(t *testing.T) {
	def := &service.Definition{
		Name:    "daily-digest",
		Command: "/usr/bin/true",
		Args:    []string{"--once"},
		Calendar: []service.CalendarEntry{
			{Hour: ip(7), Minute: ip(30)},
			{Weekday: ip(1), Hour: ip(9), Minute: ip(0)},
		},
	}

	data, parsed, got := roundTrip(t, def)

	want := []CalendarInterval{
		{Hour: ip(7), Minute: ip(30)},
		{Weekday: ip(1), Hour: ip(9), Minute: ip(0)},
	}
	if !reflect.DeepEqual(parsed.StartCalendarInterval, want) {
		t.Errorf("StartCalendarInterval = %+v, want %+v", parsed.StartCalendarInterval, want)
	}
	if parsed.StartInterval != 0 {
		t.Errorf("StartInterval = %d, want 0 for a calendar job", parsed.StartInterval)
	}
	if parsed.RunAtLoad || parsed.KeepAlive {
		t.Errorf("RunAtLoad=%v KeepAlive=%v, want both false", parsed.RunAtLoad, parsed.KeepAlive)
	}

	xml := string(data)
	for _, key := range []string{"<key>StartCalendarInterval</key>", "<key>Hour</key>", "<key>Weekday</key>"} {
		if !strings.Contains(xml, key) {
			t.Errorf("plist lacks %s:\n%s", key, xml)
		}
	}
	for _, key := range []string{"<key>Day</key>", "<key>Month</key>", "<key>StartInterval</key>"} {
		if strings.Contains(xml, key) {
			t.Errorf("plist has %s for a wildcard field:\n%s", key, xml)
		}
	}

	if !reflect.DeepEqual(got.Calendar, def.Calendar) {
		t.Errorf("round-tripped Calendar = %+v, want %+v", got.Calendar, def.Calendar)
	}
	assertSameHash(t, def, got)
}

func TestGeneratePlist_IntervalSchedule(t *testing.T) {
	def := &service.Definition{
		Name:            "hourly-audit",
		Command:         "/usr/bin/true",
		IntervalSeconds: 3600,
	}

	data, parsed, got := roundTrip(t, def)

	if parsed.StartInterval != 3600 {
		t.Errorf("StartInterval = %d, want 3600", parsed.StartInterval)
	}
	if len(parsed.StartCalendarInterval) != 0 {
		t.Errorf("StartCalendarInterval = %+v, want none", parsed.StartCalendarInterval)
	}
	if !strings.Contains(string(data), "<key>StartInterval</key>") {
		t.Errorf("plist lacks StartInterval:\n%s", data)
	}
	if got.IntervalSeconds != 3600 {
		t.Errorf("round-tripped IntervalSeconds = %d, want 3600", got.IntervalSeconds)
	}
	assertSameHash(t, def, got)
}

func TestGeneratePlist_NoScheduleKeysForService(t *testing.T) {
	def := &service.Definition{
		Name:      "plain-service",
		Command:   "/usr/bin/true",
		RunAtLoad: true,
		KeepAlive: true,
	}

	data, _, got := roundTrip(t, def)

	for _, key := range []string{"StartCalendarInterval", "StartInterval"} {
		if strings.Contains(string(data), key) {
			t.Errorf("unscheduled plist mentions %s:\n%s", key, data)
		}
	}
	if got.Scheduled() {
		t.Errorf("round-tripped definition reports Scheduled(): %+v", got)
	}
	assertSameHash(t, def, got)
}

func TestGeneratePlist_ScheduleWithKeepAliveRejected(t *testing.T) {
	def := &service.Definition{
		Name:      "bad-job",
		Command:   "/usr/bin/true",
		KeepAlive: true,
		Calendar:  []service.CalendarEntry{{Hour: ip(7), Minute: ip(30)}},
	}
	if _, err := GeneratePlist(def, "test"); err == nil {
		t.Fatal("GeneratePlist() accepted a scheduled job with KeepAlive")
	}
}

func TestGeneratePlist_ScheduleChangesHash(t *testing.T) {
	daily := &service.Definition{
		Name:     "job",
		Command:  "/usr/bin/true",
		Calendar: []service.CalendarEntry{{Hour: ip(7), Minute: ip(30)}},
	}
	_, parsed, _ := roundTrip(t, daily)

	moved := *daily
	moved.Calendar = []service.CalendarEntry{{Hour: ip(8), Minute: ip(30)}}
	movedHash, err := moved.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}
	if parsed.TManMetadata.Hash == movedHash {
		t.Error("moving the schedule by an hour left the stored hash unchanged")
	}
}
