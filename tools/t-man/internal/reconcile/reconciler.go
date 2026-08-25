package reconcile

import (
	"context"
	"errors"
	"fmt"

	"github.com/mad01/thismoon/tools/t-man/internal/notify"
	"github.com/mad01/thismoon/tools/t-man/internal/service"
)

// Reconciler implements the reconciliation logic using read-compare-apply pattern
type Reconciler struct {
	manager service.Manager
}

// NewReconciler creates a new Reconciler with the given service manager
func NewReconciler(manager service.Manager) *Reconciler {
	return &Reconciler{
		manager: manager,
	}
}

// ReconcileResult represents the result of a single service reconciliation
type ReconcileResult struct {
	ServiceName string
	Change      *Change
	Applied     bool
	Error       error
}

// String returns a human-readable description of the reconcile result
func (r *ReconcileResult) String() string {
	if r.Error != nil {
		return fmt.Sprintf("%s: error - %v", r.ServiceName, r.Error)
	}
	if r.Applied {
		return fmt.Sprintf("%s: %s (applied)", r.ServiceName, r.Change.Type)
	}
	return fmt.Sprintf("%s: %s (no action needed)", r.ServiceName, r.Change.Type)
}

// Reconcile reconciles a single service using read-compare-apply pattern
// This ensures idempotency by only applying changes when the hash differs
func (r *Reconciler) Reconcile(
	ctx context.Context,
	desired *service.Definition,
) (*ReconcileResult, error) {
	if desired == nil {
		return nil, fmt.Errorf("desired definition cannot be nil")
	}

	// Validate desired state before proceeding
	if err := desired.Validate(); err != nil {
		return &ReconcileResult{
			ServiceName: desired.Name,
			Error:       fmt.Errorf("validation failed: %w", err),
		}, err
	}

	// READ: Get current state
	current, err := r.manager.Get(ctx, desired.Name)
	if err != nil {
		// If service doesn't exist, current will be nil (not an error)
		// Only return error if it's a real failure
		if !errors.Is(err, service.ErrServiceNotFound) {
			return &ReconcileResult{
				ServiceName: desired.Name,
				Error:       fmt.Errorf("failed to get current state: %w", err),
			}, err
		}
		// Service doesn't exist, set current to nil
		current = nil
	}

	// COMPARE: Determine what change is needed
	change, err := CompareStates(current, desired)
	if err != nil {
		return &ReconcileResult{
			ServiceName: desired.Name,
			Error:       fmt.Errorf("failed to compare states: %w", err),
		}, err
	}

	result := &ReconcileResult{
		ServiceName: desired.Name,
		Change:      change,
		Applied:     false,
	}

	// APPLY: Take action based on change type
	switch change.Type {
	case ChangeTypeCreate:
		if err := r.manager.Create(ctx, desired); err != nil {
			result.Error = fmt.Errorf("failed to create service: %w", err)
			emitReconcile(
				"error",
				desired.Name,
				"create",
				"service "+desired.Name+" create failed",
				err.Error(),
			)
			return result, err
		}
		result.Applied = true
		emitReconcile(
			"info",
			desired.Name,
			"create",
			"service "+desired.Name+" created",
			change.Reason,
		)

	case ChangeTypeUpdate:
		if err := r.manager.Update(ctx, desired); err != nil {
			result.Error = fmt.Errorf("failed to update service: %w", err)
			emitReconcile(
				"error",
				desired.Name,
				"update",
				"service "+desired.Name+" update failed",
				err.Error(),
			)
			return result, err
		}
		result.Applied = true
		emitReconcile(
			"info",
			desired.Name,
			"update",
			"service "+desired.Name+" updated",
			change.Reason,
		)

	case ChangeTypeDelete:
		// Delete service
		if err := r.manager.Delete(ctx, current.Name); err != nil {
			result.Error = fmt.Errorf("failed to delete service: %w", err)
			emitReconcile(
				"error",
				desired.Name,
				"delete",
				"service "+desired.Name+" delete failed",
				err.Error(),
			)
			return result, err
		}
		result.Applied = true
		emitReconcile(
			"info",
			desired.Name,
			"delete",
			"service "+desired.Name+" removed",
			change.Reason,
		)

	case ChangeTypeNone:
		// No change needed - idempotency achieved!
		result.Applied = false

	default:
		result.Error = fmt.Errorf("unknown change type: %s", change.Type)
		return result, result.Error
	}

	return result, nil
}

// emitReconcile best-effort records a reconcile action to the local events
// service (events.this). It only fires for applied changes and failures —
// no-op reconciles (ChangeTypeNone) are intentionally silent.
func emitReconcile(level, name, action, title, message string) {
	notify.EmitEvent("t-man", level, title, message,
		map[string]string{"service": name, "action": action})
}

// ReconcileMany reconciles multiple services
func (r *Reconciler) ReconcileMany(
	ctx context.Context,
	desired []*service.Definition,
) ([]*ReconcileResult, error) {
	results := make([]*ReconcileResult, 0, len(desired))

	for _, def := range desired {
		result, err := r.Reconcile(ctx, def)
		results = append(results, result)

		// Continue processing other services even if one fails
		if err != nil {
			// Error is already captured in result.Error
			continue
		}
	}

	return results, nil
}
