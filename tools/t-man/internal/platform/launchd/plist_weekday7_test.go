package launchd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// weekdaySevenPlist is a t-man plist edited by hand to use launchd's other
// spelling of Sunday, with the entries out of order.
const weekdaySevenPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>sunday-job</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/bin/true</string>
	</array>
	<key>RunAtLoad</key>
	<false/>
	<key>KeepAlive</key>
	<false/>
	<key>StartCalendarInterval</key>
	<array>
		<dict>
			<key>Weekday</key>
			<integer>7</integer>
			<key>Hour</key>
			<integer>9</integer>
			<key>Minute</key>
			<integer>0</integer>
		</dict>
		<dict>
			<key>Weekday</key>
			<integer>6</integer>
			<key>Hour</key>
			<integer>9</integer>
			<key>Minute</key>
			<integer>0</integer>
		</dict>
	</array>
	<key>TManMetadata</key>
	<dict>
		<key>Hash</key>
		<string>deadbeef</string>
		<key>ManagedBy</key>
		<string>t-man</string>
		<key>Version</key>
		<string>test</string>
	</dict>
</dict>
</plist>
`

func TestManagerGet_WeekdaySevenReadsAsSunday(t *testing.T) {
	mgr := newTestManager(t)
	path := filepath.Join(mgr.plistDir, "sunday-job.plist")
	if err := os.WriteFile(path, []byte(weekdaySevenPlist), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	def, err := mgr.Get(context.Background(), "sunday-job")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(def.Calendar) != 2 {
		t.Fatalf("Calendar = %+v, want two entries", def.Calendar)
	}
	if wd := def.Calendar[0].Weekday; wd == nil || *wd != 0 {
		t.Errorf("first entry Weekday = %v, want Sunday as 0 sorted first", wd)
	}
	if got := def.ScheduleString(); got != "sat-sun 09:00" {
		t.Errorf("ScheduleString() = %q, want sat-sun 09:00", got)
	}
	if strings.Contains(def.ScheduleString(), "%!") {
		t.Errorf("ScheduleString() = %q leaks a formatting error", def.ScheduleString())
	}
}
