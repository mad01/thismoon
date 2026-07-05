package reconcile

import (
	"fmt"

	"github.com/mad01/thismoon/tools/t-man/internal/service"
)

// ChangeType represents the type of change detected
type ChangeType string

const (
	// ChangeTypeNone indicates no change is needed
	ChangeTypeNone ChangeType = "none"
	// ChangeTypeCreate indicates a service needs to be created
	ChangeTypeCreate ChangeType = "create"
	// ChangeTypeUpdate indicates a service needs to be updated
	ChangeTypeUpdate ChangeType = "update"
	// ChangeTypeDelete indicates a service needs to be deleted
	ChangeTypeDelete ChangeType = "delete"
)

// Change represents a detected change in service state
type Change struct {
	Type        ChangeType
	ServiceName string
	Reason      string
	Current     *service.Definition
	Desired     *service.Definition
}

// String returns a human-readable description of the change
func (c *Change) String() string {
	switch c.Type {
	case ChangeTypeCreate:
		return fmt.Sprintf("Create service '%s': %s", c.ServiceName, c.Reason)
	case ChangeTypeUpdate:
		return fmt.Sprintf("Update service '%s': %s", c.ServiceName, c.Reason)
	case ChangeTypeDelete:
		return fmt.Sprintf("Delete service '%s': %s", c.ServiceName, c.Reason)
	case ChangeTypeNone:
		return fmt.Sprintf("No change for service '%s'", c.ServiceName)
	default:
		return fmt.Sprintf("Unknown change type for service '%s'", c.ServiceName)
	}
}

// CompareStates compares current and desired service definitions
// Returns a Change describing what action is needed
func CompareStates(current, desired *service.Definition) (*Change, error) {
	// Case 1: Desired exists but current doesn't - CREATE
	if desired != nil && current == nil {
		return &Change{
			Type:        ChangeTypeCreate,
			ServiceName: desired.Name,
			Reason:      "service does not exist",
			Desired:     desired,
		}, nil
	}

	// Case 2: Current exists but desired doesn't - DELETE
	if current != nil && desired == nil {
		return &Change{
			Type:        ChangeTypeDelete,
			ServiceName: current.Name,
			Reason:      "service no longer in desired state",
			Current:     current,
		}, nil
	}

	// Case 3: Neither exists - NONE
	if current == nil && desired == nil {
		return &Change{
			Type:   ChangeTypeNone,
			Reason: "both current and desired are nil",
		}, nil
	}

	// Case 4: Both exist - compare using hash
	currentHash, err := current.Hash()
	if err != nil {
		return nil, fmt.Errorf("failed to compute current hash: %w", err)
	}

	desiredHash, err := desired.Hash()
	if err != nil {
		return nil, fmt.Errorf("failed to compute desired hash: %w", err)
	}

	if currentHash != desiredHash {
		return &Change{
			Type:        ChangeTypeUpdate,
			ServiceName: desired.Name,
			Reason: fmt.Sprintf(
				"configuration changed (hash: %s -> %s)",
				currentHash[:8],
				desiredHash[:8],
			),
			Current: current,
			Desired: desired,
		}, nil
	}

	// Hashes match - no change needed
	return &Change{
		Type:        ChangeTypeNone,
		ServiceName: desired.Name,
		Reason:      "configuration unchanged",
		Current:     current,
		Desired:     desired,
	}, nil
}
