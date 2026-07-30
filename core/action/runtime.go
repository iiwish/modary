package action

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"modary/core/audit"
	"modary/core/authz"
)

type Runtime struct {
	registry    *Registry
	authorizer  authz.Authorizer
	audit       audit.Hook
	plans       PlanStore
	idempotency IdempotencyStore
	tx          TransactionManager
	clock       func() time.Time
	planTTL     time.Duration
}

type RuntimeOptions struct {
	Registry     *Registry
	Authorizer   authz.Authorizer
	Audit        audit.Hook
	Plans        PlanStore
	Idempotency  IdempotencyStore
	Transactions TransactionManager
	Clock        func() time.Time
	PlanTTL      time.Duration
}

func NewRuntime(options RuntimeOptions) (*Runtime, error) {
	if options.Registry == nil {
		return nil, fmt.Errorf("action registry is required")
	}
	if options.Authorizer == nil {
		return nil, fmt.Errorf("authorizer is required")
	}
	if options.Audit == nil {
		options.Audit = audit.NopHook{}
	}
	if options.Plans == nil {
		options.Plans = NewMemoryPlanStore()
	}
	if options.Idempotency == nil {
		options.Idempotency = NewMemoryIdempotencyStore()
	}
	if options.Transactions == nil {
		options.Transactions = NopTransactionManager{}
	}
	if options.Clock == nil {
		options.Clock = time.Now
	}
	if options.PlanTTL <= 0 {
		options.PlanTTL = 5 * time.Minute
	}
	return &Runtime{
		registry:    options.Registry,
		authorizer:  options.Authorizer,
		audit:       options.Audit,
		plans:       options.Plans,
		idempotency: options.Idempotency,
		tx:          options.Transactions,
		clock:       options.Clock,
		planTTL:     options.PlanTTL,
	}, nil
}

func (r *Runtime) Preview(ctx context.Context, request Request) (Preview, error) {
	request = normalizeRequest(request)
	startedAt := r.clock().UTC()
	registered, err := r.resolveAndValidate(request)
	if err != nil {
		return Preview{}, r.fail(ctx, request, Descriptor{}, startedAt, err)
	}
	intent, err := r.authorize(ctx, request, registered.Descriptor, authz.PhaseIntent, authz.Impact{})
	if err != nil {
		return Preview{}, r.fail(ctx, request, registered.Descriptor, startedAt, err)
	}
	data, err := registered.Handler.Plan(ctx, request)
	if err != nil {
		return Preview{}, r.fail(ctx, request, registered.Descriptor, startedAt, WithRequest(err, request, registered.Descriptor.Permission))
	}
	impact, err := r.authorize(ctx, request, registered.Descriptor, authz.PhaseImpact, data.Impact)
	if err != nil {
		return Preview{}, r.fail(ctx, request, registered.Descriptor, startedAt, err)
	}
	if impact.Fingerprint == "" {
		impact.Fingerprint = intent.Fingerprint
	}
	createdAt := r.clock().UTC()
	input, inputHash, err := canonicalInput(request.Input)
	if err != nil {
		return Preview{}, r.fail(ctx, request, registered.Descriptor, startedAt, WithRequest(err, request, registered.Descriptor.Permission))
	}
	plan := Plan{
		ActionID:            request.ActionID,
		ActorID:             request.Actor.ID,
		WorkspaceID:         request.WorkspaceID,
		Input:               input,
		InputHash:           inputHash,
		Payload:             canonicalRaw(data.Payload),
		Summary:             canonicalRaw(data.Summary),
		Impact:              data.Impact,
		SnapshotHash:        data.SnapshotHash,
		DecisionFingerprint: impact.Fingerprint,
		CreatedAt:           createdAt,
		ExpiresAt:           createdAt.Add(r.planTTL),
	}
	plan.Hash = hashPlan(plan)
	if err := r.plans.Save(ctx, plan); err != nil {
		return Preview{}, r.fail(ctx, request, registered.Descriptor, startedAt, WithRequest(err, request, registered.Descriptor.Permission))
	}
	request.PlanHash = plan.Hash
	event := r.event(request, registered.Descriptor, startedAt, "previewed", "", string(plan.Summary))
	if err := r.audit.Record(ctx, event); err != nil {
		return Preview{}, WithRequest(fmt.Errorf("record preview audit: %w", err), request, registered.Descriptor.Permission)
	}
	return Preview{PlanHash: plan.Hash, Summary: plan.Summary, Impact: plan.Impact, ExpiresAt: plan.ExpiresAt}, nil
}

