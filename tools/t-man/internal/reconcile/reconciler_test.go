package reconcile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/thismoon/tools/t-man/internal/service"
)

// mockManager is a mock implementation of service.Manager for testing
type mockManager struct {
	services map[string]*service.Definition
	calls    []string // Track method calls for verification
}

func newMockManager() *mockManager {
	return &mockManager{
		services: make(map[string]*service.Definition),
		calls:    make([]string, 0),
	}
}

func (m *mockManager) Get(ctx context.Context, name string) (*service.Definition, error) {
	m.calls = append(m.calls, fmt.Sprintf("Get(%s)", name))
	if svc, exists := m.services[name]; exists {
		return svc, nil
	}
	return nil, service.ServiceNotFoundError(name)
}

func (m *mockManager) Create(ctx context.Context, def *service.Definition) error {
	m.calls = append(m.calls, fmt.Sprintf("Create(%s)", def.Name))
	m.services[def.Name] = def
	return nil
}

func (m *mockManager) Update(ctx context.Context, def *service.Definition) error {
	m.calls = append(m.calls, fmt.Sprintf("Update(%s)", def.Name))
	m.services[def.Name] = def
	return nil
}

func (m *mockManager) Delete(ctx context.Context, name string) error {
	m.calls = append(m.calls, fmt.Sprintf("Delete(%s)", name))
	if _, exists := m.services[name]; !exists {
		return service.ServiceNotFoundError(name)
	}
	delete(m.services, name)
	return nil
}

func (m *mockManager) List(ctx context.Context) ([]*service.Definition, error) {
	m.calls = append(m.calls, "List()")
	services := make([]*service.Definition, 0, len(m.services))
	for _, svc := range m.services {
		services = append(services, svc)
	}
	return services, nil
}

func (m *mockManager) Start(ctx context.Context, name string) error {
	m.calls = append(m.calls, fmt.Sprintf("Start(%s)", name))
	return nil
}

func (m *mockManager) Stop(ctx context.Context, name string) error {
	m.calls = append(m.calls, fmt.Sprintf("Stop(%s)", name))
	return nil
}

func (m *mockManager) Status(ctx context.Context, name string) (string, error) {
	m.calls = append(m.calls, fmt.Sprintf("Status(%s)", name))
	return "unknown", nil
}

func TestReconciler_Idempotency(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	mgr := newMockManager()
	r := NewReconciler(mgr)
	ctx := context.Background()

	desired := &service.Definition{
		Name:      "test-service",
		Command:   execPath,
		RunAtLoad: true,
	}

	// First reconciliation - should create the service
	result1, err := r.Reconcile(ctx, desired)
	if err != nil {
		t.Fatalf("First Reconcile() error = %v", err)
	}

	if !result1.Applied {
		t.Error("First Reconcile() should have applied changes")
	}

	if result1.Change.Type != ChangeTypeCreate {
		t.Errorf(
			"First Reconcile() change type = %v, want %v",
			result1.Change.Type,
			ChangeTypeCreate,
		)
	}

	// Second reconciliation with same input - should detect no changes (idempotency)
	result2, err := r.Reconcile(ctx, desired)
	if err != nil {
		t.Fatalf("Second Reconcile() error = %v", err)
	}

	if result2.Applied {
		t.Error("Second Reconcile() should NOT have applied changes (idempotency violation)")
	}

	if result2.Change.Type != ChangeTypeNone {
		t.Errorf(
			"Second Reconcile() change type = %v, want %v",
			result2.Change.Type,
			ChangeTypeNone,
		)
	}

	// Third reconciliation - verify idempotency still holds
	result3, err := r.Reconcile(ctx, desired)
	if err != nil {
		t.Fatalf("Third Reconcile() error = %v", err)
	}

	if result3.Applied {
		t.Error("Third Reconcile() should NOT have applied changes (idempotency violation)")
	}

	if result3.Change.Type != ChangeTypeNone {
		t.Errorf("Third Reconcile() change type = %v, want %v", result3.Change.Type, ChangeTypeNone)
	}
}

