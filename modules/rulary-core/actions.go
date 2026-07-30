package rulary_core

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"modary/core/action"
	"modary/core/authz"
)

type registeredAction struct {
	descriptor action.Descriptor
	handler    action.Handler
}

func actionRegistrations(store *Store) []registeredAction {
	return []registeredAction{
		{rulesetListDescriptor(), &rularyHandler{store: store, kind: "list"}},
		{rulesetGetDescriptor(), &rularyHandler{store: store, kind: "get"}},
		{rulesetCreateDescriptor(), &rularyHandler{store: store, kind: "create"}},
		{rulesetUpdateDescriptor(), &rularyHandler{store: store, kind: "update"}},
		{rulesetValidateDescriptor(), &rularyHandler{store: store, kind: "validate"}},
		{rulesetPreviewDescriptor(), &rularyHandler{store: store, kind: "preview"}},
		{rulesetPublishDescriptor(), &rularyHandler{store: store, kind: "publish"}},
		{runExecuteDescriptor(), &rularyHandler{store: store, kind: "run"}},
		{runGetDescriptor(), &rularyHandler{store: store, kind: "run-get"}},
	}
}

type rularyHandler struct {
	store *Store
	kind  string
}

type listInput struct {
	Limit int `json:"limit"`
}

type getInput struct {
	RuleSetID string `json:"ruleset_id"`
}

type createInput struct {
	Name string          `json:"name"`
	Spec json.RawMessage `json:"spec"`
}

type updateInput struct {
	RuleSetID string          `json:"ruleset_id"`
	Spec      json.RawMessage `json:"spec"`
}

type previewInput struct {
	RuleSetID string `json:"ruleset_id"`
	Limit     int    `json:"limit"`
}

type runInput struct {
	RuleSetVersionID string `json:"ruleset_version_id"`
	Source           struct {
		Table string `json:"table"`
	} `json:"source"`
	Target struct {
		Table string `json:"table"`
	} `json:"target"`
	Limit int `json:"limit"`
}

