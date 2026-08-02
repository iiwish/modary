// Package databasecontrol owns Modary's privileged database assembly boundary.
// It is internal so consumers can receive database.Access but cannot construct,
// install, resolve, or operate migration and transaction control.
package databasecontrol

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"regexp"

	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/safeerr"
)

var driverNamePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,62}$`)

// ErrControlUnavailable reports an invalid or unavailable privileged control.
var ErrControlUnavailable = errors.New("database control is unavailable")

// ServiceName is the reserved internal service name used to install Control
// through the Module Host's typed service registry. It is internal to this
// repository; consumers cannot name Control or obtain the matching service key.
const ServiceName = "modary.database-control"

// Backend is the complete privileged contract implemented by an official
// durable adapter. ReadExecutor and WriteExecutor supply the framework-owned
// consumer facade; the remaining methods own migration, administration, and
// transaction control. WithinTransaction follows the
// synchronous exactly-once private Runtime transaction contract and returns an
// exact framework outcome correlated with the callback error after rollback, or
// with the completion failure after commit or rollback fails. A nested failure
// may instead return rollback-pending proof after marking the outer transaction
// rollback-only. The callback is never retained, retried, overlapped, or called
// after return. A callback panic is rolled back and propagated without exposing
// its value.
type Backend interface {
	Driver() string
	ValidateMigration(string) error
	ReadExecutor(context.Context) (database.Executor, error)
	WriteExecutor(context.Context) (database.Executor, error)
	AdminExecutor(context.Context) (database.Executor, error)
	WithinTransaction(context.Context, func(context.Context) error) error
}

// migrationLocker is an optional durable-adapter extension. Implementations
// serialize the complete migration registry check and apply sequence across
// processes while the surrounding transaction remains open.
type migrationLocker interface {
	LockMigrations(context.Context, database.Executor) error
}

// Control is the internal capability used by Host assembly and official
// durable adapters. It is deliberately absent from package database.
type Control interface {
	Driver() string
	Access() database.Access
	Executor(context.Context) (database.Executor, error)
	WithinTransaction(context.Context, func(context.Context) error) error
	ApplyMigrations(context.Context, string, fs.FS) error
	databaseControl()
}

type control struct {
	backend Backend
	access  database.Access
	driver  string
}

type dependencyError struct {
	operation string
	cause     error
}

// New validates one privileged adapter Backend and constructs its sole
// framework-owned public Access facade.
func New(backend Backend) (Control, error) {
	if isNil(backend) {
		return nil, fmt.Errorf("%w: backend is required", ErrControlUnavailable)
	}
	driver, err := invokeDependency("read database driver", func() (string, error) {
		return backend.Driver(), nil
	})
	if err != nil {
		return nil, err
	}
	if !driverNamePattern.MatchString(driver) {
		return nil, fmt.Errorf("database driver %q is invalid", driver)
	}
	return &control{backend: backend, access: &access{backend: backend}, driver: driver}, nil
}

func (*control) databaseControl() {}

func (control *control) Driver() string {
	if control == nil || isNil(control.backend) {
		return ""
	}
	return control.driver
}

func (control *control) Access() database.Access {
	if control == nil {
		return nil
	}
	return control.access
}

func (control *control) Executor(ctx context.Context) (database.Executor, error) {
	if ctx == nil {
		return nil, fmt.Errorf("database context is required")
	}
	if control == nil || isNil(control.backend) {
		return nil, ErrControlUnavailable
	}
	executor, err := invokeDependency("resolve database executor", func() (database.Executor, error) {
		return control.backend.AdminExecutor(ctx)
	})
	if err != nil {
		return nil, err
	}
	if isNil(executor) {
		return nil, ErrControlUnavailable
	}
	return executor, nil
}

func (control *control) WithinTransaction(ctx context.Context, operation func(context.Context) error) error {
	return control.withinTransaction(ctx, operation, false)
}

func (err *dependencyError) Error() string {
	if err == nil || err.operation == "" {
		return "database dependency failed"
	}
	return err.operation + " failed"
}

func (err *dependencyError) Unwrap() error {
	if err == nil {
		return nil
	}
	return safeerr.Opaque(err.cause)
}

func invokeDependency[T any](operation string, callback func() (T, error)) (result T, err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			result = *new(T)
			err = &dependencyError{operation: operation, cause: database.ErrDependencyPanic}
		}
	}()
	result, err = callback()
	returned = true
	if err == nil {
		return result, nil
	}
	return result, &dependencyError{operation: operation, cause: err}
}

func invokeDependencyError(operation string, callback func() error) error {
	_, err := invokeDependency(operation, func() (struct{}, error) {
		return struct{}{}, callback()
	})
	return err
}

// ContainsDependencyPanic reports whether err contains the database dependency panic
// sentinel without dispatching caller-defined error methods.
func ContainsDependencyPanic(err error) bool {
	return safeerr.Is(err, database.ErrDependencyPanic)
}

func isNil(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