func TestReconciler_ChangeDetection(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	mgr := newMockManager()
	r := NewReconciler(mgr)
	ctx := context.Background()

	// Initial service
	initial := &service.Definition{
		Name:      "test-service",
		Command:   execPath,
		RunAtLoad: true,
	}

	// Create initial service
	result1, err := r.Reconcile(ctx, initial)
	if err != nil {
		t.Fatalf("Initial Reconcile() error = %v", err)
	}

	if result1.Change.Type != ChangeTypeCreate {
		t.Errorf(
			"Initial Reconcile() change type = %v, want %v",
			result1.Change.Type,
			ChangeTypeCreate,
		)
	}

	// Modified service (different args)
	modified := &service.Definition{
		Name:      "test-service",
		Command:   execPath,
		Args:      []string{"new-arg"},
		RunAtLoad: true,
	}

	// Reconcile with modified service - should detect change
	result2, err := r.Reconcile(ctx, modified)
	if err != nil {
		t.Fatalf("Modified Reconcile() error = %v", err)
	}

	if !result2.Applied {
		t.Error("Modified Reconcile() should have applied changes")
	}

	if result2.Change.Type != ChangeTypeUpdate {
		t.Errorf(
			"Modified Reconcile() change type = %v, want %v",
			result2.Change.Type,
			ChangeTypeUpdate,
		)
	}

	// Reconcile again with same modified service - should be idempotent
	result3, err := r.Reconcile(ctx, modified)
	if err != nil {
		t.Fatalf("Second modified Reconcile() error = %v", err)
	}

	if result3.Applied {
		t.Error("Second modified Reconcile() should NOT have applied changes")
	}

	if result3.Change.Type != ChangeTypeNone {
		t.Errorf(
			"Second modified Reconcile() change type = %v, want %v",
			result3.Change.Type,
			ChangeTypeNone,
		)
	}
}

func TestReconciler_NewServiceCreation(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	mgr := newMockManager()
	r := NewReconciler(mgr)
	ctx := context.Background()

	desired := &service.Definition{
		Name:      "new-service",
		Command:   execPath,
		RunAtLoad: true,
		KeepAlive: true,
	}

	result, err := r.Reconcile(ctx, desired)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if !result.Applied {
		t.Error("Reconcile() should have applied changes for new service")
	}

	if result.Change.Type != ChangeTypeCreate {
		t.Errorf("Reconcile() change type = %v, want %v", result.Change.Type, ChangeTypeCreate)
	}

	if result.ServiceName != "new-service" {
		t.Errorf("Reconcile() service name = %v, want %v", result.ServiceName, "new-service")
	}

	// Verify service was created in manager
	svc, err := mgr.Get(ctx, "new-service")
	if err != nil {
		t.Fatalf("Service not created in manager: %v", err)
	}

	if svc.Name != "new-service" {
		t.Errorf("Created service name = %v, want %v", svc.Name, "new-service")
	}
}

func TestReconciler_ServiceUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	mgr := newMockManager()
	r := NewReconciler(mgr)
	ctx := context.Background()

	// Create initial service
	initial := &service.Definition{
		Name:      "update-service",
		Command:   execPath,
		RunAtLoad: false,
	}

	mgr.services["update-service"] = initial

	// Update with different configuration
	updated := &service.Definition{
		Name:      "update-service",
		Command:   execPath,
		RunAtLoad: true, // Changed from false to true
	}

	result, err := r.Reconcile(ctx, updated)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}

	if !result.Applied {
		t.Error("Reconcile() should have applied changes for update")
	}

	if result.Change.Type != ChangeTypeUpdate {
		t.Errorf("Reconcile() change type = %v, want %v", result.Change.Type, ChangeTypeUpdate)
	}

	// Verify service was updated
	svc, err := mgr.Get(ctx, "update-service")
	if err != nil {
		t.Fatalf("Updated service not found: %v", err)
	}

	if !svc.RunAtLoad {
		t.Error("Service RunAtLoad should be true after update")
	}
}

