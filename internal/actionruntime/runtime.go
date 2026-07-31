package actionruntime

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/audit"
	"github.com/iiwish/modary/authz"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/internal/actionpersistence"
	"github.com/iiwish/modary/internal/jsonvalue"
	"github.com/iiwish/modary/internal/runtimecontrol"
	"github.com/iiwish/modary/internal/safeerr"
	"github.com/iiwish/modary/scope"
)

var (
	actionIdentifierPattern         = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}(?:\.[a-z][a-z0-9_-]{0,62})*$`)
	errImpactResourceCountExceeded  = fmt.Errorf("resource count exceeds %d", audit.MaxResources)
	errResultReferenceCountExceeded = fmt.Errorf("result reference count exceeds %d", audit.MaxReferences)
)

const (
	maxRequestIDRunes = 128
	maxChannelRunes   = 64
)

// Engine is the internal governed execution boundary for registered Actions. It
// applies validation, authorization, Preview binding, idempotency, transactions,
// and audit semantics independently of the calling channel.
type Engine struct {
	registry     *Registry
	authorizer   authz.Authorizer
	audit        audit.Hook
	plans        actionpersistence.PlanStore
	idempotency  actionpersistence.IdempotencyStore
	tx           runtimecontrol.TransactionManager
	clock        func() time.Time
	planTTL      time.Duration
	auditTimeout time.Duration
	auditFailure func(context.Context, error, audit.Event)
}

// Options supplies the required governance services and optional timing
// policy used by a Runtime. Zero durations select conservative defaults.
type Options struct {
	Authorizer   authz.Authorizer
	Audit        audit.Hook
	Plans        actionpersistence.PlanStore
	Idempotency  actionpersistence.IdempotencyStore
	Transactions runtimecontrol.TransactionManager
	Clock        func() time.Time
	PlanTTL      time.Duration
	AuditTimeout time.Duration
	AuditFailure func(context.Context, error, audit.Event)
}

// New validates its dependencies and constructs a Runtime for trusted
// framework assembly without executing an Action.
func New(registry *Registry, options Options) (*Engine, error) {
	if registry == nil {
		return nil, fmt.Errorf("action registry is required")
	}
	if !registry.available() {
		return nil, fmt.Errorf("action registry is unavailable")
	}
	if isNilDependency(options.Authorizer) {
		return nil, fmt.Errorf("authorizer is required")
	}
	if isNilDependency(options.Audit) {
		return nil, fmt.Errorf("audit hook is required")
	}
	if isNilDependency(options.Plans) {
		return nil, fmt.Errorf("plan store is required")
	}
	if isNilDependency(options.Idempotency) {
		return nil, fmt.Errorf("idempotency store is required")
	}
	if isNilDependency(options.Transactions) {
		return nil, fmt.Errorf("transaction manager is required")
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.PlanTTL < 0 {
		return nil, fmt.Errorf("plan TTL cannot be negative")
	}
	if options.PlanTTL == 0 {
		options.PlanTTL = 5 * time.Minute
	}
	if options.AuditTimeout < 0 {
		return nil, fmt.Errorf("audit timeout cannot be negative")
	}
	if options.AuditTimeout == 0 {
		options.AuditTimeout = 2 * time.Second
	}
	if options.AuditFailure == nil {
		options.AuditFailure = func(_ context.Context, _ error, event audit.Event) {
			slog.Error("record action audit failed", "request_id", event.RequestID, "action_id", event.ActionID, "decision", event.Decision)
		}
	}
	return &Engine{
		registry:     registry,
		authorizer:   options.Authorizer,
		audit:        options.Audit,
		plans:        options.Plans,
		idempotency:  options.Idempotency,
		tx:           options.Transactions,
		clock:        options.Clock,
		planTTL:      options.PlanTTL,
		auditTimeout: options.AuditTimeout,
		auditFailure: options.AuditFailure,
	}, nil
}

func isNilDependency(value any) bool {
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

// Preview validates, authorizes, plans, and durably audits a caller-visible plan.
func (r *Engine) Preview(ctx context.Context, request action.Request) (preview action.Preview, returnErr error) {
	defer func() { returnErr = finalizeRuntimeFailure(returnErr) }()
	if ctx == nil {
		return action.Preview{}, action.NewError(action.CodeValidationFailed, "context is required")
	}
	registered, executionCtx, release, found, revoked := r.registry.acquire(ctx, request.ActionID)
	if revoked {
		return action.Preview{}, action.NewError(action.CodeUnavailable, "action runtime is shutting down")
	}
	defer release()
	ctx = executionCtx
	request = normalizeRequest(request)
	startedAt, err := r.currentTime()
	if err != nil {
		return action.Preview{}, action.WithRequest(err, request, "")
	}
	if err := preflightActionJSONLimit(request); err != nil {
		request.Input = nil
		if !found {
			registered = registration{}
		}
		return action.Preview{}, r.fail(ctx, request, registered, startedAt, err)
	}
	request = cloneRequest(request)
	err = r.validateAcquired(&request, registered, found)
	if err != nil {
		return action.Preview{}, r.fail(ctx, request, registration{}, startedAt, err)
	}
	if registered.Descriptor.Preview == action.PreviewNone {
		err := action.WithRequest(action.NewError(action.CodePreconditionFailed, "action does not support preview"), request, registered.Descriptor.Permission)
		return action.Preview{}, r.fail(ctx, request, registered, startedAt, err)
	}
	intent, err := r.authorize(ctx, request, registered.Descriptor, authz.PhaseIntent, authz.Impact{})
	if err != nil {
		return action.Preview{}, r.fail(ctx, request, registered, startedAt, err)
	}
	data, err := invokeCallback("plan action "+registered.Descriptor.ID, func() (action.PlanData, error) {
		return registered.Handler.Plan(ctx, cloneRequest(request))
	})
	if err != nil {
		err = normalizeHandlerFailure(err, registered.Descriptor)
		return action.Preview{}, r.fail(ctx, request, registered, startedAt, action.WithRequest(err, request, registered.Descriptor.Permission))
	}
	if err := validateActionJSONSize(data.Payload, "plan payload"); err != nil {
		err := action.WithRequest(&action.Error{Code: action.CodeInternal, Message: "handler returned invalid plan payload", Cause: err}, request, registered.Descriptor.Permission)
		return action.Preview{}, r.fail(ctx, request, registered, startedAt, err)
	}
	if err := validateActionJSONSize(data.Summary, "preview summary"); err != nil {
		err := action.WithRequest(&action.Error{Code: action.CodeInternal, Message: "handler returned invalid preview summary", Cause: err}, request, registered.Descriptor.Permission)
		return action.Preview{}, r.fail(ctx, request, registered, startedAt, err)
	}
	if cause := validateImpactCollectionSize(data.Impact); cause != nil {
		err = action.WithRequest(&action.Error{Code: action.CodeInternal, Message: "handler returned invalid impact", Cause: cause}, request, registered.Descriptor.Permission)
		return action.Preview{}, r.fail(ctx, request, registered, startedAt, err)
	}
	data = clonePlanData(data)
	if err := registered.validatePreview(data.Summary); err != nil {
		err := action.WithRequest(&action.Error{Code: action.CodeInternal, Message: "handler returned invalid preview summary", Cause: err}, request, registered.Descriptor.Permission)
		return action.Preview{}, r.fail(ctx, request, registered, startedAt, err)
	}
	payload, err := canonicalHandlerJSON(data.Payload, "plan payload")
	if err != nil {
		return action.Preview{}, r.fail(ctx, request, registered, startedAt, action.WithRequest(err, request, registered.Descriptor.Permission))
	}
	previewSummary, err := canonicalHandlerJSON(data.Summary, "preview summary")
	if err != nil {
		return action.Preview{}, r.fail(ctx, request, registered, startedAt, action.WithRequest(err, request, registered.Descriptor.Permission))
	}
	if err := validateImpact(data.Impact); err != nil {
		err := action.WithRequest(&action.Error{Code: action.CodeInternal, Message: "handler returned invalid impact", Cause: err}, request, registered.Descriptor.Permission)
		return action.Preview{}, r.fail(ctx, request, registered, startedAt, err)
	}
	if err := action.ValidateSnapshotHash(data.SnapshotHash); err != nil {
		err := action.WithRequest(&action.Error{Code: action.CodeInternal, Message: "handler returned invalid snapshot hash", Cause: err}, request, registered.Descriptor.Permission)
		return action.Preview{}, r.fail(ctx, request, registered, startedAt, err)
	}
	_, err = r.authorizeImpact(ctx, request, registered.Descriptor, intent, data.Impact, "")
	if err != nil {
		return action.Preview{}, r.fail(ctx, request, registered, startedAt, err)
	}
	_, inputHash, err := canonicalInput(request.Input)
	if err != nil {
		return action.Preview{}, r.fail(ctx, request, registered, startedAt, action.WithRequest(err, request, registered.Descriptor.Permission))
	}
	var (
		plan      action.Plan
		createdAt time.Time
	)
	err = r.withinTransaction(ctx, func(txCtx context.Context) error {
		transactionIntent, err := r.authorize(txCtx, request, registered.Descriptor, authz.PhaseIntent, authz.Impact{})
		if err != nil {
			return err
		}
		transactionImpact, err := r.authorizeImpact(txCtx, request, registered.Descriptor, transactionIntent, data.Impact, "")
		if err != nil {
			return err
		}
		createdAt, err = r.currentTime()
		if err != nil {
			return err
		}
		deleted, err := invokeDependencyCallback("clean expired preview plans", func() (int64, error) {
			return r.plans.DeleteExpired(txCtx, createdAt)
		})
		if err != nil {
			return err
		}
		if err := validateDeletedPlanCount(deleted); err != nil {
			return err
		}
		plan = action.Plan{
			ActionID:            request.ActionID,
			ActionVersion:       registered.Descriptor.Version,
			ContractHash:        registered.ContractHash,
			ActorID:             request.Actor.ID,
			ActorType:           request.Actor.Type,
			Channel:             request.Channel,
			Scope:               request.Scope,
			InputHash:           inputHash,
			Payload:             payload,
			Impact:              cloneImpact(data.Impact),
			SnapshotHash:        data.SnapshotHash,
			DecisionFingerprint: transactionImpact.Fingerprint,
			CreatedAt:           createdAt,
			ExpiresAt:           createdAt.Add(r.planTTL),
		}
		plan.Hash, err = hashPlan(plan)
		if err != nil {
			return err
		}
		if err := actionpersistence.ValidatePlanRecord(plan); err != nil {
			return &action.Error{Code: action.CodeInternal, Message: "runtime produced an invalid preview plan", Cause: err}
		}
		if err := invokeDependencyErrorCallback("save preview plan", func() error {
			return r.plans.Save(txCtx, clonePlan(plan))
		}); err != nil {
			return err
		}
		eventRequest := request
		eventRequest.PlanHash = plan.Hash
		event := r.event(eventRequest, registered, startedAt, "previewed", "", fmt.Sprintf("previewed %d rows", data.Impact.Rows), data.Impact, nil)
		return r.recordRequired(txCtx, event)
	})
	if err != nil {
		r.reportRequiredAuditFailure(err)
		return action.Preview{}, r.fail(ctx, request, registered, startedAt, action.WithRequest(err, request, registered.Descriptor.Permission))
	}
	request.PlanHash = plan.Hash
	return action.Preview{PlanHash: plan.Hash, Summary: previewSummary, Impact: cloneImpact(plan.Impact), ExpiresAt: plan.ExpiresAt}, nil
}

// Execute runs one governed Action request and returns its validated result.
func (r *Engine) Execute(ctx context.Context, request action.Request) (_ action.Result, returnErr error) {
	defer func() { returnErr = finalizeRuntimeFailure(returnErr) }()
	if ctx == nil {
		return action.Result{}, action.NewError(action.CodeValidationFailed, "context is required")
	}
	registered, executionCtx, release, found, revoked := r.registry.acquire(ctx, request.ActionID)
	if revoked {
		return action.Result{}, action.NewError(action.CodeUnavailable, "action runtime is shutting down")
	}
	defer release()
	ctx = executionCtx
	request = normalizeRequest(request)
	startedAt, err := r.currentTime()
	if err != nil {
		return action.Result{}, action.WithRequest(err, request, "")
	}
	if err := preflightActionJSONLimit(request); err != nil {
		request.Input = nil
		if !found {
			registered = registration{}
		}
		return action.Result{}, r.fail(ctx, request, registered, startedAt, err)
	}
	request = cloneRequest(request)
	err = r.validateAcquired(&request, registered, found)
	if err != nil {
		return action.Result{}, r.fail(ctx, request, registration{}, startedAt, err)
	}
	intent, err := r.authorize(ctx, request, registered.Descriptor, authz.PhaseIntent, authz.Impact{})
	if err != nil {
		return action.Result{}, r.fail(ctx, request, registered, startedAt, err)
	}
	if registered.Descriptor.RequiresIdempotency && request.IdempotencyKey == "" {
		err := action.WithRequest(action.NewError(action.CodeIdempotencyRequired, "idempotency_key is required"), request, registered.Descriptor.Permission)
		return action.Result{}, r.fail(ctx, request, registered, startedAt, err)
	}
	if err := validateExecutionPlanPolicy(registered.Descriptor, request.PlanHash); err != nil {
		return action.Result{}, r.fail(ctx, request, registered, startedAt, action.WithRequest(err, request, registered.Descriptor.Permission))
	}
	_, inputHash, err := canonicalInput(request.Input)
	if err != nil {
		return action.Result{}, r.fail(ctx, request, registered, startedAt, action.WithRequest(err, request, registered.Descriptor.Permission))
	}
	record := actionpersistence.IdempotencyRecord{
		Scope:         request.Scope,
		ActorID:       request.Actor.ID,
		ActorType:     request.Actor.Type,
		ActionID:      request.ActionID,
		ActionVersion: registered.Descriptor.Version,
		ContractHash:  registered.ContractHash,
		Channel:       request.Channel,
		Key:           request.IdempotencyKey,
		InputHash:     inputHash,
		PlanHash:      request.PlanHash,
		Status:        actionpersistence.IdempotencyRunning,
	}
	if request.IdempotencyKey != "" {
		existing, lookupErr := invokeDependencyCallback("look up idempotency record", func() (*actionpersistence.IdempotencyRecord, error) {
			return r.idempotency.Lookup(ctx, cloneIdempotencyRecord(record))
		})
		if lookupErr != nil {
			return action.Result{}, r.fail(ctx, request, registered, startedAt, action.WithRequest(lookupErr, request, registered.Descriptor.Permission))
		}
		if existing != nil {
			var result action.Result
			var existingRecord actionpersistence.IdempotencyRecord
			replayErr := r.withinTransaction(ctx, func(txCtx context.Context) error {
				current, err := invokeDependencyCallback("look up idempotency record", func() (*actionpersistence.IdempotencyRecord, error) {
					return r.idempotency.Lookup(txCtx, cloneIdempotencyRecord(record))
				})
				if err != nil {
					return err
				}
				if current == nil {
					return action.NewError(action.CodeIdempotencyProgress, "idempotency record changed while authorization was rechecked")
				}
				existingRecord, err = ownStoredIdempotencyRecord(current)
				if err != nil {
					return err
				}
				transactionIntent, err := r.authorize(txCtx, request, registered.Descriptor, authz.PhaseIntent, authz.Impact{})
				if err != nil {
					return err
				}
				result, err = r.completedResult(txCtx, request, registered, transactionIntent, record, existingRecord)
				return err
			})
			if replayErr != nil {
				return action.Result{}, r.fail(ctx, request, registered, startedAt, action.WithRequest(replayErr, request, registered.Descriptor.Permission))
			}
			request.PlanHash = existingRecord.PlanHash
			r.recordDetached(ctx, r.event(request, registered, startedAt, "idempotent_replay", "", result.Summary, existingRecord.Impact, result.References))
			return result, nil
		}
	}

	plan, err := r.resolveExecutionPlan(ctx, request, registered)
	if err != nil {
		return action.Result{}, r.fail(ctx, request, registered, startedAt, err)
	}
	internalPlan := plan.DecisionFingerprint == ""
	if !internalPlan {
		request.PlanHash = plan.Hash
	}
	decision, err := r.authorizeImpact(ctx, request, registered.Descriptor, intent, plan.Impact, plan.DecisionFingerprint)
	if err != nil {
		return action.Result{}, r.fail(ctx, request, registered, startedAt, err)
	}
	if internalPlan {
		if err := sealExecutionPlan(&plan, decision.Fingerprint); err != nil {
			return action.Result{}, r.fail(ctx, request, registered, startedAt, action.WithRequest(err, request, registered.Descriptor.Permission))
		}
		request.PlanHash = plan.Hash
	}
	if record.InputHash != plan.InputHash {
		err := action.WithRequest(action.NewError(action.CodePlanStale, "action input differs from preview"), request, registered.Descriptor.Permission)
		return action.Result{}, r.fail(ctx, request, registered, startedAt, err)
	}
	var result action.Result
	var replay bool
	var replayImpact authz.Impact
	var replayPlanHash string
	reservation := cloneIdempotencyRecord(record)
	reservation.Impact = cloneImpact(plan.Impact)
	reservation.DecisionFingerprint = decision.Fingerprint
	reservation.PlanHash = plan.Hash
	err = r.withinTransaction(ctx, func(txCtx context.Context) (txErr error) {
		transactionIntent, err := r.authorize(txCtx, request, registered.Descriptor, authz.PhaseIntent, authz.Impact{})
		if err != nil {
			return err
		}
		transactionDecision, err := r.authorizeImpact(txCtx, request, registered.Descriptor, transactionIntent, plan.Impact, plan.DecisionFingerprint)
		if err != nil {
			return err
		}
		reservation.DecisionFingerprint = transactionDecision.Fingerprint
		reserved := false
		defer func() {
			if txErr != nil && reserved {
				if abortErr := invokeDependencyErrorCallback("release idempotency reservation", func() error {
					return r.idempotency.Abort(txCtx, cloneIdempotencyRecord(reservation))
				}); abortErr != nil {
					txErr = errors.Join(txErr, abortErr)
				}
			}
		}()
		if request.IdempotencyKey != "" {
			existing, reserveErr := invokeDependencyCallback("reserve idempotency record", func() (*actionpersistence.IdempotencyRecord, error) {
				return r.idempotency.Reserve(txCtx, cloneIdempotencyRecord(reservation))
			})
			if reserveErr != nil {
				return reserveErr
			}
			if existing != nil {
				existingRecord, err := ownStoredIdempotencyRecord(existing)
				if err != nil {
					return err
				}
				var replayErr error
				result, replayErr = r.completedResult(txCtx, request, registered, transactionIntent, record, existingRecord)
				if replayErr != nil {
					return replayErr
				}
				replayImpact = cloneImpact(existingRecord.Impact)
				replayPlanHash = existingRecord.PlanHash
				replay = true
				return nil
			}
			reserved = true
		}
		var executeErr error
		result, executeErr = invokeCallback("execute action "+registered.Descriptor.ID, func() (action.Result, error) {
			return registered.Handler.Execute(txCtx, clonePlan(plan))
		})
		if executeErr != nil {
			return normalizeHandlerFailure(executeErr, registered.Descriptor)
		}
		if err := validateActionJSONSize(result.Data, "result data"); err != nil {
			return &action.Error{Code: action.CodeInternal, Message: "handler returned invalid output", Cause: err}
		}
		if err := validateResultCollectionSize(result); err != nil {
			return &action.Error{Code: action.CodeInternal, Message: "handler returned invalid result metadata", Cause: err}
		}
		result = cloneResult(result)
		if err := registered.validateOutput(result.Data); err != nil {
			return &action.Error{Code: action.CodeInternal, Message: "handler returned invalid output", Cause: err}
		}
		if err := validateResult(result); err != nil {
			return &action.Error{Code: action.CodeInternal, Message: "handler returned invalid result metadata", Cause: err}
		}
		if request.IdempotencyKey != "" {
			completed := cloneIdempotencyRecord(reservation)
			completed.Status = actionpersistence.IdempotencyCompleted
			completed.Result = cloneResult(result)
			if err := invokeDependencyErrorCallback("complete idempotency record", func() error {
				return r.idempotency.Complete(txCtx, completed)
			}); err != nil {
				return err
			}
		}
		return r.recordRequired(txCtx, r.event(request, registered, startedAt, "allowed", "", result.Summary, plan.Impact, result.References))
	})
	if err != nil {
		r.reportRequiredAuditFailure(err)
		err = action.WithRequest(err, request, registered.Descriptor.Permission)
		return action.Result{}, r.fail(ctx, request, registered, startedAt, err)
	}
	if replay {
		request.PlanHash = replayPlanHash
		r.recordDetached(ctx, r.event(request, registered, startedAt, "idempotent_replay", "", result.Summary, replayImpact, result.References))
	}
	return cloneResult(result), nil
}

func (r *Engine) resolveExecutionPlan(ctx context.Context, request action.Request, registered registration) (action.Plan, error) {
	if err := validateExecutionPlanPolicy(registered.Descriptor, request.PlanHash); err != nil {
		return action.Plan{}, action.WithRequest(err, request, registered.Descriptor.Permission)
	}
	if request.PlanHash != "" {
		return r.loadExecutionPlan(ctx, request, registered)
	}
	data, err := invokeCallback("plan action "+registered.Descriptor.ID, func() (action.PlanData, error) {
		return registered.Handler.Plan(ctx, cloneRequest(request))
	})
	if err != nil {
		err = normalizeHandlerFailure(err, registered.Descriptor)
		return action.Plan{}, action.WithRequest(err, request, registered.Descriptor.Permission)
	}
	if err := validateActionJSONSize(data.Payload, "plan payload"); err != nil {
		return action.Plan{}, action.WithRequest(&action.Error{Code: action.CodeInternal, Message: "handler returned invalid plan payload", Cause: err}, request, registered.Descriptor.Permission)
	}
	if err := validateActionJSONSize(data.Summary, "preview summary"); err != nil {
		return action.Plan{}, action.WithRequest(&action.Error{Code: action.CodeInternal, Message: "handler returned invalid preview summary", Cause: err}, request, registered.Descriptor.Permission)
	}
	if registered.Descriptor.Preview == action.PreviewNone {
		data.Summary = nil
	}
	if err := validateImpactCollectionSize(data.Impact); err != nil {
		return action.Plan{}, action.WithRequest(&action.Error{Code: action.CodeInternal, Message: "handler returned invalid impact", Cause: err}, request, registered.Descriptor.Permission)
	}
	data = clonePlanData(data)
	if registered.Descriptor.Preview != action.PreviewNone {
		if err := registered.validatePreview(data.Summary); err != nil {
			return action.Plan{}, action.WithRequest(&action.Error{Code: action.CodeInternal, Message: "handler returned invalid preview summary", Cause: err}, request, registered.Descriptor.Permission)
		}
		data.Summary = nil
	}
	if err := validateImpact(data.Impact); err != nil {
		return action.Plan{}, action.WithRequest(&action.Error{Code: action.CodeInternal, Message: "handler returned invalid impact", Cause: err}, request, registered.Descriptor.Permission)
	}
	if err := action.ValidateSnapshotHash(data.SnapshotHash); err != nil {
		return action.Plan{}, action.WithRequest(&action.Error{Code: action.CodeInternal, Message: "handler returned invalid snapshot hash", Cause: err}, request, registered.Descriptor.Permission)
	}
	_, inputHash, err := canonicalInput(request.Input)
	if err != nil {
		return action.Plan{}, action.WithRequest(err, request, registered.Descriptor.Permission)
	}
	now, err := r.currentTime()
	if err != nil {
		return action.Plan{}, action.WithRequest(err, request, registered.Descriptor.Permission)
	}
	payload, err := canonicalHandlerJSON(data.Payload, "plan payload")
	if err != nil {
		return action.Plan{}, action.WithRequest(err, request, registered.Descriptor.Permission)
	}
	plan := action.Plan{
		ActionID:      request.ActionID,
		ActionVersion: registered.Descriptor.Version,
		ContractHash:  registered.ContractHash,
		ActorID:       request.Actor.ID,
		ActorType:     request.Actor.Type,
		Channel:       request.Channel,
		Scope:         request.Scope,
		InputHash:     inputHash,
		Payload:       payload,
		Impact:        cloneImpact(data.Impact),
		SnapshotHash:  data.SnapshotHash,
		CreatedAt:     now,
		ExpiresAt:     now.Add(r.planTTL),
	}
	return plan, nil
}

func validateExecutionPlanPolicy(descriptor action.Descriptor, planHash string) error {
	if descriptor.Preview == action.PreviewRequired && planHash == "" {
		return action.NewError(action.CodePlanRequired, "a valid plan_hash from preview is required")
	}
	if descriptor.Preview == action.PreviewNone && planHash != "" {
		return action.NewError(action.CodePreconditionFailed, "action does not accept a preview plan")
	}
	return nil
}

func (r *Engine) loadExecutionPlan(ctx context.Context, request action.Request, registered registration) (action.Plan, error) {
	plan, err := invokeDependencyCallback("load preview plan", func() (action.Plan, error) {
		return r.plans.Get(ctx, request.PlanHash)
	})
	if err != nil {
		if safeErrorIs(err, actionpersistence.ErrPlanNotFound) {
			return action.Plan{}, action.WithRequest(&action.Error{Code: action.CodePlanNotFound, Message: "preview plan was not found", Cause: err}, request, registered.Descriptor.Permission)
		}
		return action.Plan{}, action.WithRequest(&action.Error{Code: action.CodeInternal, Message: "load preview plan", Cause: err}, request, registered.Descriptor.Permission)
	}
	if err := validateActionJSONSize(plan.Payload, "stored plan payload"); err != nil {
		return action.Plan{}, action.WithRequest(action.NewError(action.CodePlanStale, "preview plan contains invalid persisted fields"), request, registered.Descriptor.Permission)
	}
	if validateImpactCollectionSize(plan.Impact) != nil {
		return action.Plan{}, action.WithRequest(action.NewError(action.CodePlanStale, "preview plan contains invalid impact"), request, registered.Descriptor.Permission)
	}
	plan = clonePlan(plan)
	if err := actionpersistence.ValidatePlanRecord(plan); err != nil {
		return action.Plan{}, action.WithRequest(action.NewError(action.CodePlanStale, "preview plan contains invalid persisted fields"), request, registered.Descriptor.Permission)
	}
	computedHash, hashErr := hashPlan(plan)
	if hashErr != nil || plan.Hash != request.PlanHash || computedHash != plan.Hash {
		return action.Plan{}, action.WithRequest(action.NewError(action.CodePlanStale, "preview plan integrity check failed"), request, registered.Descriptor.Permission)
	}
	_, inputHash, err := canonicalInput(request.Input)
	if err != nil {
		return action.Plan{}, action.WithRequest(err, request, registered.Descriptor.Permission)
	}
	if plan.ActionID != request.ActionID || plan.ActionVersion != registered.Descriptor.Version || plan.ContractHash != registered.ContractHash ||
		plan.ActorID != request.Actor.ID || plan.ActorType != request.Actor.Type || plan.Channel != request.Channel ||
		plan.Scope != request.Scope || plan.InputHash != inputHash {
		return action.Plan{}, action.WithRequest(action.NewError(action.CodePlanStale, "action contract, actor, channel, execution scope, or input differs from preview"), request, registered.Descriptor.Permission)
	}
	now, err := r.currentTime()
	if err != nil {
		return action.Plan{}, action.WithRequest(err, request, registered.Descriptor.Permission)
	}
	if !now.Before(plan.ExpiresAt) {
		return action.Plan{}, action.WithRequest(action.NewError(action.CodePlanStale, "preview plan expired"), request, registered.Descriptor.Permission)
	}
	if err := validateImpact(plan.Impact); err != nil {
		return action.Plan{}, action.WithRequest(action.NewError(action.CodePlanStale, "preview plan contains invalid impact"), request, registered.Descriptor.Permission)
	}
	return plan, nil
}

func enforceConstraints(decision authz.Decision, impact authz.Impact) error {
	if decision.Constraints.MaxRows < 0 {
		return &action.Error{Code: action.CodeInternal, Message: "authorizer returned invalid row constraints"}
	}
	if decision.Constraints.MaxRows > 0 && impact.Rows > decision.Constraints.MaxRows {
		return action.NewError(action.CodeLimitExceeded, fmt.Sprintf("planned impact of %d rows exceeds the authorized limit of %d", impact.Rows, decision.Constraints.MaxRows))
	}
	return nil
}

func validateImpact(impact authz.Impact) error {
	if impact.Rows < 0 {
		return fmt.Errorf("row count cannot be negative")
	}
	if err := validateImpactCollectionSize(impact); err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(impact.Resources))
	for _, resource := range impact.Resources {
		if !utf8.ValidString(resource) || resource == "" || strings.TrimSpace(resource) != resource || utf8.RuneCountInString(resource) > audit.MaxResourceRunes || strings.ContainsFunc(resource, unicode.IsControl) {
			return fmt.Errorf("resource identifier is invalid")
		}
		if _, exists := seen[resource]; exists {
			return fmt.Errorf("resource identifier %q is duplicated", resource)
		}
		seen[resource] = struct{}{}
	}
	return nil
}

func (r *Engine) validateAcquired(request *action.Request, registered registration, found bool) error {
	if err := validateRequestEnvelope(*request); err != nil {
		message := err.Error()
		if !action.ValidIdentifier(request.ActionID) {
			message = "action id is invalid"
		}
		return action.WithRequest(action.NewError(action.CodeValidationFailed, message), *request, "")
	}
	if !found {
		return action.WithRequest(action.NewError(action.CodeActionNotFound, "action is not registered"), *request, "")
	}
	if len(registered.Descriptor.Channels) > 0 && !slices.Contains(registered.Descriptor.Channels, request.Channel) {
		return action.WithRequest(action.NewError(action.CodeAuthzDenied, "action is not available through this channel"), *request, registered.Descriptor.Permission)
	}
	if request.Actor.ID == "" {
		return action.WithRequest(action.NewError(action.CodeAuthzDenied, "authenticated actor is required"), *request, registered.Descriptor.Permission)
	}
	if err := request.Scope.Validate(); err != nil || request.Actor.Scope != request.Scope {
		return action.WithRequest(action.NewError(action.CodeAuthzDenied, "actor execution scope does not match request scope"), *request, registered.Descriptor.Permission)
	}
	canonical, _, err := canonicalInput(request.Input)
	if err != nil {
		return action.WithRequest(err, *request, registered.Descriptor.Permission)
	}
	request.Input = canonical
	if err := registered.validateInput(request.Input); err != nil {
		return action.WithRequest(err, *request, registered.Descriptor.Permission)
	}
	return nil
}

func validateRequestEnvelope(request action.Request) error {
	if err := validateDescriptorText("request id", request.RequestID, true, maxRequestIDRunes); err != nil {
		return err
	}
	if !action.ValidIdentifier(request.ActionID) {
		return fmt.Errorf("action id %q must match %s", request.ActionID, actionIdentifierPattern.String())
	}
	if err := validateDescriptorText("channel", string(request.Channel), true, maxChannelRunes); err != nil {
		return err
	}
	if err := identity.ValidateActor(request.Actor); err != nil {
		return err
	}
	if request.IdempotencyKey != "" {
		if err := action.ValidateIdempotencyKey(request.IdempotencyKey); err != nil {
			return err
		}
	}
	if request.PlanHash != "" && action.ValidatePlanHash(request.PlanHash) != nil {
		return fmt.Errorf("plan hash must be a lowercase SHA-256 digest")
	}
	return nil
}

func (r *Engine) authorize(ctx context.Context, request action.Request, descriptor action.Descriptor, phase authz.Phase, impact authz.Impact) (authz.Decision, error) {
	return r.authorizeWithFallback(ctx, request, descriptor, phase, impact, "")
}

func (r *Engine) authorizeWithFallback(
	ctx context.Context,
	request action.Request,
	descriptor action.Descriptor,
	phase authz.Phase,
	impact authz.Impact,
	fingerprintFallback string,
) (authz.Decision, error) {
	decision, err := invokeDependencyCallback("authorize action "+descriptor.ID, func() (authz.Decision, error) {
		return r.authorizer.Authorize(ctx, authz.Request{
			Actor:      request.Actor,
			ActionID:   request.ActionID,
			Permission: descriptor.Permission,
			Scope:      request.Scope,
			Phase:      phase,
			Impact:     cloneImpact(impact),
		})
	})
	if err != nil {
		return authz.Decision{}, action.WithRequest(err, request, descriptor.Permission)
	}
	decision, err = normalizeDecision(decision, descriptor, fingerprintFallback)
	if err != nil {
		return authz.Decision{}, action.WithRequest(&action.Error{Code: action.CodeInternal, Message: "authorizer returned an invalid decision", Cause: err}, request, descriptor.Permission)
	}
	if !decision.Allowed {
		kind, _ := descriptorErrorKind(descriptor, decision.Code)
		return decision, action.WithRequest(&action.Error{Code: decision.Code, Kind: kind, Message: decision.Reason}, request, descriptor.Permission)
	}
	return decision, nil
}

func normalizeDecision(decision authz.Decision, descriptor action.Descriptor, fingerprintFallback string) (authz.Decision, error) {
	permission := descriptor.Permission
	if decision.RequiredPermission == "" {
		decision.RequiredPermission = permission
	}
	if decision.RequiredPermission != permission || !action.ValidIdentifier(decision.RequiredPermission) {
		return authz.Decision{}, fmt.Errorf("required permission does not match the Action contract")
	}
	if decision.Constraints.MaxRows < 0 {
		return authz.Decision{}, fmt.Errorf("maximum row constraint cannot be negative")
	}
	if decision.Fingerprint == "" {
		decision.Fingerprint = fingerprintFallback
	}
	if err := validatePolicyToken("decision fingerprint", decision.Fingerprint, true, authz.MaxFingerprintRunes); err != nil {
		return authz.Decision{}, err
	}
	if err := validatePolicyToken("decision code", decision.Code, false, authz.MaxDecisionCodeRunes); err != nil {
		return authz.Decision{}, err
	}
	if err := validatePolicyText("decision reason", decision.Reason, false, authz.MaxDecisionReasonRunes); err != nil {
		return authz.Decision{}, err
	}
	if decision.Allowed {
		if decision.Code != "" {
			return authz.Decision{}, fmt.Errorf("an allowed decision cannot include an error code")
		}
		return decision, nil
	}
	if decision.Constraints.MaxRows != 0 {
		return authz.Decision{}, fmt.Errorf("a denied decision cannot grant row constraints")
	}
	if decision.Code == "" {
		decision.Code = action.CodeAuthzDenied
	}
	kind, declared := descriptorErrorKind(descriptor, decision.Code)
	if !declared || kind != action.ErrorKindDenied {
		return authz.Decision{}, fmt.Errorf("denied decision code is not declared as an authorization denial")
	}
	if decision.Reason == "" {
		decision.Reason = "actor is not allowed to execute this action"
	}
	return decision, nil
}

func validatePolicyToken(field, value string, required bool, maxRunes int) error {
	if err := validatePolicyText(field, value, required, maxRunes); err != nil {
		return err
	}
	if strings.ContainsFunc(value, unicode.IsSpace) {
		return fmt.Errorf("%s cannot contain whitespace", field)
	}
	return nil
}

func validatePolicyText(field, value string, required bool, maxRunes int) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s must be valid UTF-8", field)
	}
	if required && value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("%s cannot contain surrounding whitespace", field)
	}
	if utf8.RuneCountInString(value) > maxRunes {
		return fmt.Errorf("%s cannot exceed %d characters", field, maxRunes)
	}
	if strings.ContainsFunc(value, func(character rune) bool {
		return unicode.IsControl(character) || character == '\u2028' || character == '\u2029'
	}) {
		return fmt.Errorf("%s cannot contain control characters", field)
	}
	return nil
}

func (r *Engine) authorizeImpact(
	ctx context.Context,
	request action.Request,
	descriptor action.Descriptor,
	intent authz.Decision,
	impact authz.Impact,
	expectedFingerprint string,
) (authz.Decision, error) {
	decision, err := r.authorizeWithFallback(ctx, request, descriptor, authz.PhaseImpact, impact, intent.Fingerprint)
	if err != nil {
		return authz.Decision{}, err
	}
	if err := enforceConstraints(decision, impact); err != nil {
		return authz.Decision{}, action.WithRequest(err, request, descriptor.Permission)
	}
	if expectedFingerprint != "" && decision.Fingerprint != expectedFingerprint {
		return authz.Decision{}, action.WithRequest(action.NewError(action.CodePlanStale, "authorization or grant changed after preview"), request, descriptor.Permission)
	}
	return decision, nil
}

func (r *Engine) fail(ctx context.Context, request action.Request, registered registration, startedAt time.Time, err error) error {
	wrapped := finalizeRuntimeFailure(action.WithRequest(err, request, registered.Descriptor.Permission))
	code := action.ErrorCode(wrapped)
	kind := action.ErrorKindOf(wrapped)
	decision := "rejected"
	switch kind {
	case action.ErrorKindDenied:
		decision = "denied"
	case action.ErrorKindUnavailable, action.ErrorKindInternal:
		decision = "failed"
	}
	event := r.event(request, registered, startedAt, decision, code, "", authz.Impact{}, nil)
	event.ErrorKind = string(kind)
	if actionErr, ok := findActionError(wrapped); ok {
		event.Reason = actionErr.Message
	} else {
		event.Reason = "action execution failed"
	}
	r.recordDetached(ctx, audit.Normalize(event))
	return wrapped
}

func (r *Engine) event(request action.Request, registered registration, startedAt time.Time, decision, errorCode, summary string, impact authz.Impact, references []audit.Reference) audit.Event {
	_, inputHash, _ := canonicalInput(request.Input)
	finishedAt := startedAt
	if now, err := r.currentTime(); err == nil {
		finishedAt = now
	}
	event := audit.Event{
		RequestID:     request.RequestID,
		ActorID:       request.Actor.ID,
		ActorType:     request.Actor.Type,
		Channel:       string(request.Channel),
		ActionID:      request.ActionID,
		ActionVersion: registered.Descriptor.Version,
		ContractHash:  registered.ContractHash,
		Scope:         request.Scope,
		InputHash:     inputHash,
		PlanHash:      request.PlanHash,
		Decision:      decision,
		AuditLevel:    string(registered.Descriptor.AuditLevel),
		ResultSummary: summary,
		Impact:        &audit.Impact{Rows: impact.Rows, Resources: impact.Resources},
		ResultRefs:    references,
		ErrorCode:     errorCode,
		StartedAt:     startedAt,
		FinishedAt:    finishedAt,
	}
	return audit.Normalize(event)
}

func (r *Engine) completedResult(
	ctx context.Context,
	request action.Request,
	registered registration,
	intent authz.Decision,
	expected, existing actionpersistence.IdempotencyRecord,
) (action.Result, error) {
	if err := actionpersistence.ValidateStoredIdempotencyRecord(existing); err != nil {
		return action.Result{}, &action.Error{Code: action.CodeInternal, Message: "stored idempotency record is invalid", Cause: err}
	}
	if existing.Scope != expected.Scope || existing.ActorID != expected.ActorID || existing.ActionID != expected.ActionID || existing.Key != expected.Key {
		return action.Result{}, action.NewError(action.CodeIdempotencyConflict, "idempotency record binding does not match request")
	}
	if existing.ActorType != expected.ActorType || existing.Channel != expected.Channel {
		return action.Result{}, action.NewError(action.CodeIdempotencyConflict, "idempotency key belongs to a different actor type or channel")
	}
	if existing.ActionVersion != expected.ActionVersion || existing.ContractHash != expected.ContractHash {
		return action.Result{}, action.NewError(action.CodeIdempotencyConflict, "idempotency key belongs to a different action contract")
	}
	if expected.PlanHash != "" && existing.PlanHash != expected.PlanHash {
		return action.Result{}, action.NewError(action.CodeIdempotencyConflict, "idempotency key belongs to a different execution plan")
	}
	if existing.InputHash != expected.InputHash {
		return action.Result{}, action.NewError(action.CodeIdempotencyConflict, "idempotency key was used with different input")
	}
	if existing.Status != actionpersistence.IdempotencyCompleted {
		return action.Result{}, action.NewError(action.CodeIdempotencyProgress, "an execution with this idempotency key is still in progress")
	}
	if err := validateImpact(existing.Impact); err != nil {
		return action.Result{}, &action.Error{Code: action.CodeInternal, Message: "stored idempotent impact is invalid", Cause: err}
	}
	decision, err := r.authorizeWithFallback(ctx, request, registered.Descriptor, authz.PhaseImpact, existing.Impact, intent.Fingerprint)
	if err != nil {
		return action.Result{}, err
	}
	if err := enforceConstraints(decision, existing.Impact); err != nil {
		return action.Result{}, err
	}
	if decision.Fingerprint != existing.DecisionFingerprint {
		return action.Result{}, action.NewError(action.CodePlanStale, "authorization or grant changed since the original execution")
	}
	if err := registered.validateOutput(existing.Result.Data); err != nil {
		return action.Result{}, &action.Error{Code: action.CodeInternal, Message: "stored idempotent result is invalid", Cause: err}
	}
	if err := validateResult(existing.Result); err != nil {
		return action.Result{}, &action.Error{Code: action.CodeInternal, Message: "stored idempotent result metadata is invalid", Cause: err}
	}
	return cloneResult(existing.Result), nil
}

type dependencyError struct {
	operation string
	cause     error
}

// Error returns a stable dependency failure description.
func (err *dependencyError) Error() string {
	if err == nil || err.operation == "" {
		return "action dependency failed"
	}
	return err.operation + " failed"
}

// Unwrap exposes the dependency error through a safe opaque boundary.
func (err *dependencyError) Unwrap() error {
	if err == nil {
		return nil
	}
	return safeerr.Opaque(err.cause)
}

type requiredAuditError struct {
	cause error
	event audit.Event
}

// Error returns a stable required-audit failure description.
func (err *requiredAuditError) Error() string { return "record required action audit failed" }

// Unwrap exposes the audit-hook error through a safe opaque boundary.
func (err *requiredAuditError) Unwrap() error {
	if err == nil {
		return nil
	}
	return safeerr.Opaque(err.cause)
}

func (r *Engine) recordRequired(ctx context.Context, event audit.Event) error {
	if err := invokeDependencyErrorCallback("record required action audit", func() error {
		return r.audit.Record(ctx, cloneAuditEvent(event))
	}); err != nil {
		return &requiredAuditError{cause: err, event: cloneAuditEvent(event)}
	}
	return nil
}

func (r *Engine) reportRequiredAuditFailure(err error) {
	auditErr, ok := findRequiredAuditError(err)
	if !ok {
		return
	}
	r.reportAuditFailure(auditErr.cause, auditErr.event)
}

func findRequiredAuditError(err error) (auditErr *requiredAuditError, ok bool) {
	return safeerr.Find[*requiredAuditError](err)
}

func (r *Engine) recordDetached(ctx context.Context, event audit.Event) {
	auditCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.auditTimeout)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- invokeDependencyErrorCallback("record detached action audit", func() error {
			return r.audit.Record(auditCtx, cloneAuditEvent(event))
		})
	}()
	var err error
	select {
	case err = <-result:
	case <-auditCtx.Done():
		err = auditCtx.Err()
	}
	if err != nil {
		r.reportAuditFailure(err, event)
	}
}

func (r *Engine) reportAuditFailure(err error, event audit.Event) {
	failureEvent := cloneAuditEvent(event)
	reportCtx, cancel := context.WithTimeout(context.Background(), r.auditTimeout)
	defer cancel()
	done := make(chan struct{}, 1)
	go func() {
		defer func() {
			_ = recover()
			done <- struct{}{}
		}()
		r.auditFailure(reportCtx, err, failureEvent)
	}()
	select {
	case <-done:
	case <-reportCtx.Done():
	}
}

func cloneAuditEvent(event audit.Event) audit.Event {
	if event.Impact != nil {
		impact := *event.Impact
		impact.Resources = append([]string(nil), event.Impact.Resources...)
		event.Impact = &impact
	}
	event.ResultRefs = append([]audit.Reference(nil), event.ResultRefs...)
	return event
}

// CleanupExpiredPlans removes expired Preview plans while holding a Runtime lease.
func (r *Engine) CleanupExpiredPlans(ctx context.Context) (deletedCount int64, returnErr error) {
	defer func() { returnErr = finalizeRuntimeFailure(returnErr) }()
	if ctx == nil {
		return 0, action.NewError(action.CodeValidationFailed, "context is required")
	}
	executionCtx, release, revoked := r.registry.acquireLease(ctx)
	if revoked {
		return 0, action.NewError(action.CodeUnavailable, "action runtime is shutting down")
	}
	defer release()
	now, err := r.currentTime()
	if err != nil {
		return 0, &action.Error{Code: action.CodeInternal, Message: "read action runtime clock", Cause: err}
	}
	deleted, err := invokeDependencyCallback("clean expired preview plans", func() (int64, error) {
		return r.plans.DeleteExpired(executionCtx, now)
	})
	if err != nil {
		return 0, &action.Error{Code: action.CodeInternal, Message: "clean expired preview plans", Cause: err}
	}
	if err := validateDeletedPlanCount(deleted); err != nil {
		return 0, err
	}
	return deleted, nil
}

func validateDeletedPlanCount(deleted int64) error {
	if deleted < 0 {
		return &action.Error{Code: action.CodeInternal, Message: "plan store returned an invalid deletion count"}
	}
	return nil
}

func normalizeRequest(request action.Request) action.Request {
	if request.RequestID == "" {
		var data [12]byte
		if _, err := rand.Read(data[:]); err == nil {
			request.RequestID = "req_" + hex.EncodeToString(data[:])
		} else {
			request.RequestID = fmt.Sprintf("req_%d", time.Now().UnixNano())
		}
	}
	return request
}

func preflightActionJSONLimit(request action.Request) error {
	err := action.ValidateJSONDocument(request.Input)
	if !jsonvalue.IsLimit(err) {
		return nil
	}
	return action.WithRequest(
		action.NewError(action.CodeLimitExceeded, "input exceeds the Action JSON resource limits"),
		request,
		"",
	)
}

func validateActionJSONSize(value json.RawMessage, field string) error {
	if int64(len(value)) > action.MaxJSONDocumentBytes {
		return fmt.Errorf("%s exceeds %d bytes", field, action.MaxJSONDocumentBytes)
	}
	return nil
}

func ownStoredIdempotencyRecord(record *actionpersistence.IdempotencyRecord) (actionpersistence.IdempotencyRecord, error) {
	if record == nil {
		return actionpersistence.IdempotencyRecord{}, &action.Error{Code: action.CodeInternal, Message: "stored idempotency record is unavailable"}
	}
	if err := validateActionJSONSize(record.Result.Data, "stored idempotency result"); err != nil {
		return actionpersistence.IdempotencyRecord{}, &action.Error{Code: action.CodeInternal, Message: "stored idempotency record is invalid", Cause: err}
	}
	if err := validateImpactCollectionSize(record.Impact); err != nil {
		return actionpersistence.IdempotencyRecord{}, &action.Error{Code: action.CodeInternal, Message: "stored idempotency record is invalid", Cause: err}
	}
	if err := validateResultCollectionSize(record.Result); err != nil {
		return actionpersistence.IdempotencyRecord{}, &action.Error{Code: action.CodeInternal, Message: "stored idempotency record is invalid", Cause: err}
	}
	return cloneIdempotencyRecord(*record), nil
}

func canonicalInput(input json.RawMessage) (json.RawMessage, string, error) {
	value, err := decodeSingleJSON(input)
	if err != nil {
		if jsonvalue.IsLimit(err) {
			return nil, "", action.NewError(action.CodeLimitExceeded, "input exceeds the Action JSON resource limits")
		}
		return nil, "", action.NewError(action.CodeValidationFailed, "input is not valid JSON")
	}
	canonicalValue, err := canonicalizeJSONValue(value)
	if err != nil {
		return nil, "", &action.Error{Code: action.CodeValidationFailed, Message: "input cannot be canonicalized", Cause: err}
	}
	canonical, err := json.Marshal(canonicalValue)
	if err != nil {
		return nil, "", err
	}
	if err := action.ValidateJSONDocument(canonical); err != nil {
		if jsonvalue.IsLimit(err) {
			return nil, "", action.NewError(action.CodeLimitExceeded, "input exceeds the Action JSON resource limits after canonicalization")
		}
		return nil, "", &action.Error{Code: action.CodeInternal, Message: "canonical JSON is invalid", Cause: err}
	}
	hash := sha256.Sum256(canonical)
	return canonical, "sha256:" + hex.EncodeToString(hash[:]), nil
}

func canonicalizeJSONValue(value any) (any, error) {
	switch typed := value.(type) {
	case json.Number:
		normalized, err := normalizeJSONNumber(string(typed))
		if err != nil {
			return nil, err
		}
		return json.RawMessage(normalized), nil
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, item := range typed {
			canonical, err := canonicalizeJSONValue(item)
			if err != nil {
				return nil, err
			}
			result[key] = canonical
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for index, item := range typed {
			canonical, err := canonicalizeJSONValue(item)
			if err != nil {
				return nil, err
			}
			result[index] = canonical
		}
		return result, nil
	default:
		return value, nil
	}
}

func normalizeJSONNumber(value string) (string, error) {
	if len(value) > action.MaxJSONNumberBytes {
		return "", fmt.Errorf("JSON number exceeds %d bytes", action.MaxJSONNumberBytes)
	}
	negative := strings.HasPrefix(value, "-")
	if negative {
		value = value[1:]
	}
	mantissa := value
	exponent := new(big.Int)
	if index := strings.IndexAny(value, "eE"); index >= 0 {
		mantissa = value[:index]
		if _, ok := exponent.SetString(value[index+1:], 10); !ok {
			return "", fmt.Errorf("invalid JSON number exponent")
		}
	}
	fractionDigits := 0
	if index := strings.IndexByte(mantissa, '.'); index >= 0 {
		fractionDigits = len(mantissa) - index - 1
		mantissa = mantissa[:index] + mantissa[index+1:]
	}
	coefficient := strings.TrimLeft(mantissa, "0")
	if coefficient == "" {
		return "0", nil
	}
	exponent.Sub(exponent, big.NewInt(int64(fractionDigits)))
	trimmed := strings.TrimRight(coefficient, "0")
	exponent.Add(exponent, big.NewInt(int64(len(coefficient)-len(trimmed))))
	coefficient = trimmed
	if negative {
		coefficient = "-" + coefficient
	}
	if exponent.Sign() == 0 {
		return coefficient, nil
	}
	return renderCanonicalNumber(coefficient, exponent), nil
}

func renderCanonicalNumber(coefficient string, exponent *big.Int) string {
	scientific := coefficient + "e" + exponent.String()
	negative := strings.HasPrefix(coefficient, "-")
	digits := coefficient
	if negative {
		digits = coefficient[1:]
	}
	signBytes := 0
	if negative {
		signBytes = 1
	}

	if exponent.Sign() > 0 && exponent.IsInt64() {
		zeros := exponent.Int64()
		if zeros <= int64(action.MaxJSONNumberBytes-len(coefficient)) {
			return coefficient + strings.Repeat("0", int(zeros))
		}
		return scientific
	}
	if exponent.Sign() >= 0 {
		return scientific
	}

	places := new(big.Int).Neg(new(big.Int).Set(exponent))
	if !places.IsInt64() {
		return scientific
	}
	fractionPlaces := places.Int64()
	if fractionPlaces < int64(len(digits)) {
		point := len(digits) - int(fractionPlaces)
		if len(coefficient)+1 <= action.MaxJSONNumberBytes {
			prefix := ""
			if negative {
				prefix = "-"
			}
			return prefix + digits[:point] + "." + digits[point:]
		}
		return scientific
	}
	leadingZeros := fractionPlaces - int64(len(digits))
	outputBytes := int64(signBytes+2+len(digits)) + leadingZeros
	if outputBytes <= action.MaxJSONNumberBytes {
		prefix := "0."
		if negative {
			prefix = "-0."
		}
		return prefix + strings.Repeat("0", int(leadingZeros)) + digits
	}
	return scientific
}

func canonicalHandlerJSON(value json.RawMessage, field string) (json.RawMessage, error) {
	if len(value) == 0 {
		value = json.RawMessage(`{}`)
	}
	canonical, _, err := canonicalInput(value)
	if err != nil {
		return nil, &action.Error{Code: action.CodeInternal, Message: "handler returned invalid " + field, Cause: err}
	}
	return canonical, nil
}

func hashPlan(plan action.Plan) (string, error) {
	material := struct {
		ActionID            string          `json:"action_id"`
		ActionVersion       string          `json:"action_version"`
		ContractHash        string          `json:"contract_hash"`
		ActorID             string          `json:"actor_id"`
		ActorType           string          `json:"actor_type"`
		Channel             action.Channel  `json:"channel"`
		Scope               scope.Execution `json:"scope"`
		InputHash           string          `json:"input_hash"`
		Payload             json.RawMessage `json:"payload"`
		Impact              authz.Impact    `json:"impact"`
		SnapshotHash        string          `json:"snapshot_hash"`
		DecisionFingerprint string          `json:"decision_fingerprint"`
		CreatedAt           time.Time       `json:"created_at"`
		ExpiresAt           time.Time       `json:"expires_at"`
	}{plan.ActionID, plan.ActionVersion, plan.ContractHash, plan.ActorID, plan.ActorType, plan.Channel, plan.Scope, plan.InputHash, plan.Payload, plan.Impact, plan.SnapshotHash, plan.DecisionFingerprint, plan.CreatedAt, plan.ExpiresAt}
	data, err := json.Marshal(material)
	if err != nil {
		return "", fmt.Errorf("marshal Action plan hash material: %w", err)
	}
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}

func sealExecutionPlan(plan *action.Plan, decisionFingerprint string) error {
	if plan == nil || plan.Hash != "" || plan.DecisionFingerprint != "" {
		return &action.Error{Code: action.CodeInternal, Message: "internal execution plan is already sealed"}
	}
	if err := action.ValidateDecisionFingerprint(decisionFingerprint); err != nil {
		return &action.Error{Code: action.CodeInternal, Message: "authorizer returned an invalid decision fingerprint", Cause: err}
	}
	candidate := clonePlan(*plan)
	candidate.DecisionFingerprint = decisionFingerprint
	hash, err := hashPlan(candidate)
	if err != nil {
		return &action.Error{Code: action.CodeInternal, Message: "hash internal execution plan", Cause: err}
	}
	candidate.Hash = hash
	if err := actionpersistence.ValidatePlanRecord(candidate); err != nil {
		return &action.Error{Code: action.CodeInternal, Message: "internal execution plan is invalid", Cause: err}
	}
	*plan = candidate
	return nil
}

func (r *Engine) currentTime() (time.Time, error) {
	now, err := invokeDependencyCallback("read action runtime clock", func() (time.Time, error) {
		return r.clock().UTC(), nil
	})
	if err != nil {
		return time.Time{}, err
	}
	return now, nil
}

func validateResult(result action.Result) error {
	if err := validatePolicyText("result summary", result.Summary, false, audit.MaxSummaryRunes); err != nil {
		return err
	}
	if err := validateResultCollectionSize(result); err != nil {
		return err
	}
	seen := make(map[audit.Reference]struct{}, len(result.References))
	for _, reference := range result.References {
		if err := validatePolicyToken("result reference kind", reference.Kind, true, audit.MaxKindRunes); err != nil {
			return err
		}
		if err := validatePolicyToken("result reference id", reference.ID, true, audit.MaxIDRunes); err != nil {
			return err
		}
		if _, exists := seen[reference]; exists {
			return fmt.Errorf("result reference is duplicated")
		}
		seen[reference] = struct{}{}
	}
	return nil
}

func validateImpactCollectionSize(impact authz.Impact) error {
	if len(impact.Resources) > audit.MaxResources {
		return errImpactResourceCountExceeded
	}
	return nil
}

func validateResultCollectionSize(result action.Result) error {
	if len(result.References) > audit.MaxReferences {
		return errResultReferenceCountExceeded
	}
	return nil
}

func invokeCallback[T any](operation string, callback func() (T, error)) (result T, err error) {
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			result = *new(T)
			err = &action.CallbackPanicError{Operation: operation}
		}
	}()
	result, err = callback()
	returned = true
	return result, err
}

// invokeDependencyCallback contains dependency panics, discards partial results,
// and fixes the public classification at CodeInternal before an untrusted error
// graph can be inspected by action.WithRequest.
func invokeDependencyCallback[T any](operation string, callback func() (T, error)) (result T, err error) {
	result, err = invokeCallback(operation, callback)
	if err == nil {
		return result, nil
	}
	return *new(T), &action.Error{
		Code:    action.CodeInternal,
		Message: operation + " failed",
		Cause:   &dependencyError{operation: operation, cause: err},
	}
}

func invokeDependencyErrorCallback(operation string, callback func() error) error {
	_, err := invokeDependencyCallback(operation, func() (struct{}, error) {
		return struct{}{}, callback()
	})
	return err
}
