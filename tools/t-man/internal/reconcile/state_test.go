package reconcile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/thismoon/tools/t-man/internal/service"
)

func TestCompareStates_Create(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	desired := &service.Definition{
		Name:      "new-service",
		Command:   execPath,
		RunAtLoad: true,
	}

	change, err := CompareStates(nil, desired)
	if err != nil {
		t.Fatalf("CompareStates() error = %v", err)
	}

	if change.Type != ChangeTypeCreate {
		t.Errorf("CompareStates() change type = %v, want %v", change.Type, ChangeTypeCreate)
	}

	if change.ServiceName != "new-service" {
		t.Errorf("CompareStates() service name = %v, want %v", change.ServiceName, "new-service")
	}

	if change.Reason != "service does not exist" {
		t.Errorf("CompareStates() reason = %v, want %v", change.Reason, "service does not exist")
	}
}

func TestCompareStates_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	current := &service.Definition{
		Name:      "old-service",
		Command:   execPath,
		RunAtLoad: true,
	}

	change, err := CompareStates(current, nil)
	if err != nil {
		t.Fatalf("CompareStates() error = %v", err)
	}

	if change.Type != ChangeTypeDelete {
		t.Errorf("CompareStates() change type = %v, want %v", change.Type, ChangeTypeDelete)
	}

	if change.ServiceName != "old-service" {
		t.Errorf("CompareStates() service name = %v, want %v", change.ServiceName, "old-service")
	}

	if change.Reason != "service no longer in desired state" {
		t.Errorf(
			"CompareStates() reason = %v, want %v",
			change.Reason,
			"service no longer in desired state",
		)
	}
}

func TestCompareStates_Update(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	current := &service.Definition{
		Name:      "test-service",
		Command:   execPath,
		RunAtLoad: false,
	}

	desired := &service.Definition{
		Name:      "test-service",
		Command:   execPath,
		RunAtLoad: true, // Changed
	}

	change, err := CompareStates(current, desired)
	if err != nil {
		t.Fatalf("CompareStates() error = %v", err)
	}

	if change.Type != ChangeTypeUpdate {
		t.Errorf("CompareStates() change type = %v, want %v", change.Type, ChangeTypeUpdate)
	}

	if change.ServiceName != "test-service" {
		t.Errorf("CompareStates() service name = %v, want %v", change.ServiceName, "test-service")
	}
}

func TestCompareStates_NoChange(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	current := &service.Definition{
		Name:      "test-service",
		Command:   execPath,
		RunAtLoad: true,
		KeepAlive: true,
	}

	desired := &service.Definition{
		Name:      "test-service",
		Command:   execPath,
		RunAtLoad: true,
		KeepAlive: true,
	}

	change, err := CompareStates(current, desired)
	if err != nil {
		t.Fatalf("CompareStates() error = %v", err)
	}

	if change.Type != ChangeTypeNone {
		t.Errorf("CompareStates() change type = %v, want %v", change.Type, ChangeTypeNone)
	}

	if change.ServiceName != "test-service" {
		t.Errorf("CompareStates() service name = %v, want %v", change.ServiceName, "test-service")
	}

	if change.Reason != "configuration unchanged" {
		t.Errorf("CompareStates() reason = %v, want %v", change.Reason, "configuration unchanged")
	}
}

func TestCompareStates_BothNil(t *testing.T) {
	change, err := CompareStates(nil, nil)
	if err != nil {
		t.Fatalf("CompareStates() error = %v", err)
	}

	if change.Type != ChangeTypeNone {
		t.Errorf("CompareStates() change type = %v, want %v", change.Type, ChangeTypeNone)
	}
}

func TestCompareStates_DifferentArgs(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	current := &service.Definition{
		Name:    "test-service",
		Command: execPath,
		Args:    []string{"arg1", "arg2"},
	}

	desired := &service.Definition{
		Name:    "test-service",
		Command: execPath,
		Args:    []string{"arg1", "arg3"}, // Different arg
	}

	change, err := CompareStates(current, desired)
	if err != nil {
		t.Fatalf("CompareStates() error = %v", err)
	}

	if change.Type != ChangeTypeUpdate {
		t.Errorf("CompareStates() change type = %v, want %v", change.Type, ChangeTypeUpdate)
	}
}

func TestCompareStates_DifferentEnvironment(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	current := &service.Definition{
		Name:        "test-service",
		Command:     execPath,
		Environment: map[string]string{"KEY": "value1"},
	}

	desired := &service.Definition{
		Name:        "test-service",
		Command:     execPath,
		Environment: map[string]string{"KEY": "value2"}, // Different value
	}

	change, err := CompareStates(current, desired)
	if err != nil {
		t.Fatalf("CompareStates() error = %v", err)
	}

	if change.Type != ChangeTypeUpdate {
		t.Errorf("CompareStates() change type = %v, want %v", change.Type, ChangeTypeUpdate)
	}
}

func TestChangeString(t *testing.T) {
	tests := []struct {
		name   string
		change *Change
		want   string
	}{
		{
			name: "create change",
			change: &Change{
				Type:        ChangeTypeCreate,
				ServiceName: "test-service",
				Reason:      "service does not exist",
			},
			want: "Create service 'test-service': service does not exist",
		},
		{
			name: "update change",
			change: &Change{
				Type:        ChangeTypeUpdate,
				ServiceName: "test-service",
				Reason:      "configuration changed",
			},
			want: "Update service 'test-service': configuration changed",
		},
		{
			name: "delete change",
			change: &Change{
				Type:        ChangeTypeDelete,
				ServiceName: "test-service",
				Reason:      "service no longer needed",
			},
			want: "Delete service 'test-service': service no longer needed",
		},
		{
			name: "no change",
			change: &Change{
				Type:        ChangeTypeNone,
				ServiceName: "test-service",
			},
			want: "No change for service 'test-service'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.change.String()
			if got != tt.want {
				t.Errorf("Change.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareStates_HashConsistency(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	// Create two identical definitions
	def1 := &service.Definition{
		Name:        "test-service",
		Command:     execPath,
		Args:        []string{"arg1", "arg2"},
		Environment: map[string]string{"KEY": "value"},
		RunAtLoad:   true,
	}

	def2 := &service.Definition{
		Name:        "test-service",
		Command:     execPath,
		Args:        []string{"arg1", "arg2"},
		Environment: map[string]string{"KEY": "value"},
		RunAtLoad:   true,
	}

	// First comparison
	change1, err := CompareStates(def1, def2)
	if err != nil {
		t.Fatalf("First CompareStates() error = %v", err)
	}

	if change1.Type != ChangeTypeNone {
		t.Errorf("First CompareStates() change type = %v, want %v", change1.Type, ChangeTypeNone)
	}

	// Second comparison (should be consistent)
	change2, err := CompareStates(def1, def2)
	if err != nil {
		t.Fatalf("Second CompareStates() error = %v", err)
	}

	if change2.Type != ChangeTypeNone {
		t.Errorf("Second CompareStates() change type = %v, want %v", change2.Type, ChangeTypeNone)
	}

	if change1.Reason != change2.Reason {
		t.Errorf(
			"CompareStates() not consistent: reason1 = %v, reason2 = %v",
			change1.Reason,
			change2.Reason,
		)
	}
}