func (r *Runtime) Execute(ctx context.Context, request Request) (Result, error) {
	request = normalizeRequest(request)
	startedAt := r.clock().UTC()
	registered, err := r.resolveAndValidate(request)
	if err != nil {
		return Result{}, r.fail(ctx, request, Descriptor{}, startedAt, err)
	}
	if _, err := r.authorize(ctx, request, registered.Descriptor, authz.PhaseIntent, authz.Impact{}); err != nil {
		return Result{}, r.fail(ctx, request, registered.Descriptor, startedAt, err)
	}

	plan, err := r.resolveExecutionPlan(ctx, request, registered)
	if err != nil {
		return Result{}, r.fail(ctx, request, registered.Descriptor, startedAt, err)
	}
	request.PlanHash = plan.Hash
	decision, err := r.authorize(ctx, request, registered.Descriptor, authz.PhaseImpact, plan.Impact)
	if err != nil {
		return Result{}, r.fail(ctx, request, registered.Descriptor, startedAt, err)
	}
	if plan.DecisionFingerprint != "" && decision.Fingerprint != plan.DecisionFingerprint {
		err := WithRequest(NewError(CodePlanStale, "authorization or grant changed after preview"), request, registered.Descriptor.Permission)
		return Result{}, r.fail(ctx, request, registered.Descriptor, startedAt, err)
	}
	if registered.Descriptor.RequiresIdempotency && request.IdempotencyKey == "" {
		err := WithRequest(NewError(CodeIdempotencyRequired, "idempotency_key is required"), request, registered.Descriptor.Permission)
		return Result{}, r.fail(ctx, request, registered.Descriptor, startedAt, err)
	}

	record := IdempotencyRecord{
		WorkspaceID: request.WorkspaceID,
		ActorID:     request.Actor.ID,
		ActionID:    request.ActionID,
		Key:         request.IdempotencyKey,
		InputHash:   plan.InputHash,
	}
	var result Result
	var replay bool
	err = r.tx.WithinTransaction(ctx, func(txCtx context.Context) error {
		if request.IdempotencyKey != "" {
			existing, reserveErr := r.idempotency.Reserve(txCtx, record)
			if reserveErr != nil {
				return reserveErr
			}
			if existing != nil {
				if existing.InputHash != record.InputHash {
					return NewError(CodeIdempotencyConflict, "idempotency key was used with different input")
				}
				if existing.Status != "completed" {
					return NewError(CodeIdempotencyProgress, "an execution with this idempotency key is still in progress")
				}
				result = existing.Result
				replay = true
				return nil
			}
		}
		var executeErr error
		result, executeErr = registered.Handler.Execute(txCtx, plan)
		if executeErr != nil {
			return executeErr
		}
		if err := ValidateJSON(registered.Descriptor.OutputSchema, result.Data); err != nil {
			return fmt.Errorf("handler returned invalid output: %w", err)
		}
		if request.IdempotencyKey != "" {
			record.Result = result
			if err := r.idempotency.Complete(txCtx, record); err != nil {
				return err
			}
		}
		return r.audit.Record(txCtx, r.event(request, registered.Descriptor, startedAt, "allowed", "", result.Summary))
	})
	if err != nil {
		err = WithRequest(err, request, registered.Descriptor.Permission)
		return Result{}, r.fail(ctx, request, registered.Descriptor, startedAt, err)
	}
	if replay {
		_ = r.audit.Record(ctx, r.event(request, registered.Descriptor, startedAt, "idempotent_replay", "", result.Summary))
	}
	return result, nil
}

