package databasecontrol

import (
	"context"
	"errors"

	"github.com/iiwish/modary/database"
	"github.com/iiwish/modary/internal/callbackcontract"
	"github.com/iiwish/modary/internal/safeerr"
	"github.com/iiwish/modary/internal/transactionoutcome"
)

var errTransactionBackendContract = errors.New("database transaction backend contract violation")

type transactionOperationError struct {
	cause          error
	invalidContext bool
	panicked       bool
}

type transactionOutcomeError struct {
	cause      error
	diagnostic error
}

type transactionContractError struct {
	backend   error
	operation error
}

func (control *control) withinTransaction(ctx context.Context, operation func(context.Context) error, trustedOperation bool) error {
	if ctx == nil {
		return errors.New("transaction context is required")
	}
	if operation == nil {
		return errors.New("transaction operation is required")
	}
	if control == nil || isNil(control.backend) {
		return ErrControlUnavailable
	}

	var guard callbackcontract.ExactlyOnce
	var operationFailure *transactionOperationError
	var panicValue any
	guardedOperation := func(transactionContext context.Context) error {
		return guard.Invoke(func() (operationErr error) {
			returned := false
			defer func() {
				if !returned {
					panicValue = recover()
					operationFailure = &transactionOperationError{
						cause:    errTransactionBackendContract,
						panicked: true,
					}
					operationErr = operationFailure
				}
			}()
			if isNil(transactionContext) {
				operationFailure = &transactionOperationError{
					cause:          errTransactionBackendContract,
					invalidContext: true,
				}
				returned = true
				return operationFailure
			}
			operationErr = operation(transactionContext)
			if operationErr == nil {
				returned = true
				return nil
			}
			if !trustedOperation {
				operationErr = &dependencyError{operation: "database transaction operation", cause: operationErr}
			}
			operationFailure = &transactionOperationError{cause: operationErr}
			returned = true
			return operationFailure
		})
	}

	returnedErr, panicked := invokeTransactionBackend(func() error {
		return control.backend.WithinTransaction(ctx, guardedOperation)
	})
	var backendErr error
	if panicked {
		backendErr = &dependencyError{operation: "run database transaction", cause: database.ErrDependencyPanic}
	} else if returnedErr != nil {
		backendErr = &dependencyError{operation: "run database transaction", cause: returnedErr}
	}
	proof, hasProof := transactionoutcome.Root(returnedErr)
	snapshot := guard.CloseAndWait()

	switch snapshot.Outcome() {
	case callbackcontract.OutcomeNotInvoked:
		if backendErr == nil || hasProof {
			return newTransactionBackendContractError(nil, nil)
		}
		return backendErr
	case callbackcontract.OutcomeReturned:
		if operationFailure == nil {
			if snapshot.Result != nil {
				return newTransactionBackendContractError(backendErr, snapshot.Result)
			}
			if panicked {
				return newTransactionBackendContractError(backendErr, nil)
			}
			if returnedErr == nil {
				return nil
			}
			if !hasProof || proof.State != transactionoutcome.StateCommitFailed {
				return newTransactionBackendContractError(backendErr, nil)
			}
			return projectTransactionOutcome(proof)
		}
		if operationFailure.invalidContext ||
			!safeerr.Is(snapshot.Result, operationFailure) || panicked ||
			!hasProof || !safeerr.Is(proof.Operation, operationFailure) {
			return newTransactionBackendContractError(backendErr, operationFailure)
		}
		if proof.State != transactionoutcome.StateRollbackPending &&
			proof.State != transactionoutcome.StateRolledBack &&
			proof.State != transactionoutcome.StateRollbackFailed {
			return newTransactionBackendContractError(backendErr, operationFailure)
		}
		if operationFailure.panicked {
			panic(panicValue)
		}
		if trustedOperation && proof.State == transactionoutcome.StateRolledBack {
			return &transactionOutcomeError{cause: backendErr, diagnostic: operationFailure.cause}
		}
		return projectTransactionOutcome(proof)
	case callbackcontract.OutcomePanicked:
		return newTransactionBackendContractError(backendErr, operationFailure)
	default:
		return newTransactionBackendContractError(backendErr, operationFailure)
	}
}

func projectTransactionOutcome(proof transactionoutcome.Snapshot) error {
	switch proof.State {
	case transactionoutcome.StateRollbackPending:
		return transactionoutcome.RollbackPending(proof.Operation)
	case transactionoutcome.StateRolledBack:
		return transactionoutcome.RolledBack(proof.Operation)
	case transactionoutcome.StateRollbackFailed:
		return transactionoutcome.RollbackFailed(
			proof.Operation,
			&dependencyError{operation: "roll back database transaction", cause: proof.Completion},
		)
	case transactionoutcome.StateCommitFailed:
		return transactionoutcome.CommitFailed(
			&dependencyError{operation: "commit database transaction", cause: proof.Completion},
		)
	default:
		return newTransactionBackendContractError(nil, nil)
	}
}

func invokeTransactionBackend(callback func() error) (err error, panicked bool) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			panicked = true
		}
	}()
	err = callback()
	returned = true
	return err, false
}

func newTransactionBackendContractError(backend, operation error) error {
	return &transactionContractError{backend: backend, operation: operation}
}

func (*transactionOperationError) Error() string { return "database transaction operation failed" }

func (err *transactionOperationError) Unwrap() error {
	if err == nil {
		return nil
	}
	return safeerr.Opaque(err.cause)
}

func (err *transactionOutcomeError) Error() string {
	if err != nil && err.diagnostic != nil {
		return err.diagnostic.Error()
	}
	return "database transaction operation failed"
}

func (err *transactionOutcomeError) Unwrap() error {
	if err == nil {
		return nil
	}
	return safeerr.Opaque(err.cause)
}

func (*transactionContractError) Error() string {
	return "database transaction backend contract violation"
}

func (err *transactionContractError) Unwrap() []error {
	if err == nil {
		return nil
	}
	causes := []error{errTransactionBackendContract}
	if err.backend != nil {
		causes = append(causes, safeerr.Opaque(err.backend))
	}
	if err.operation != nil {
		causes = append(causes, safeerr.Opaque(err.operation))
	}
	return causes
}
