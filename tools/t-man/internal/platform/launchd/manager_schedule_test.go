package launchd

import (
	"context"
	"testing"

	"github.com/mad01/thismoon/tools/t-man/internal/service"
)

func TestRunStateFor(t *testing.T) {
	job := &service.Definition{Calendar: []service.CalendarEntry{{Minute: ip(0)}}}
	svc := &service.Definition{KeepAlive: true}

	tests := []struct {
		name        string
		def         *service.Definition
		status      ServiceStatus
		wantStatus  string
		wantRunning bool
	}{
		{
			name:       "idle scheduled job from print reads scheduled",
			def:        job,
			status:     ServiceStatus{PID: "-", Status: "waiting", LastExitCode: "0"},
			wantStatus: "scheduled",
		},
		{
			name:       "idle scheduled job from list fallback reads scheduled",
			def:        job,
			status:     ServiceStatus{PID: "-", Status: "stopped"},
			wantStatus: "scheduled",
		},
		{
			name:       "failed last run still reads scheduled",
			def:        job,
			status:     ServiceStatus{PID: "-", Status: "error", LastExitCode: "1"},
			wantStatus: "scheduled",
		},
		{
			name:        "scheduled job mid-run reads running",
			def:         job,
			status:      ServiceStatus{PID: "4242", Status: "running"},
			wantStatus:  "running",
			wantRunning: true,
		},
		{
			name:       "long-lived service keeps launchd's stopped",
			def:        svc,
			status:     ServiceStatus{PID: "-", Status: "stopped"},
			wantStatus: "stopped",
		},
		{
			name:       "long-lived service keeps launchd's error",
			def:        svc,
			status:     ServiceStatus{PID: "-", Status: "error", LastExitCode: "78"},
			wantStatus: "error",
		},
		{
			name:       "long-lived service keeps spawn scheduled",
			def:        svc,
			status:     ServiceStatus{PID: "-", Status: "spawn scheduled"},
			wantStatus: "spawn scheduled",
		},
		{
			name:        "long-lived service running",
			def:         svc,
			status:      ServiceStatus{PID: "17", Status: "running"},
			wantStatus:  "running",
			wantRunning: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runStateFor(tt.def, &tt.status)
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", got.Status, tt.wantStatus)
			}
			if got.Running != tt.wantRunning {
				t.Errorf("Running = %v, want %v", got.Running, tt.wantRunning)
			}
			if got.LastExitCode != tt.status.LastExitCode {
				t.Errorf("LastExitCode = %q, want %q", got.LastExitCode, tt.status.LastExitCode)
			}
		})
	}
}

// TestManagerStatus_ScheduledJob drives Status through a plist on disk and a
// faked launchctl print, the path list and status take.
func TestManagerStatus_ScheduledJob(t *testing.T) {
	mgr := newTestManager(t)
	mgr.launchctl = &LaunchctlClient{execCommand: mockExecCommand(`gui/501/nightly = {
	state = waiting
	runs = 3
	last exit code = 0
}`, nil)}

	def := &service.Definition{
		Name:     "nightly",
		Command:  "/usr/bin/true",
		Calendar: []service.CalendarEntry{{Hour: ip(2), Minute: ip(0)}},
	}
	writeManagedPlist(t, mgr, def)

	status, err := mgr.Status(context.Background(), "nightly")
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status != "scheduled" {
		t.Errorf("Status() = %q, want scheduled", status)
	}

	state, err := mgr.RunState(context.Background(), def)
	if err != nil {
		t.Fatalf("RunState() error = %v", err)
	}
	if state.Running || state.LastExitCode != "0" {
		t.Errorf("RunState() = %+v, want idle with last exit code 0", state)
	}
}

func TestManagerGet_ScheduledJobRoundTrips(t *testing.T) {
	mgr := newTestManager(t)
	def := &service.Definition{
		Name:            "every-hour",
		Command:         "/usr/bin/true",
		IntervalSeconds: 3600,
	}
	writeManagedPlist(t, mgr, def)

	got, err := mgr.Get(context.Background(), "every-hour")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.IntervalSeconds != 3600 || !got.Scheduled() {
		t.Errorf("Get() = %+v, want an interval job of 3600s", got)
	}
	assertSameHash(t, def, got)
}