func TestReconciler_ValidationError(t *testing.T) {
	mgr := newMockManager()
	r := NewReconciler(mgr)
	ctx := context.Background()

	// Service with invalid command (empty)
	invalid := &service.Definition{
		Name:    "invalid-service",
		Command: "", // Invalid - command required
	}

	result, err := r.Reconcile(ctx, invalid)
	if err == nil {
		t.Error("Reconcile() should return error for invalid definition")
	}

	if result == nil {
		t.Fatal("Reconcile() should return result even on validation error")
	}

	if result.Error == nil {
		t.Error("Result should contain error for invalid definition")
	}

	if result.Applied {
		t.Error("Reconcile() should not apply invalid definition")
	}
}

func TestReconciler_ReconcileMany(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	mgr := newMockManager()
	r := NewReconciler(mgr)
	ctx := context.Background()

	desired := []*service.Definition{
		{
			Name:      "service-1",
			Command:   execPath,
			RunAtLoad: true,
		},
		{
			Name:      "service-2",
			Command:   execPath,
			RunAtLoad: false,
		},
		{
			Name:      "service-3",
			Command:   execPath,
			KeepAlive: true,
		},
	}

	results, err := r.ReconcileMany(ctx, desired)
	if err != nil {
		t.Fatalf("ReconcileMany() error = %v", err)
	}

	if len(results) != 3 {
		t.Errorf("ReconcileMany() returned %d results, want 3", len(results))
	}

	// All should be creates
	for i, result := range results {
		if result.Change.Type != ChangeTypeCreate {
			t.Errorf(
				"Result[%d] change type = %v, want %v",
				i,
				result.Change.Type,
				ChangeTypeCreate,
			)
		}
		if !result.Applied {
			t.Errorf("Result[%d] should have applied changes", i)
		}
	}

	// Second reconciliation - should be idempotent
	results2, err := r.ReconcileMany(ctx, desired)
	if err != nil {
		t.Fatalf("Second ReconcileMany() error = %v", err)
	}

	for i, result := range results2 {
		if result.Change.Type != ChangeTypeNone {
			t.Errorf(
				"Result[%d] change type = %v, want %v (idempotency violation)",
				i,
				result.Change.Type,
				ChangeTypeNone,
			)
		}
		if result.Applied {
			t.Errorf("Result[%d] should NOT have applied changes (idempotency)", i)
		}
	}
}

func TestReconciler_CreateDoesNotDeleteOtherServices(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	mgr := newMockManager()
	r := NewReconciler(mgr)
	ctx := context.Background()

	// Create service A
	serviceA := &service.Definition{
		Name:      "service-a",
		Command:   execPath,
		RunAtLoad: true,
	}
	resultA, err := r.Reconcile(ctx, serviceA)
	if err != nil {
		t.Fatalf("Reconcile(service-a) error = %v", err)
	}
	if resultA.Change.Type != ChangeTypeCreate {
		t.Fatalf("Expected service-a to be created, got %v", resultA.Change.Type)
	}

	// Verify service A exists
	if _, err := mgr.Get(ctx, "service-a"); err != nil {
		t.Fatalf("service-a should exist after creation: %v", err)
	}

	// Create service B — this must NOT delete service A
	serviceB := &service.Definition{
		Name:      "service-b",
		Command:   execPath,
		RunAtLoad: true,
	}
	resultB, err := r.Reconcile(ctx, serviceB)
	if err != nil {
		t.Fatalf("Reconcile(service-b) error = %v", err)
	}
	if resultB.Change.Type != ChangeTypeCreate {
		t.Fatalf("Expected service-b to be created, got %v", resultB.Change.Type)
	}

	// Verify BOTH services still exist
	if _, err := mgr.Get(ctx, "service-a"); err != nil {
		t.Errorf("service-a was deleted when service-b was created — this is the critical bug")
	}
	if _, err := mgr.Get(ctx, "service-b"); err != nil {
		t.Errorf("service-b should exist after creation: %v", err)
	}

	// Verify no Delete calls were made
	for _, call := range mgr.calls {
		if call == "Delete(service-a)" {
			t.Errorf(
				"Delete(service-a) was called — creating service-b should not delete service-a",
			)
		}
	}
}