type runGetInput struct {
	RunID  string `json:"run_id"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

type updatePlan struct {
	RuleSetID    string          `json:"ruleset_id"`
	ExpectedHash string          `json:"expected_hash"`
	Spec         json.RawMessage `json:"spec"`
}

type validationPlan struct {
	RuleSetID string `json:"ruleset_id"`
	SpecHash  string `json:"spec_hash"`
}

type publishPlan struct {
	RuleSetID string `json:"ruleset_id"`
	SpecHash  string `json:"spec_hash"`
}

type runPlan struct {
	Input        runInput       `json:"input"`
	Version      RuleVersion    `json:"version"`
	Labels       []PlannedLabel `json:"labels"`
	SnapshotHash string         `json:"snapshot_hash"`
}

func (h *rularyHandler) Plan(ctx context.Context, request action.Request) (action.PlanData, error) {
	switch h.kind {
	case "list":
		return passthroughPlan(request.Input, `{"operation":"list rulesets"}`), nil
	case "get", "run-get":
		return passthroughPlan(request.Input, `{"operation":"read detail"}`), nil
	case "create":
		var input createInput
		if err := decode(request.Input, &input); err != nil {
			return action.PlanData{}, err
		}
		if _, err := ParseRuleSpec(input.Spec); err != nil {
			return action.PlanData{}, action.NewError(action.CodeValidationFailed, err.Error())
		}
		return passthroughPlan(request.Input, `{"operation":"create ruleset"}`), nil
	case "update":
		var input updateInput
		if err := decode(request.Input, &input); err != nil {
			return action.PlanData{}, err
		}
		if _, err := ParseRuleSpec(input.Spec); err != nil {
			return action.PlanData{}, action.NewError(action.CodeValidationFailed, err.Error())
		}
		ruleset, err := h.store.GetRuleSet(ctx, request.WorkspaceID, input.RuleSetID)
		if err != nil {
			return action.PlanData{}, mapNotFound(err, "ruleset not found")
		}
		return marshalPlan(updatePlan{RuleSetID: input.RuleSetID, ExpectedHash: ruleset.DraftHash, Spec: input.Spec}, map[string]any{"operation": "update draft", "ruleset_id": input.RuleSetID})
	case "validate":
		var input getInput
		if err := decode(request.Input, &input); err != nil {
			return action.PlanData{}, err
		}
		ruleset, err := h.store.GetRuleSet(ctx, request.WorkspaceID, input.RuleSetID)
		if err != nil {
			return action.PlanData{}, mapNotFound(err, "ruleset not found")
		}
		if _, err := ParseRuleSpec(ruleset.DraftSpec); err != nil {
			return action.PlanData{}, action.NewError(action.CodeValidationFailed, err.Error())
		}
		return marshalPlan(validationPlan{RuleSetID: ruleset.ID, SpecHash: ruleset.DraftHash}, map[string]any{"valid": true, "spec_hash": ruleset.DraftHash, "errors": []string{}})
	case "preview":
		var input previewInput
		if err := decode(request.Input, &input); err != nil {
			return action.PlanData{}, err
		}
		ruleset, err := h.store.GetRuleSet(ctx, request.WorkspaceID, input.RuleSetID)
		if err != nil {
			return action.PlanData{}, mapNotFound(err, "ruleset not found")
		}
		spec, err := ParseRuleSpec(ruleset.DraftSpec)
		if err != nil {
			return action.PlanData{}, action.NewError(action.CodeValidationFailed, err.Error())
		}
		labels, snapshot, err := h.buildLabels(ctx, spec, input.Limit)
		if err != nil {
			return action.PlanData{}, err
		}
		summary := labelSummary(labels, sampleLabels(labels, 20))
		payload, _ := json.Marshal(map[string]any{"results": labels})
		summaryJSON, _ := json.Marshal(summary)
		return action.PlanData{Payload: payload, Summary: summaryJSON, Impact: authz.Impact{Rows: len(labels)}, SnapshotHash: snapshot}, nil
	case "publish":
		var input getInput
		if err := decode(request.Input, &input); err != nil {
			return action.PlanData{}, err
		}
		ruleset, err := h.store.GetRuleSet(ctx, request.WorkspaceID, input.RuleSetID)
		if err != nil {
			return action.PlanData{}, mapNotFound(err, "ruleset not found")
		}
		if ruleset.ValidatedHash != ruleset.DraftHash {
			return action.PlanData{}, action.NewError(action.CodePreconditionFailed, "current draft must be validated before publish")
		}
		summary := map[string]any{
			"ruleset_id":          ruleset.ID,
			"draft_hash":          ruleset.DraftHash,
			"previous_version_id": ruleset.CurrentVersionID,
			"change":              map[bool]string{true: "new version", false: "first version"}[ruleset.CurrentVersionID != ""],
		}
		return marshalPlan(publishPlan{RuleSetID: ruleset.ID, SpecHash: ruleset.DraftHash}, summary)
	case "run":
		var input runInput
		if err := decode(request.Input, &input); err != nil {
			return action.PlanData{}, err
		}
		if input.Source.Table != SourceTable || input.Target.Table != TargetTable {
			return action.PlanData{}, action.NewError(action.CodeValidationFailed, "source or target table is not allowed by the published RuleSpec")
		}
		version, err := h.store.GetVersion(ctx, request.WorkspaceID, input.RuleSetVersionID)
		if err != nil {
			return action.PlanData{}, mapNotFound(err, "published RuleVersion not found")
		}
		spec, err := ParseRuleSpec(version.Spec)
		if err != nil {
			return action.PlanData{}, action.NewError(action.CodeValidationFailed, err.Error())
		}
		labels, snapshot, err := h.buildLabels(ctx, spec, input.Limit)
		if err != nil {
			return action.PlanData{}, err
		}
		plan := runPlan{Input: input, Version: version, Labels: labels, SnapshotHash: snapshot}
		payload, _ := json.Marshal(plan)
		summaryJSON, _ := json.Marshal(labelSummary(labels, sampleLabels(labels, 20)))
		return action.PlanData{Payload: payload, Summary: summaryJSON, Impact: authz.Impact{Rows: len(labels)}, SnapshotHash: snapshot}, nil
	default:
		return action.PlanData{}, fmt.Errorf("unknown Rulary handler %s", h.kind)
	}
}

func (h *rularyHandler) Execute(ctx context.Context, plan action.Plan) (action.Result, error) {
	switch h.kind {
	case "list":
		var input listInput
		_ = decode(plan.Payload, &input)
		items, err := h.store.ListRuleSets(ctx, plan.WorkspaceID, input.Limit)
		return result(map[string]any{"rulesets": items}, fmt.Sprintf("returned %d rulesets", len(items)), err)
	case "get":
		var input getInput
		if err := decode(plan.Payload, &input); err != nil {
			return action.Result{}, err
		}
		item, err := h.store.GetRuleSet(ctx, plan.WorkspaceID, input.RuleSetID)
		return result(map[string]any{"ruleset": item}, "returned ruleset", mapNotFound(err, "ruleset not found"))
	case "create":
		var input createInput
		if err := decode(plan.Payload, &input); err != nil {
			return action.Result{}, err
		}
		item, err := h.store.CreateRuleSet(ctx, plan.WorkspaceID, input.Name, input.Spec)
		return result(map[string]any{"ruleset": item}, "created ruleset", err)
	case "update":
		var input updatePlan
		if err := decode(plan.Payload, &input); err != nil {
			return action.Result{}, err
		}
		item, err := h.store.UpdateDraft(ctx, plan.WorkspaceID, input.RuleSetID, input.ExpectedHash, input.Spec)
		return result(map[string]any{"ruleset": item}, "updated ruleset draft", err)
	case "validate":
		var input validationPlan
		if err := decode(plan.Payload, &input); err != nil {
			return action.Result{}, err
		}
		if err := h.store.MarkValidated(ctx, plan.WorkspaceID, input.RuleSetID, input.SpecHash); err != nil {
			return action.Result{}, err
		}
		return result(map[string]any{"valid": true, "spec_hash": input.SpecHash, "errors": []string{}}, "validated ruleset draft", nil)
	case "preview":
		var payload map[string]any
		if err := json.Unmarshal(plan.Payload, &payload); err != nil {
			return action.Result{}, err
		}
		return result(payload, "previewed ruleset against source data", nil)
	case "publish":
		var input publishPlan
		if err := decode(plan.Payload, &input); err != nil {
			return action.Result{}, err
		}
		version, err := h.store.Publish(ctx, plan.WorkspaceID, input.RuleSetID, plan.ActorID, input.SpecHash)
		return result(map[string]any{"version": version}, "published immutable RuleVersion", err)
	case "run":
		var payload runPlan
		if err := decode(plan.Payload, &payload); err != nil {
			return action.Result{}, err
		}
		spec, err := ParseRuleSpec(payload.Version.Spec)
		if err != nil {
			return action.Result{}, action.NewError(action.CodeValidationFailed, err.Error())
		}
		labels, snapshot, err := h.buildLabels(ctx, spec, payload.Input.Limit)
		if err != nil {
			return action.Result{}, err
		}
		if snapshot != payload.SnapshotHash {
			return action.Result{}, action.NewError(action.CodePlanStale, "source rows changed after preview")
		}
		run, err := h.store.WriteRun(ctx, plan.WorkspaceID, payload.Version, labels)
		return result(map[string]any{"run": run}, fmt.Sprintf("processed %d rows and wrote %d results", run.MatchedRows, run.WrittenRows), err)
	case "run-get":
		var input runGetInput
		if err := decode(plan.Payload, &input); err != nil {
			return action.Result{}, err
		}
		run, err := h.store.GetRun(ctx, plan.WorkspaceID, input.RunID, input.Offset, input.Limit)
		return result(map[string]any{"run": run}, "returned run detail", mapNotFound(err, "run not found"))
	default:
		return action.Result{}, fmt.Errorf("unknown Rulary handler %s", h.kind)
	}
}

func (h *rularyHandler) buildLabels(ctx context.Context, spec RuleSpec, limit int) ([]PlannedLabel, string, error) {
	sources, err := h.store.LoadSources(ctx, limit)
	if err != nil {
		return nil, "", err
	}
	ids := make([]string, 0, len(sources))
	for _, source := range sources {
		ids = append(ids, source.CompanyID)
	}
	existing, err := h.store.ExistingLabels(ctx, ids)
	if err != nil {
		return nil, "", err
	}
	labels := make([]PlannedLabel, 0, len(sources))
	for _, source := range sources {
		item := PlannedLabel{CompanySource: source}
		if source.LicenseAddress == "" {
			item.Rejected = true
			item.Reason = "license address is empty"
		} else {
			item.Label = ExtractAddressLabel(source.LicenseAddress, spec.Operator.FilingMarker)
			previous, exists := existing[source.CompanyID]
			item.Changed = !exists || !reflect.DeepEqual(previous, item.Label)
		}
		labels = append(labels, item)
	}
	snapshotData, _ := json.Marshal(struct {
		SpecHash string          `json:"spec_hash"`
		Sources  []CompanySource `json:"sources"`
	}{mustRuleSpecHash(spec), sources})
	hash := sha256.Sum256(snapshotData)
	return labels, "sha256:" + hex.EncodeToString(hash[:]), nil
}

func labelSummary(labels []PlannedLabel, samples []PlannedLabel) map[string]any {
	matched, writable, rejected, unchanged := len(labels), 0, 0, 0
	for _, item := range labels {
		switch {
		case item.Rejected:
			rejected++
		case item.Changed:
			writable++
		default:
			unchanged++
		}
	}
	return map[string]any{
		"matched_rows": matched, "writable_rows": writable, "rejected_rows": rejected,
		"unchanged_rows": unchanged, "target_table": TargetTable, "sample_results": samples,
		"warnings": []string{},
	}
}

func sampleLabels(labels []PlannedLabel, limit int) []PlannedLabel {
	if len(labels) <= limit {
		return labels
	}
	return labels[:limit]
}

func mustRuleSpecHash(spec RuleSpec) string {
	data, _ := json.Marshal(spec)
	hash, _ := RuleSpecHash(data)
	return hash
}

func passthroughPlan(payload json.RawMessage, summary string) action.PlanData {
	return action.PlanData{Payload: payload, Summary: json.RawMessage(summary)}
}

func marshalPlan(payload, summary any) (action.PlanData, error) {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return action.PlanData{}, err
	}
	summaryJSON, err := json.Marshal(summary)
	if err != nil {
		return action.PlanData{}, err
	}
	return action.PlanData{Payload: payloadJSON, Summary: summaryJSON}, nil
}

func result(value any, summary string, err error) (action.Result, error) {
	if err != nil {
		return action.Result{}, err
	}
	data, marshalErr := json.Marshal(value)
	if marshalErr != nil {
		return action.Result{}, marshalErr
	}
	return action.Result{Data: data, Summary: summary}, nil
}

func decode(data []byte, target any) error {
	if err := json.Unmarshal(data, target); err != nil {
		return action.NewError(action.CodeValidationFailed, "invalid action input")
	}
	return nil
}

func mapNotFound(err error, message string) error {
	if errors.Is(err, sql.ErrNoRows) {
		return action.NewError(action.CodePreconditionFailed, message)
	}
	return err
}

func objectOutputSchema() json.RawMessage {
	return json.RawMessage(`{"$schema":"http://json-schema.org/draft-07/schema#","type":"object"}`)
}

func baseDescriptor(id, title, permission string, preview action.PreviewPolicy, channels []string) action.Descriptor {
	return action.Descriptor{ID: id, Title: title, Permission: permission, Preview: preview, AuditLevel: action.AuditDetailed, Channels: channels, OutputSchema: objectOutputSchema()}
}

func rulesetListDescriptor() action.Descriptor {
	d := baseDescriptor("rulary.ruleset.list", "List RuleSets", "rulary.ruleset.preview", action.PreviewNone, []string{"http", "cli"})
	d.InputSchema = action.ObjectSchema(`"limit":{"type":"integer","minimum":1,"maximum":200}`)
	return d
}

func rulesetGetDescriptor() action.Descriptor {
	d := baseDescriptor("rulary.ruleset.get", "Get RuleSet", "rulary.ruleset.preview", action.PreviewNone, []string{"http", "cli"})
	d.InputSchema = action.ObjectSchema(`"ruleset_id":{"type":"string","minLength":1}`, "ruleset_id")
	return d
}

func rulesetCreateDescriptor() action.Descriptor {
	d := baseDescriptor("rulary.ruleset.create", "Create RuleSet", "rulary.ruleset.create", action.PreviewNone, []string{"http", "cli"})
	d.InputSchema = action.ObjectSchema(`"name":{"type":"string","minLength":1,"maxLength":120},"spec":{"type":"object"}`, "name", "spec")
	d.RequiresIdempotency = true
	return d
}

func rulesetUpdateDescriptor() action.Descriptor {
	d := baseDescriptor("rulary.ruleset.update_draft", "Update RuleSet draft", "rulary.ruleset.edit", action.PreviewNone, []string{"http"})
	d.InputSchema = action.ObjectSchema(`"ruleset_id":{"type":"string","minLength":1},"spec":{"type":"object"}`, "ruleset_id", "spec")
	d.RequiresIdempotency = true
	return d
}

func rulesetValidateDescriptor() action.Descriptor {
	d := baseDescriptor("rulary.ruleset.validate", "Validate RuleSet", "rulary.ruleset.preview", action.PreviewNone, []string{"http", "cli", "mcp"})
	d.InputSchema = action.ObjectSchema(`"ruleset_id":{"type":"string","minLength":1}`, "ruleset_id")
	d.RequiresIdempotency = true
	return d
}

func rulesetPreviewDescriptor() action.Descriptor {
	d := baseDescriptor("rulary.ruleset.preview", "Preview RuleSet", "rulary.ruleset.preview", action.PreviewOptional, []string{"http", "cli", "mcp"})
	d.InputSchema = action.ObjectSchema(`"ruleset_id":{"type":"string","minLength":1},"limit":{"type":"integer","minimum":1,"maximum":1000}`, "ruleset_id")
	return d
}

func rulesetPublishDescriptor() action.Descriptor {
	d := baseDescriptor("rulary.ruleset.publish", "Publish RuleSet", "rulary.ruleset.publish", action.PreviewRequired, []string{"http"})
	d.InputSchema = action.ObjectSchema(`"ruleset_id":{"type":"string","minLength":1}`, "ruleset_id")
	d.RequiresIdempotency = true
	return d
}

func runExecuteDescriptor() action.Descriptor {
	d := baseDescriptor("rulary.run.execute", "Execute published RuleSet", "rulary.run.execute", action.PreviewRequired, []string{"http", "cli", "mcp"})
	d.InputSchema = action.ObjectSchema(
		`"ruleset_version_id":{"type":"string","minLength":1},"source":{"type":"object","properties":{"table":{"const":"company_license"}},"required":["table"],"additionalProperties":false},"target":{"type":"object","properties":{"table":{"const":"company_address_labels"}},"required":["table"],"additionalProperties":false},"limit":{"type":"integer","minimum":1,"maximum":1000}`,
		"ruleset_version_id", "source", "target", "limit",
	)
	d.RequiresIdempotency = true
	return d
}

func runGetDescriptor() action.Descriptor {
	d := baseDescriptor("rulary.run.get", "Get RuleRun", "rulary.run.execute", action.PreviewNone, []string{"http", "cli"})
	d.InputSchema = action.ObjectSchema(`"run_id":{"type":"string","minLength":1},"offset":{"type":"integer","minimum":0},"limit":{"type":"integer","minimum":1,"maximum":200}`, "run_id")
	return d
}
