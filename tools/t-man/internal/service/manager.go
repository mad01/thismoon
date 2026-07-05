package service

import (
	"context"
	"errors"
	"fmt"
)

// ErrServiceNotFound is returned when a service does not exist
var ErrServiceNotFound = errors.New("service: service not found")

// ServiceNotFoundError returns a wrapped ErrServiceNotFound with the service name
func ServiceNotFoundError(name string) error {
	return fmt.Errorf("%w: %s", ErrServiceNotFound, name)
}

// Manager defines the interface for managing services.
// It deliberately has no fleet-wide reconcile: every operation targets a
// single named service, so no code path can mass-delete managed services.
type Manager interface {
	// Create creates a single service without affecting other managed services
	Create(ctx context.Context, def *Definition) error

	// Update updates a single service in place without affecting other managed services
	Update(ctx context.Context, def *Definition) error

	// List returns all currently managed services
	List(ctx context.Context) ([]*Definition, error)

	// Get retrieves a specific service by name
	Get(ctx context.Context, name string) (*Definition, error)

	// Delete removes a service
	Delete(ctx context.Context, name string) error

	// Start starts a service
	Start(ctx context.Context, name string) error

	// Stop stops a service
	Stop(ctx context.Context, name string) error

	// Status returns the status of a service
	Status(ctx context.Context, name string) (string, error)
}