func TestReconciler_NilDefinition(t *testing.T) {
	mgr := newMockManager()
	r := NewReconciler(mgr)
	ctx := context.Background()

	result, err := r.Reconcile(ctx, nil)
	if err == nil {
		t.Error("Reconcile() should return error for nil definition")
	}

	if result != nil {
		t.Error("Reconcile() should return nil result for nil definition")
	}
}

// errorManager is a mock that returns errors at specific stages
type errorManager struct {
	mockManager
	getErr    error
	createErr error
	updateErr error
	deleteErr error
}

func newErrorManager() *errorManager {
	return &errorManager{
		mockManager: mockManager{
			services: make(map[string]*service.Definition),
			calls:    make([]string, 0),
		},
	}
}

func (m *errorManager) Get(ctx context.Context, name string) (*service.Definition, error) {
	m.calls = append(m.calls, fmt.Sprintf("Get(%s)", name))
	if m.getErr != nil {
		return nil, m.getErr
	}
	if svc, exists := m.services[name]; exists {
		return svc, nil
	}
	return nil, service.ServiceNotFoundError(name)
}

func (m *errorManager) Create(ctx context.Context, def *service.Definition) error {
	m.calls = append(m.calls, fmt.Sprintf("Create(%s)", def.Name))
	if m.createErr != nil {
		return m.createErr
	}
	m.services[def.Name] = def
	return nil
}

func (m *errorManager) Update(ctx context.Context, def *service.Definition) error {
	m.calls = append(m.calls, fmt.Sprintf("Update(%s)", def.Name))
	if m.updateErr != nil {
		return m.updateErr
	}
	m.services[def.Name] = def
	return nil
}

func (m *errorManager) Delete(ctx context.Context, name string) error {
	m.calls = append(m.calls, fmt.Sprintf("Delete(%s)", name))
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.services, name)
	return nil
}

func TestReconciler_UpdateDoesNotAffectOtherServices(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	mgr := newMockManager()
	r := NewReconciler(mgr)
	ctx := context.Background()

	// Create two services
	serviceA := &service.Definition{Name: "service-a", Command: execPath, RunAtLoad: true}
	serviceB := &service.Definition{Name: "service-b", Command: execPath, RunAtLoad: false}

	if _, err := r.Reconcile(ctx, serviceA); err != nil {
		t.Fatalf("Reconcile(service-a) error = %v", err)
	}
	if _, err := r.Reconcile(ctx, serviceB); err != nil {
		t.Fatalf("Reconcile(service-b) error = %v", err)
	}

	// Update service B
	serviceBUpdated := &service.Definition{Name: "service-b", Command: execPath, RunAtLoad: true}
	result, err := r.Reconcile(ctx, serviceBUpdated)
	if err != nil {
		t.Fatalf("Reconcile(service-b update) error = %v", err)
	}
	if result.Change.Type != ChangeTypeUpdate {
		t.Errorf("Expected update, got %v", result.Change.Type)
	}

	// Verify service A is untouched
	svcA, err := mgr.Get(ctx, "service-a")
	if err != nil {
		t.Fatalf("service-a was lost after updating service-b: %v", err)
	}
	if svcA.RunAtLoad != true {
		t.Error("service-a was modified when updating service-b")
	}

	// Verify no Delete calls for service-a
	for _, call := range mgr.calls {
		if call == "Delete(service-a)" {
			t.Error("Delete(service-a) was called when updating service-b")
		}
	}
}