func (r *Runtime) resolveExecutionPlan(ctx context.Context, request Request, registered Registered) (Plan, error) {
	if registered.Descriptor.Preview == PreviewRequired {
		if request.PlanHash == "" {
			return Plan{}, WithRequest(NewError(CodePlanRequired, "a valid plan_hash from preview is required"), request, registered.Descriptor.Permission)
		}
		plan, err := r.plans.Get(ctx, request.PlanHash)
		if err != nil {
			return Plan{}, WithRequest(&Error{Code: CodePlanNotFound, Message: "preview plan was not found", Cause: err}, request, registered.Descriptor.Permission)
		}
		_, inputHash, err := canonicalInput(request.Input)
		if err != nil {
			return Plan{}, WithRequest(err, request, registered.Descriptor.Permission)
		}
		if plan.ActionID != request.ActionID || plan.ActorID != request.Actor.ID || plan.WorkspaceID != request.WorkspaceID || plan.InputHash != inputHash {
			return Plan{}, WithRequest(NewError(CodePlanStale, "action, actor, workspace, or input differs from preview"), request, registered.Descriptor.Permission)
		}
		if !r.clock().UTC().Before(plan.ExpiresAt) {
			return Plan{}, WithRequest(NewError(CodePlanStale, "preview plan expired"), request, registered.Descriptor.Permission)
		}
		return plan, nil
	}
	data, err := registered.Handler.Plan(ctx, request)
	if err != nil {
		return Plan{}, WithRequest(err, request, registered.Descriptor.Permission)
	}
	input, inputHash, err := canonicalInput(request.Input)
	if err != nil {
		return Plan{}, WithRequest(err, request, registered.Descriptor.Permission)
	}
	now := r.clock().UTC()
	plan := Plan{
		ActionID:     request.ActionID,
		ActorID:      request.Actor.ID,
		WorkspaceID:  request.WorkspaceID,
		Input:        input,
		InputHash:    inputHash,
		Payload:      canonicalRaw(data.Payload),
		Summary:      canonicalRaw(data.Summary),
		Impact:       data.Impact,
		SnapshotHash: data.SnapshotHash,
		CreatedAt:    now,
		ExpiresAt:    now.Add(r.planTTL),
	}
	plan.Hash = hashPlan(plan)
	return plan, nil
}

func (r *Runtime) resolveAndValidate(request Request) (Registered, error) {
	registered, ok := r.registry.Resolve(request.ActionID)
	if !ok {
		return Registered{}, WithRequest(NewError(CodeActionNotFound, "action is not registered"), request, "")
	}
	if len(registered.Descriptor.Channels) > 0 && !slices.Contains(registered.Descriptor.Channels, request.Channel) {
		return Registered{}, WithRequest(NewError(CodeAuthzDenied, "action is not available through this channel"), request, registered.Descriptor.Permission)
	}
	if request.Actor.ID == "" {
		return Registered{}, WithRequest(NewError(CodeAuthzDenied, "authenticated actor is required"), request, registered.Descriptor.Permission)
	}
	if request.WorkspaceID == "" || request.Actor.WorkspaceID != request.WorkspaceID {
		return Registered{}, WithRequest(NewError(CodeAuthzDenied, "actor workspace does not match request workspace"), request, registered.Descriptor.Permission)
	}
	if err := ValidateJSON(registered.Descriptor.InputSchema, request.Input); err != nil {
		return Registered{}, WithRequest(err, request, registered.Descriptor.Permission)
	}
	return registered, nil
}

