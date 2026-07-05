package launchd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// mockExecCommand creates a mock exec.Command function for testing
func mockExecCommand(
	output string,
	err error,
) func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		// Create a command that will return our mock output
		cmd := exec.Command("echo", output)
		if err != nil {
			// If we want to simulate an error, use a command that will fail
			cmd = exec.Command("sh", "-c", "exit 1")
		}
		return cmd
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name      string
		plistPath string
		mockErr   error
		wantErr   bool
	}{
		{
			name:      "successful load",
			plistPath: "/Library/LaunchDaemons/com.example.service.plist",
			mockErr:   nil,
			wantErr:   false,
		},
		{
			name:      "load with error",
			plistPath: "/Library/LaunchDaemons/com.example.bad.plist",
			mockErr:   exec.ErrNotFound,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &LaunchctlClient{
				execCommand: mockExecCommand("", tt.mockErr),
			}

			err := client.Load(context.Background(), tt.plistPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUnload(t *testing.T) {
	tests := []struct {
		name      string
		plistPath string
		mockErr   error
		wantErr   bool
	}{
		{
			name:      "successful unload",
			plistPath: "/Library/LaunchDaemons/com.example.service.plist",
			mockErr:   nil,
			wantErr:   false,
		},
		{
			name:      "unload with error",
			plistPath: "/Library/LaunchDaemons/com.example.bad.plist",
			mockErr:   exec.ErrNotFound,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &LaunchctlClient{
				execCommand: mockExecCommand("", tt.mockErr),
			}

			err := client.Unload(context.Background(), tt.plistPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("Unload() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStart(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		mockErr error
		wantErr bool
	}{
		{
			name:    "successful start",
			label:   "com.example.service",
			mockErr: nil,
			wantErr: false,
		},
		{
			name:    "start with error",
			label:   "com.example.bad",
			mockErr: exec.ErrNotFound,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &LaunchctlClient{
				execCommand: mockExecCommand("", tt.mockErr),
			}

			err := client.Start(context.Background(), tt.label)
			if (err != nil) != tt.wantErr {
				t.Errorf("Start() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestStop(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		mockErr error
		wantErr bool
	}{
		{
			name:    "successful stop",
			label:   "com.example.service",
			mockErr: nil,
			wantErr: false,
		},
		{
			name:    "stop with error",
			label:   "com.example.bad",
			mockErr: exec.ErrNotFound,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &LaunchctlClient{
				execCommand: mockExecCommand("", tt.mockErr),
			}

			err := client.Stop(context.Background(), tt.label)
			if (err != nil) != tt.wantErr {
				t.Errorf("Stop() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParseListOutput(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    []ServiceStatus
		wantErr bool
	}{
		{
			name: "parse multiple services",
			output: `PID	Status	Label
12345	0	com.example.service1
-	0	com.example.service2
67890	0	com.example.service3`,
			want: []ServiceStatus{
				{PID: "12345", Status: "0", Label: "com.example.service1", Loaded: true},
				{PID: "-", Status: "0", Label: "com.example.service2", Loaded: true},
				{PID: "67890", Status: "0", Label: "com.example.service3", Loaded: true},
			},
			wantErr: false,
		},
		{
			name:    "parse empty output",
			output:  "PID	Status	Label\n",
			want:    []ServiceStatus{},
			wantErr: false,
		},
		{
			name:    "parse header only",
			output:  "PID	Status	Label",
			want:    []ServiceStatus{},
			wantErr: false,
		},
		{
			name: "skip malformed lines",
			output: `PID	Status	Label
12345	0	com.example.service1
invalid
67890	0	com.example.service2`,
			want: []ServiceStatus{
				{PID: "12345", Status: "0", Label: "com.example.service1", Loaded: true},
				{PID: "67890", Status: "0", Label: "com.example.service2", Loaded: true},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseListOutput(tt.output)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseListOutput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("parseListOutput() got %d services, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i].PID != tt.want[i].PID ||
					got[i].Status != tt.want[i].Status ||
					got[i].Label != tt.want[i].Label ||
					got[i].Loaded != tt.want[i].Loaded {
					t.Errorf("parseListOutput() service[%d] = %+v, want %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestParsePrintOutput(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		output  string
		want    *ServiceStatus
		wantErr bool
	}{
		{
			name:  "parse running service",
			label: "com.example.service",
			output: `com.example.service = {
	state = running
	pid = 12345
	label = com.example.service
}`,
			want: &ServiceStatus{
				Label:  "com.example.service",
				PID:    "12345",
				Status: "running",
				Loaded: true,
			},
			wantErr: false,
		},
		{
			name:  "parse stopped service",
			label: "com.example.service",
			output: `com.example.service = {
	state = stopped
	label = com.example.service
}`,
			want: &ServiceStatus{
				Label:  "com.example.service",
				PID:    "-",
				Status: "stopped",
				Loaded: true,
			},
			wantErr: false,
		},
		{
			name:  "parse waiting service",
			label: "com.example.service",
			output: `com.example.service = {
	state = waiting
	label = com.example.service
}`,
			want: &ServiceStatus{
				Label:  "com.example.service",
				PID:    "-",
				Status: "waiting",
				Loaded: true,
			},
			wantErr: false,
		},
		{
			name:  "parse minimal output",
			label: "com.example.service",
			output: `com.example.service = {
	label = com.example.service
}`,
			want: &ServiceStatus{
				Label:  "com.example.service",
				PID:    "-",
				Status: "unknown",
				Loaded: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePrintOutput(tt.label, tt.output)
			if (err != nil) != tt.wantErr {
				t.Errorf("parsePrintOutput() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.Label != tt.want.Label ||
				got.PID != tt.want.PID ||
				got.Status != tt.want.Status ||
				got.Loaded != tt.want.Loaded {
				t.Errorf("parsePrintOutput() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestGetStatus(t *testing.T) {
	tests := []struct {
		name        string
		label       string
		mockOutput  string
		mockErr     error
		want        *ServiceStatus
		wantErr     bool
		errContains string
	}{
		{
			name:  "get status for running service",
			label: "com.example.service",
			mockOutput: `com.example.service = {
	state = running
	pid = 12345
}`,
			mockErr: nil,
			want: &ServiceStatus{
				Label:  "com.example.service",
				PID:    "12345",
				Status: "running",
				Loaded: true,
			},
			wantErr: false,
		},
		{
			name:        "service not found",
			label:       "com.example.notfound",
			mockOutput:  "",
			mockErr:     exec.ErrNotFound,
			want:        nil,
			wantErr:     true,
			errContains: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &LaunchctlClient{
				execCommand: mockExecCommand(tt.mockOutput, tt.mockErr),
			}

			got, err := client.GetStatus(context.Background(), tt.label)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetStatus() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf(
						"GetStatus() error = %v, want error containing %q",
						err,
						tt.errContains,
					)
				}
				return
			}
			if got.Label != tt.want.Label ||
				got.PID != tt.want.PID ||
				got.Status != tt.want.Status ||
				got.Loaded != tt.want.Loaded {
				t.Errorf("GetStatus() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestList(t *testing.T) {
	mockOutput := `PID	Status	Label
12345	0	com.example.service1
-	0	com.example.service2`

	client := &LaunchctlClient{
		execCommand: mockExecCommand(mockOutput, nil),
	}

	got, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(got) != 2 {
		t.Errorf("List() returned %d services, want 2", len(got))
	}

	if got[0].Label != "com.example.service1" {
		t.Errorf("List() first service label = %q, want %q", got[0].Label, "com.example.service1")
	}
}

func TestListError(t *testing.T) {
	client := &LaunchctlClient{
		execCommand: mockExecCommand("", exec.ErrNotFound),
	}

	_, err := client.List(context.Background())
	if err == nil {
		t.Error("List() expected error, got nil")
	}
}

func TestGetUserID_NotHardcoded(t *testing.T) {
	uid := getUserID()

	// getUserID should return os.Getuid() -- verify it's not hardcoded to 501
	// On CI or other users, the UID will differ
	expected := os.Getuid()
	if uid != expected {
		t.Errorf("getUserID() = %d, want os.Getuid() = %d", uid, expected)
	}

	// Sanity: should be a valid positive UID (or 0 for root)
	if uid < 0 {
		t.Errorf("getUserID() returned negative UID: %d", uid)
	}
}

func TestParseListOutput_EmptyString(t *testing.T) {
	services, err := parseListOutput("")
	if err != nil {
		t.Fatalf("parseListOutput() error = %v", err)
	}
	if len(services) != 0 {
		t.Errorf("Expected 0 services for empty output, got %d", len(services))
	}
}

func TestParseListOutput_OnlyWhitespace(t *testing.T) {
	services, err := parseListOutput("   \n\t\n  ")
	if err != nil {
		t.Fatalf("parseListOutput() error = %v", err)
	}
	// First line is treated as header, remaining lines may be empty/whitespace
	// Should not produce any services
	if len(services) != 0 {
		t.Errorf("Expected 0 services for whitespace output, got %d", len(services))
	}
}

func TestParseListOutput_ServiceWithExitCode(t *testing.T) {
	output := `PID	Status	Label
-	78	com.example.crashed`

	services, err := parseListOutput(output)
	if err != nil {
		t.Fatalf("parseListOutput() error = %v", err)
	}
	if len(services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(services))
	}
	if services[0].Status != "78" {
		t.Errorf("Status = %q, want %q", services[0].Status, "78")
	}
	if services[0].PID != "-" {
		t.Errorf("PID = %q, want %q", services[0].PID, "-")
	}
}

func TestParsePrintOutput_StateVariations(t *testing.T) {
	tests := []struct {
		name       string
		output     string
		wantStatus string
	}{
		{
			name: "running state",
			output: `test = {
	state = running
	pid = 1234
}`,
			wantStatus: "running",
		},
		{
			name: "waiting state",
			output: `test = {
	state = waiting
}`,
			wantStatus: "waiting",
		},
		{
			name: "stopped state",
			output: `test = {
	state = stopped
}`,
			wantStatus: "stopped",
		},
		{
			name: "spawn scheduled state",
			output: `test = {
	state = spawn scheduled
	pid = 1234
}`,
			wantStatus: "spawn scheduled",
		},
		{
			name: "not running state",
			output: `test = {
	state = not running
}`,
			wantStatus: "not running",
		},
		{
			name: "first state line wins over nested",
			output: `test = {
	state = spawn scheduled
	resource coalition = {
		state = active
	}
	jetsam coalition = {
		state = active
	}
}`,
			wantStatus: "spawn scheduled",
		},
		{
			name:       "no state line",
			output:     `test = { label = test }`,
			wantStatus: "unknown",
		},
		{
			name:       "empty output",
			output:     "",
			wantStatus: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := parsePrintOutput("test", tt.output)
			if err != nil {
				t.Fatalf("parsePrintOutput() error = %v", err)
			}
			if status.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", status.Status, tt.wantStatus)
			}
		})
	}
}

func TestGetStatus_ServiceNotFoundOutput(t *testing.T) {
	// Simulate launchctl returning "Could not find service" text
	mockCmd := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.Command(
			"sh",
			"-c",
			"echo 'Could not find service \"test\" in domain for uid: 501' && exit 1",
		)
	}

	client := &LaunchctlClient{execCommand: mockCmd}
	_, err := client.GetStatus(context.Background(), "test")
	if err == nil {
		t.Error("GetStatus() should return error for not-found service")
	}
	if !strings.Contains(err.Error(), "service not found") {
		t.Errorf("Error should mention 'service not found', got: %v", err)
	}
}

func TestGetStatus_FallbackToList(t *testing.T) {
	tests := []struct {
		name       string
		label      string
		listOutput string
		want       *ServiceStatus
		wantErr    bool
	}{
		{
			name:  "running service found via list fallback",
			label: "window-cycle",
			listOutput: "PID\tStatus\tLabel\n" +
				"12345\t0\twindow-cycle\n",
			want: &ServiceStatus{
				Label:  "window-cycle",
				PID:    "12345",
				Status: "running",
				Loaded: true,
			},
		},
		{
			name:  "stopped service found via list fallback",
			label: "window-cycle",
			listOutput: "PID\tStatus\tLabel\n" +
				"-\t0\twindow-cycle\n",
			want: &ServiceStatus{
				Label:  "window-cycle",
				PID:    "-",
				Status: "stopped",
				Loaded: true,
			},
		},
		{
			name:  "crashed service found via list fallback",
			label: "window-cycle",
			listOutput: "PID\tStatus\tLabel\n" +
				"-\t78\twindow-cycle\n",
			want: &ServiceStatus{
				Label:  "window-cycle",
				PID:    "-",
				Status: "error",
				Loaded: true,
			},
		},
		{
			name:       "service not in list either",
			label:      "ghost-service",
			listOutput: "PID\tStatus\tLabel\n",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCmd := func(ctx context.Context, name string, args ...string) *exec.Cmd {
				if len(args) > 0 && args[0] == "print" {
					return exec.Command(
						"sh",
						"-c",
						fmt.Sprintf(
							"echo 'Could not find service \"%s\" in domain for uid: 501' >&2; exit 1",
							tt.label,
						),
					)
				}
				// launchctl list
				return exec.Command("echo", tt.listOutput)
			}

			client := &LaunchctlClient{execCommand: mockCmd}
			got, err := client.GetStatus(context.Background(), tt.label)
			if (err != nil) != tt.wantErr {
				t.Fatalf("GetStatus() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Label != tt.want.Label ||
				got.PID != tt.want.PID ||
				got.Status != tt.want.Status ||
				got.Loaded != tt.want.Loaded {
				t.Errorf("GetStatus() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestStatusFromListEntry(t *testing.T) {
	tests := []struct {
		name       string
		input      ServiceStatus
		wantStatus string
		wantPID    string
	}{
		{
			name:       "running with PID",
			input:      ServiceStatus{Label: "svc", PID: "12345", Status: "0"},
			wantStatus: "running",
			wantPID:    "12345",
		},
		{
			name:       "stopped cleanly",
			input:      ServiceStatus{Label: "svc", PID: "-", Status: "0"},
			wantStatus: "stopped",
			wantPID:    "-",
		},
		{
			name:       "crashed with exit code",
			input:      ServiceStatus{Label: "svc", PID: "-", Status: "78"},
			wantStatus: "error",
			wantPID:    "-",
		},
		{
			name:       "empty PID treated as stopped",
			input:      ServiceStatus{Label: "svc", PID: "", Status: "0"},
			wantStatus: "stopped",
			wantPID:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := statusFromListEntry(&tt.input)
			if got.Status != tt.wantStatus {
				t.Errorf("Status = %q, want %q", got.Status, tt.wantStatus)
			}
			if got.PID != tt.wantPID {
				t.Errorf("PID = %q, want %q", got.PID, tt.wantPID)
			}
			if !got.Loaded {
				t.Error("Loaded should be true")
			}
		})
	}
}

func TestNewLaunchctlClient(t *testing.T) {
	client := NewLaunchctlClient()
	if client == nil {
		t.Fatal("NewLaunchctlClient() returned nil")
	}
	if client.execCommand == nil {
		t.Error("execCommand should not be nil")
	}
}

func TestParseListOutput_LargeServiceCount(t *testing.T) {
	var b strings.Builder
	b.WriteString("PID\tStatus\tLabel\n")
	for i := 0; i < 100; i++ {
		if i%2 == 0 {
			fmt.Fprintf(&b, "%d\t0\tcom.example.service%d\n", 10000+i, i)
		} else {
			fmt.Fprintf(&b, "-\t0\tcom.example.service%d\n", i)
		}
	}

	services, err := parseListOutput(b.String())
	if err != nil {
		t.Fatalf("parseListOutput() error = %v", err)
	}
	if len(services) != 100 {
		t.Errorf("Expected 100 services, got %d", len(services))
	}
}