func TestReconciler_UpdateNonexistentService(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	mgr := newMockManager()
	r := NewReconciler(mgr)
	ctx := context.Background()

	// Reconcile a service that doesn't exist yet -- should create it
	desired := &service.Definition{Name: "nonexistent", Command: execPath, RunAtLoad: true}
	result, err := r.Reconcile(ctx, desired)
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if result.Change.Type != ChangeTypeCreate {
		t.Errorf("Expected create for nonexistent service, got %v", result.Change.Type)
	}
	if !result.Applied {
		t.Error("Expected Applied=true for new service creation")
	}
}

func TestReconciler_RemoveNonexistentService(t *testing.T) {
	// CompareStates with current=nil desired=nil => ChangeTypeNone
	// CompareStates with current=something desired=nil => ChangeTypeDelete
	// The reconciler only handles the case where desired is provided,
	// so we test the graceful handling at the state comparison level
	change, err := CompareStates(nil, nil)
	if err != nil {
		t.Fatalf("CompareStates(nil, nil) error = %v", err)
	}
	if change.Type != ChangeTypeNone {
		t.Errorf("CompareStates(nil, nil) = %v, want ChangeTypeNone", change.Type)
	}
}

func TestReconciler_IdempotentCreate(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	mgr := newMockManager()
	r := NewReconciler(mgr)
	ctx := context.Background()

	desired := &service.Definition{Name: "idempotent-svc", Command: execPath, RunAtLoad: true}

	// First create
	result1, err := r.Reconcile(ctx, desired)
	if err != nil {
		t.Fatalf("First Reconcile() error = %v", err)
	}
	if result1.Change.Type != ChangeTypeCreate {
		t.Errorf("First reconcile should create, got %v", result1.Change.Type)
	}

	// Second reconcile with same definition = no-op
	result2, err := r.Reconcile(ctx, desired)
	if err != nil {
		t.Fatalf("Second Reconcile() error = %v", err)
	}
	if result2.Change.Type != ChangeTypeNone {
		t.Errorf("Second reconcile should be no-op, got %v", result2.Change.Type)
	}
	if result2.Applied {
		t.Error("Second reconcile should not apply anything")
	}

	// Count Create calls -- should be exactly 1
	createCount := 0
	for _, call := range mgr.calls {
		if call == "Create(idempotent-svc)" {
			createCount++
		}
	}
	if createCount != 1 {
		t.Errorf("Expected exactly 1 Create call, got %d", createCount)
	}
}

func TestReconciler_ManagerGetError(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	mgr := newErrorManager()
	mgr.getErr = fmt.Errorf("disk I/O error")

	r := NewReconciler(mgr)
	ctx := context.Background()

	desired := &service.Definition{Name: "test-svc", Command: execPath}
	result, err := r.Reconcile(ctx, desired)
	if err == nil {
		t.Fatal("Expected error from Get failure")
	}
	if result == nil {
		t.Fatal("Expected non-nil result even on error")
	}
	if result.Error == nil {
		t.Error("Result.Error should be set on Get failure")
	}
}

func TestReconciler_ManagerCreateError(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	mgr := newErrorManager()
	mgr.createErr = fmt.Errorf("permission denied")

	r := NewReconciler(mgr)
	ctx := context.Background()

	desired := &service.Definition{Name: "test-svc", Command: execPath}
	result, err := r.Reconcile(ctx, desired)
	if err == nil {
		t.Fatal("Expected error from Create failure")
	}
	if result == nil {
		t.Fatal("Expected non-nil result even on error")
	}
	if !result.Applied == false {
		t.Error("Applied should be false on Create failure")
	}
}