func (r *Runtime) authorize(ctx context.Context, request Request, descriptor Descriptor, phase authz.Phase, impact authz.Impact) (authz.Decision, error) {
	decision, err := r.authorizer.Authorize(ctx, authz.Request{
		Actor:       request.Actor,
		ActionID:    request.ActionID,
		Permission:  descriptor.Permission,
		WorkspaceID: request.WorkspaceID,
		Phase:       phase,
		Impact:      impact,
	})
	if err != nil {
		return authz.Decision{}, WithRequest(err, request, descriptor.Permission)
	}
	if !decision.Allowed {
		code := decision.Code
		if code == "" {
			code = CodeAuthzDenied
		}
		message := decision.Reason
		if message == "" {
			message = "actor is not allowed to execute this action"
		}
		return decision, WithRequest(NewError(code, message), request, descriptor.Permission)
	}
	return decision, nil
}

func (r *Runtime) fail(ctx context.Context, request Request, descriptor Descriptor, startedAt time.Time, err error) error {
	wrapped := WithRequest(err, request, descriptor.Permission)
	code := ErrorCode(wrapped)
	decision := "denied"
	if code == CodeInternal {
		decision = "failed"
	}
	event := r.event(request, descriptor, startedAt, decision, code, "")
	event.Reason = wrapped.Error()
	_ = r.audit.Record(ctx, event)
	return wrapped
}

func (r *Runtime) event(request Request, descriptor Descriptor, startedAt time.Time, decision, errorCode, summary string) audit.Event {
	_, inputHash, _ := canonicalInput(request.Input)
	return audit.Event{
		RequestID:     request.RequestID,
		ActorID:       request.Actor.ID,
		ActorType:     request.Actor.Type,
		Channel:       request.Channel,
		ActionID:      request.ActionID,
		WorkspaceID:   request.WorkspaceID,
		InputHash:     inputHash,
		PlanHash:      request.PlanHash,
		Decision:      decision,
		ResultSummary: summary,
		ErrorCode:     errorCode,
		StartedAt:     startedAt,
		FinishedAt:    r.clock().UTC(),
	}
}

func normalizeRequest(request Request) Request {
	if request.RequestID == "" {
		var data [12]byte
		if _, err := rand.Read(data[:]); err == nil {
			request.RequestID = "req_" + hex.EncodeToString(data[:])
		} else {
			request.RequestID = fmt.Sprintf("req_%d", time.Now().UnixNano())
		}
	}
	if len(request.Input) == 0 {
		request.Input = json.RawMessage(`{}`)
	}
	return request
}

func canonicalInput(input json.RawMessage) (json.RawMessage, string, error) {
	var value any
	if err := json.Unmarshal(input, &value); err != nil {
		return nil, "", NewError(CodeValidationFailed, "input is not valid JSON")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	hash := sha256.Sum256(canonical)
	return canonical, "sha256:" + hex.EncodeToString(hash[:]), nil
}

func canonicalRaw(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	canonical, _, err := canonicalInput(value)
	if err != nil {
		return value
	}
	return canonical
}

func hashPlan(plan Plan) string {
	material := struct {
		ActionID            string          `json:"action_id"`
		ActorID             string          `json:"actor_id"`
		WorkspaceID         string          `json:"workspace_id"`
		InputHash           string          `json:"input_hash"`
		Payload             json.RawMessage `json:"payload"`
		Impact              authz.Impact    `json:"impact"`
		SnapshotHash        string          `json:"snapshot_hash"`
		DecisionFingerprint string          `json:"decision_fingerprint"`
	}{plan.ActionID, plan.ActorID, plan.WorkspaceID, plan.InputHash, plan.Payload, plan.Impact, plan.SnapshotHash, plan.DecisionFingerprint}
	data, _ := json.Marshal(material)
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:])
}

func IsCode(err error, code string) bool {
	var actionErr *Error
	return errors.As(err, &actionErr) && actionErr.Code == code
}
