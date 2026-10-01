package launchd

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// dictFormPlist is a scheduled job whose StartCalendarInterval uses the
// single-dict form launchd accepts (t-man itself always writes an array).
const dictFormPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/bin/true</string>
	</array>
	<key>RunAtLoad</key>
	<false/>
	<key>KeepAlive</key>
	<false/>
	<key>StartCalendarInterval</key>
	<dict>
		<key>Hour</key>
		<integer>7</integer>
		<key>Minute</key>
		<integer>30</integer>
	</dict>
%s</dict>
</plist>
`

const tmanMetadataBlock = `	<key>TManMetadata</key>
	<dict>
		<key>Hash</key>
		<string>deadbeef</string>
		<key>ManagedBy</key>
		<string>t-man</string>
		<key>Version</key>
		<string>test</string>
	</dict>
`

// keepAliveDictPlist is the shape a third-party agent may use: launchd
// allows KeepAlive to be a dict of conditions, which t-man's own struct
// cannot hold.
const keepAliveDictPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>com.example.vendor-agent</string>
	<key>ProgramArguments</key>
	<array>
		<string>/usr/bin/true</string>
	</array>
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
</dict>
</plist>
`

// dictForm fills the label and the metadata block of dictFormPlist; an
// empty metadata block makes a third-party plist.
func dictForm(label, metadata string) []byte {
	out := strings.Replace(dictFormPlist, "%s", label, 1)
	return []byte(strings.Replace(out, "%s", metadata, 1))
}

func TestParsePlist_DictFormCalendar(t *testing.T) {
	parsed, err := ParsePlist(dictForm("dict-job", tmanMetadataBlock))
	if err != nil {
		t.Fatalf("ParsePlist() error: %v", err)
	}
	if len(parsed.StartCalendarInterval) != 1 {
		t.Fatalf("StartCalendarInterval = %+v, want one entry", parsed.StartCalendarInterval)
	}
	entry := parsed.StartCalendarInterval[0]
	if entry.Hour == nil || *entry.Hour != 7 || entry.Minute == nil || *entry.Minute != 30 {
		t.Errorf("entry = %+v, want hour 7 minute 30", entry)
	}

	def := (&Manager{}).plistToDefinition(parsed)
	if !def.Scheduled() || def.ScheduleString() != "daily 07:30" {
		t.Errorf("plistToDefinition() schedule = %q, want daily 07:30", def.ScheduleString())
	}

	// Writing it back produces the array form, which launchd also accepts
	data, err := GeneratePlist(def, "test")
	if err != nil {
		t.Fatalf("GeneratePlist() error: %v", err)
	}
	if !strings.Contains(string(data), "<key>StartCalendarInterval</key>\n\t\t<array>") &&
		!strings.Contains(string(data), "<key>StartCalendarInterval</key><array>") {
		t.Errorf("regenerated plist does not use the array form:\n%s", data)
	}
}

func TestIsManagedByTMan_IgnoresForeignShapes(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{name: "t-man job in dict form", data: dictForm("dict-job", tmanMetadataBlock), want: true},
		{name: "third-party dict-form job", data: dictForm("com.example.cal", ""), want: false},
		{name: "third-party keep-alive dict", data: []byte(keepAliveDictPlist), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := IsManagedByTMan(tt.data)
			if err != nil {
				t.Fatalf("IsManagedByTMan() error = %v, want none", err)
			}
			if got != tt.want {
				t.Errorf("IsManagedByTMan() = %v, want %v", got, tt.want)
			}
		})
	}
	if _, err := IsManagedByTMan(nil); err == nil {
		t.Error("IsManagedByTMan(nil) returned no error")
	}
}

// TestManagerList_SkipsForeignPlistsQuietly puts third-party plists beside a
// managed job and expects List to return only the managed one, without a
// parse error on the foreign shapes.
func TestManagerList_SkipsForeignPlistsQuietly(t *testing.T) {
	mgr := newTestManager(t)
	for name, data := range map[string][]byte{
		"com.example.cal.plist":          dictForm("com.example.cal", ""),
		"com.example.vendor-agent.plist": []byte(keepAliveDictPlist),
		"dict-job.plist":                 dictForm("dict-job", tmanMetadataBlock),
	} {
		if err := os.WriteFile(filepath.Join(mgr.plistDir, name), data, 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	defs, err := mgr.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(defs) != 1 || defs[0].Name != "dict-job" {
		t.Fatalf("List() = %+v, want only dict-job", defs)
	}
	if defs[0].ScheduleString() != "daily 07:30" {
		t.Errorf("List() schedule = %q, want daily 07:30", defs[0].ScheduleString())
	}
}