func TestReconciler_ManagerUpdateError(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	mgr := newErrorManager()
	r := NewReconciler(mgr)
	ctx := context.Background()

	// Create a service first (no error on create)
	initial := &service.Definition{Name: "test-svc", Command: execPath, RunAtLoad: false}
	if _, err := r.Reconcile(ctx, initial); err != nil {
		t.Fatalf("Initial Reconcile() error = %v", err)
	}

	// Now set update to fail
	mgr.updateErr = fmt.Errorf("update failed")

	updated := &service.Definition{Name: "test-svc", Command: execPath, RunAtLoad: true}
	result, err := r.Reconcile(ctx, updated)
	if err == nil {
		t.Fatal("Expected error from Update failure")
	}
	if result == nil {
		t.Fatal("Expected non-nil result even on error")
	}
	if result.Change.Type != ChangeTypeUpdate {
		t.Errorf("Expected update change type, got %v", result.Change.Type)
	}
}

func TestReconciler_ReconcileManyPartialFailure(t *testing.T) {
	tmpDir := t.TempDir()
	execPath := filepath.Join(tmpDir, "test-exec")
	if err := os.WriteFile(execPath, []byte("#!/bin/sh\necho test"), 0o755); err != nil {
		t.Fatalf("Failed to create test executable: %v", err)
	}

	desired := []*service.Definition{
		{Name: "good-service", Command: execPath, RunAtLoad: true},
		{Name: "bad-service", Command: ""}, // Invalid: empty command
		{Name: "another-good", Command: execPath, RunAtLoad: false},
	}

	mgr := newMockManager()
	r := NewReconciler(mgr)
	ctx := context.Background()

	results, err := r.ReconcileMany(ctx, desired)
	// ReconcileMany should not return a top-level error
	if err != nil {
		t.Fatalf("ReconcileMany() should not return top-level error, got %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	// First should succeed
	if results[0].Error != nil {
		t.Errorf("results[0] should succeed, got error: %v", results[0].Error)
	}
	if results[0].Change.Type != ChangeTypeCreate {
		t.Errorf("results[0] change type = %v, want create", results[0].Change.Type)
	}

	// Second should fail validation
	if results[1].Error == nil {
		t.Error("results[1] should fail validation for empty command")
	}

	// Third should still succeed despite second failing
	if results[2].Error != nil {
		t.Errorf("results[2] should succeed, got error: %v", results[2].Error)
	}
	if results[2].Change.Type != ChangeTypeCreate {
		t.Errorf("results[2] change type = %v, want create", results[2].Change.Type)
	}
}

func TestReconcileResult_String(t *testing.T) {
	tests := []struct {
		name   string
		result *ReconcileResult
		want   string
	}{
		{
			name: "error result",
			result: &ReconcileResult{
				ServiceName: "svc",
				Error:       fmt.Errorf("something broke"),
			},
			want: "svc: error - something broke",
		},
		{
			name: "applied result",
			result: &ReconcileResult{
				ServiceName: "svc",
				Change:      &Change{Type: ChangeTypeCreate},
				Applied:     true,
			},
			want: "svc: create (applied)",
		},
		{
			name: "no-op result",
			result: &ReconcileResult{
				ServiceName: "svc",
				Change:      &Change{Type: ChangeTypeNone},
				Applied:     false,
			},
			want: "svc: none (no action needed)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.result.String()
			if got != tt.want {
				t.Errorf("ReconcileResult.String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReconciler_ReconcileManyEmpty(t *testing.T) {
	mgr := newMockManager()
	r := NewReconciler(mgr)
	ctx := context.Background()

	results, err := r.ReconcileMany(ctx, []*service.Definition{})
	if err != nil {
		t.Fatalf("ReconcileMany() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results for empty input, got %d", len(results))
	}
}

func TestReconciler_ReconcileManyNil(t *testing.T) {
	mgr := newMockManager()
	r := NewReconciler(mgr)
	ctx := context.Background()

	results, err := r.ReconcileMany(ctx, nil)
	if err != nil {
		t.Fatalf("ReconcileMany() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results for nil input, got %d", len(results))
	}
}
