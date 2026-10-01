package launchd

import "testing"

func TestParsePrintOutput_LastExitCode(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name: "numeric exit code",
			output: `gui/501/job = {
	state = waiting
	runs = 2
	last exit code = 1
}`,
			want: "1",
		},
		{
			name: "exit code with symbolic name",
			output: `gui/501/job = {
	state = spawn scheduled
	last exit code = 78: EX_CONFIG
}`,
			want: "78: EX_CONFIG",
		},
		{
			name: "never exited",
			output: `gui/501/job = {
	state = running
	pid = 14986
	last exit code = (never exited)
}`,
			want: "(never exited)",
		},
		{
			name: "nested block does not count",
			output: `gui/501/job = {
	state = waiting
	resource coalition = {
		last exit code = 9
	}
}`,
			want: "",
		},
		{
			name: "absent",
			output: `gui/501/job = {
	state = waiting
}`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePrintOutput("job", tt.output)
			if err != nil {
				t.Fatalf("parsePrintOutput() error = %v", err)
			}
			if got.LastExitCode != tt.want {
				t.Errorf("LastExitCode = %q, want %q", got.LastExitCode, tt.want)
			}
		})
	}
}

func TestStatusFromListEntry_LastExitCode(t *testing.T) {
	tests := []struct {
		name       string
		entry      ServiceStatus
		wantStatus string
		wantExit   string
	}{
		{
			name:       "running keeps the exit column",
			entry:      ServiceStatus{Label: "job", PID: "42", Status: "0"},
			wantStatus: "running",
			wantExit:   "0",
		},
		{
			name:       "clean exit reads stopped",
			entry:      ServiceStatus{Label: "job", PID: "-", Status: "0"},
			wantStatus: "stopped",
			wantExit:   "0",
		},
		{
			name:       "failed exit reads error and keeps the code",
			entry:      ServiceStatus{Label: "job", PID: "-", Status: "1"},
			wantStatus: "error",
			wantExit:   "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusFromListEntry(&tt.entry)
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", got.Status, tt.wantStatus)
			}
			if got.LastExitCode != tt.wantExit {
				t.Errorf("LastExitCode = %q, want %q", got.LastExitCode, tt.wantExit)
			}
		})
	}
}
