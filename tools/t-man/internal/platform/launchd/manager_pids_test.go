package launchd

import (
	"context"
	"os/exec"
	"testing"
)

func TestRunningPIDs(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		mockErr error
		want    map[string]int
		wantErr bool
	}{
		{
			name: "running and stopped services",
			output: "PID\tStatus\tLabel\n" +
				"1234\t0\tsvc-a\n" +
				"-\t0\tsvc-b\n" +
				"5678\t0\tsvc-c\n",
			want: map[string]int{"svc-a": 1234, "svc-c": 5678},
		},
		{
			name:   "header only",
			output: "PID\tStatus\tLabel\n",
			want:   map[string]int{},
		},
		{
			name:    "launchctl failure",
			output:  "",
			mockErr: exec.ErrNotFound,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manager{
				launchctl: &LaunchctlClient{
					execCommand: mockExecCommand(tt.output, tt.mockErr),
				},
			}

			got, err := m.RunningPIDs(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("RunningPIDs() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("RunningPIDs() = %v, want %v", got, tt.want)
			}
			for label, pid := range tt.want {
				if got[label] != pid {
					t.Errorf("RunningPIDs()[%q] = %d, want %d", label, got[label], pid)
				}
			}
		})
	}
}
