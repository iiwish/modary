package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"modary/core/action"
	"modary/core/config"
	"modary/internal/app"
)

func TestCLIActionRunPreviewsAndExecutesThroughRuntime(t *testing.T) {
	dataDir := t.TempDir()
	databasePath := filepath.Join(dataDir, "modary.db")
	t.Setenv("MODARY_DATA_DIR", dataDir)
	t.Setenv("MODARY_DATABASE_PATH", databasePath)
	t.Setenv("MODARY_DEMO_PASSWORD", "cli-test-password")
	t.Setenv("MODARY_AGENT_TOKEN", "cli-test-agent-token")

	cfg := config.FromEnvironment()
	application, err := app.Bootstrap(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := application.Identity.ResolveByID(context.Background(), "user_admin")
	if err != nil {
		t.Fatal(err)
	}
	spec := map[string]any{
		"schema_version": "rulary.ruleset.f0", "id": "company-address", "name": "Address labels",
		"source":   map[string]any{"table": "company_license", "primary_key": "company_id", "field": "license_address"},
		"operator": map[string]any{"type": "rulary.address.extract_v1", "filing_marker": "经营地址备案", "parenthetical_note_target": "address_note"},
		"output":   map[string]any{"table": "company_address_labels", "unique_key": "company_id"},
	}
	created := executeForCLITest(t, application, admin.ID, "rulary.ruleset.create", map[string]any{"name": "CLI flow", "spec": spec}, "cli-create", "")
	rulesetID := created["ruleset"].(map[string]any)["id"].(string)
	executeForCLITest(t, application, admin.ID, "rulary.ruleset.validate", map[string]any{"ruleset_id": rulesetID}, "cli-validate", "")
	publishInput := map[string]any{"ruleset_id": rulesetID}
	publishPlan := previewForCLITest(t, application, admin.ID, "rulary.ruleset.publish", publishInput)
	published := executeForCLITest(t, application, admin.ID, "rulary.ruleset.publish", publishInput, "cli-publish", publishPlan.PlanHash)
	versionID := published["version"].(map[string]any)["id"].(string)
	if err := application.Close(); err != nil {
		t.Fatal(err)
	}

	input := map[string]any{
		"ruleset_version_id": versionID,
		"source":             map[string]any{"table": "company_license"},
		"target":             map[string]any{"table": "company_address_labels"},
		"limit":              20,
	}
	inputJSON, _ := json.Marshal(input)
	inputPath := filepath.Join(dataDir, "run-input.json")
	if err := os.WriteFile(inputPath, inputJSON, 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := runAction([]string{"run", "rulary.run.execute", "--actor", admin.ID, "--input", inputPath, "--preview"}, &output); err != nil {
		t.Fatalf("CLI preview failed: %v", err)
	}
	var preview action.Preview
	if err := json.Unmarshal(output.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.PlanHash == "" || preview.Impact.Rows != 20 {
		t.Fatalf("CLI preview = %#v", preview)
	}

	output.Reset()
	if err := runAction([]string{"run", "rulary.run.execute", "--actor", admin.ID, "--input", inputPath, "--plan", preview.PlanHash, "--idempotency-key", "cli-run"}, &output); err != nil {
		t.Fatalf("CLI execute failed: %v", err)
	}
	var executed map[string]any
	if err := json.Unmarshal(output.Bytes(), &executed); err != nil {
		t.Fatal(err)
	}
	run := executed["run"].(map[string]any)
	if run["matched_rows"] != float64(20) || run["written_rows"] != float64(20) {
		t.Fatalf("CLI run = %#v", run)
	}

	verification, err := app.Bootstrap(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer verification.Close()
	var auditCount int
	if err := verification.DB.QueryRowContext(context.Background(), `
		SELECT COUNT(*) FROM modary_audit_log
		WHERE channel = 'cli' AND action_id = 'rulary.run.execute' AND decision = 'allowed'`).Scan(&auditCount); err != nil {
		t.Fatal(err)
	}
	if auditCount != 1 {
		t.Fatalf("CLI allowed audit count = %d", auditCount)
	}
}

func executeForCLITest(t *testing.T, application *app.Application, actorID, actionID string, input any, key, planHash string) map[string]any {
	t.Helper()
	actor, err := application.Identity.ResolveByID(context.Background(), actorID)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(input)
	result, err := application.Runtime.Execute(context.Background(), action.Request{
		Actor: actor, Channel: "http", ActionID: actionID, WorkspaceID: actor.WorkspaceID,
		Input: data, IdempotencyKey: key, PlanHash: planHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(result.Data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func previewForCLITest(t *testing.T, application *app.Application, actorID, actionID string, input any) action.Preview {
	t.Helper()
	actor, err := application.Identity.ResolveByID(context.Background(), actorID)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(input)
	preview, err := application.Runtime.Preview(context.Background(), action.Request{
		Actor: actor, Channel: "http", ActionID: actionID, WorkspaceID: actor.WorkspaceID, Input: data,
	})
	if err != nil {
		t.Fatal(err)
	}
	return preview
}
