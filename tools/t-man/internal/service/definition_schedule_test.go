package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefinitionHash_Schedule(t *testing.T) {
	base := Definition{Name: "job", Command: "/usr/bin/true"}
	baseHash, err := base.Hash()
	if err != nil {
		t.Fatalf("Hash() error: %v", err)
	}

	// nil and empty calendar must hash identically, or every service that
	// predates schedules would reconcile on its next add
	withEmpty := base
	withEmpty.Calendar = []CalendarEntry{}
	emptyHash, _ := withEmpty.Hash()
	if emptyHash != baseHash {
		t.Error("empty calendar hashes differently from nil")
	}

	withCalendar := base
	withCalendar.Calendar = []CalendarEntry{{Hour: ip(7), Minute: ip(30)}}
	calendarHash, _ := withCalendar.Hash()
	if calendarHash == baseHash {
		t.Error("adding a calendar did not change the hash")
	}

	movedCalendar := base
	movedCalendar.Calendar = []CalendarEntry{{Hour: ip(8), Minute: ip(30)}}
	movedHash, _ := movedCalendar.Hash()
	if movedHash == calendarHash {
		t.Error("changing the calendar hour did not change the hash")
	}

	withInterval := base
	withInterval.IntervalSeconds = 3600
	intervalHash, _ := withInterval.Hash()
	if intervalHash == baseHash || intervalHash == calendarHash {
		t.Error("adding an interval did not produce a distinct hash")
	}
}

// TestDefinitionJSON_ScheduleKeys pins the serialized key names, which are
// both the hash input and the vocabulary any future config reader must use.
func TestDefinitionJSON_ScheduleKeys(t *testing.T) {
	def := Definition{
		Name:            "job",
		Command:         "/usr/bin/true",
		Calendar:        []CalendarEntry{{Weekday: ip(1), Hour: ip(7), Minute: ip(30)}},
		IntervalSeconds: 0,
	}
	data, err := json.Marshal(def)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	want := `"calendar":[{"minute":30,"hour":7,"weekday":1}]`
	if got := string(data); !strings.Contains(got, want) {
		t.Errorf("JSON = %s, want it to contain %s", got, want)
	}
	if strings.Contains(string(data), "interval_seconds") {
		t.Errorf("JSON = %s, zero interval must be omitted", data)
	}

	var back Definition
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}
	if back.ScheduleString() != def.ScheduleString() {
		t.Errorf("round trip = %q, want %q", back.ScheduleString(), def.ScheduleString())
	}
}

func TestDefinitionLastLogWrite(t *testing.T) {
	dir := t.TempDir()
	stdout := filepath.Join(dir, "stdout.log")
	stderr := filepath.Join(dir, "stderr.log")

	def := Definition{StandardOutPath: stdout, StandardErrPath: stderr}
	if _, ok := def.LastLogWrite(); ok {
		t.Fatal("LastLogWrite() reported a time with no log files")
	}

	older := time.Date(2026, time.October, 1, 7, 30, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	for _, f := range []struct {
		path string
		at   time.Time
	}{{stdout, older}, {stderr, newer}} {
		if err := os.WriteFile(f.path, []byte("x"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		if err := os.Chtimes(f.path, f.at, f.at); err != nil {
			t.Fatalf("Chtimes: %v", err)
		}
	}

	got, ok := def.LastLogWrite()
	if !ok || !got.Equal(newer) {
		t.Errorf("LastLogWrite() = %v, %v; want %v", got, ok, newer)
	}

	onlyStdout := Definition{StandardOutPath: stdout, StandardErrPath: filepath.Join(dir, "no")}
	if got, ok := onlyStdout.LastLogWrite(); !ok || !got.Equal(older) {
		t.Errorf("LastLogWrite() with one file = %v, %v; want %v", got, ok, older)
	}
}

// TestDefinitionHash_SameScheduleWrittenThreeWays pins what NormalizeCalendar
// buys: a weekday set, a range, and five listed days produce one hash.
func TestDefinitionHash_SameScheduleWrittenThreeWays(t *testing.T) {
	ways := map[string]func() ([]CalendarEntry, error){
		"weekdays set":  func() ([]CalendarEntry, error) { return ParseClockSchedule("weekdays@07:30") },
		"weekday range": func() ([]CalendarEntry, error) { return ParseClockSchedule("mon-fri@07:30") },
		"listed days": func() ([]CalendarEntry, error) {
			return ParseClockSchedule("fri@07:30,thu@07:30,wed@07:30,tue@07:30,mon@07:30")
		},
		"calendar range": func() ([]CalendarEntry, error) { return ParseCalendarFields("weekday=1-5,hour=7,minute=30") },
	}
	hashes := map[string]string{}
	for name, parse := range ways {
		entries, err := parse()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		def := Definition{Name: "job", Command: "/usr/bin/true", Calendar: entries}
		h, err := def.Hash()
		if err != nil {
			t.Fatalf("%s: Hash() error: %v", name, err)
		}
		hashes[name] = h
	}
	for name, h := range hashes {
		if h != hashes["weekdays set"] {
			t.Errorf("%s hashes %s, weekdays set hashes %s", name, h, hashes["weekdays set"])
		}
	}
}
