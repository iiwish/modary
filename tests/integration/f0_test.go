package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"modary/core/action"
	"modary/core/config"
	"modary/core/identity"
	"modary/core/module"
	"modary/internal/app"
)

func TestRularyF0GovernedVerticalSlice(t *testing.T) {
	ctx := context.Background()
	application, cfg := bootstrap(t)
	defer application.Close()
	admin := resolve(t, application, "user_admin")

	spec := defaultSpec()
	created := execute(t, application, request(admin, "http", "rulary.ruleset.create", map[string]any{
		"name": "企业地址标签", "spec": spec,
	}, "", "create-integration"))
	rulesetID := nestedString(t, created, "ruleset", "id")

	validated := execute(t, application, request(admin, "http", "rulary.ruleset.validate", map[string]any{"ruleset_id": rulesetID}, "", "validate-integration"))
	if valid, _ := validated["valid"].(bool); !valid {
		t.Fatalf("validation result = %#v", validated)
	}

	rulesetPreview := preview(t, application, request(admin, "http", "rulary.ruleset.preview", map[string]any{"ruleset_id": rulesetID, "limit": 20}, "", ""))
	var previewSummary map[string]any
	if err := json.Unmarshal(rulesetPreview.Summary, &previewSummary); err != nil {
		t.Fatal(err)
	}
	samples := previewSummary["sample_results"].([]any)
	first := samples[0].(map[string]any)["label"].(map[string]any)
	if first["registered_address"] != "平顶山市卫东区建设路东段南4号院" ||
		first["business_address"] != "平顶山市黄河路与高新大道交叉口尼龙织造产业园内办公楼50号" {
		t.Fatalf("golden label = %#v", first)
	}

	publishRequest := request(admin, "http", "rulary.ruleset.publish", map[string]any{"ruleset_id": rulesetID}, "", "publish-integration")
	publishPlan := preview(t, application, publishRequest)
	publishRequest.PlanHash = publishPlan.PlanHash
	published := execute(t, application, publishRequest)
	versionID := nestedString(t, published, "version", "id")

	runInput := map[string]any{
		"ruleset_version_id": versionID,
		"source":             map[string]any{"table": "company_license"},
		"target":             map[string]any{"table": "company_address_labels"},
		"limit":              20,
	}
	runRequest := request(admin, "http", "rulary.run.execute", runInput, "", "run-integration")
	runPlan := preview(t, application, runRequest)
	runRequest.PlanHash = runPlan.PlanHash
	run := execute(t, application, runRequest)
	if nestedNumber(t, run, "run", "matched_rows") != 20 || nestedNumber(t, run, "run", "written_rows") != 20 {
		t.Fatalf("run = %#v", run)
	}
	replayed := execute(t, application, runRequest)
	if nestedString(t, replayed, "run", "id") != nestedString(t, run, "run", "id") {
		t.Fatal("idempotent replay created another run")
	}
	repeatRequest := request(admin, "http", "rulary.run.execute", runInput, "", "run-repeat-unchanged")
	repeatPlan := preview(t, application, repeatRequest)
	repeatRequest.PlanHash = repeatPlan.PlanHash
	repeatedRun := execute(t, application, repeatRequest)
	if nestedNumber(t, repeatedRun, "run", "written_rows") != 0 {
		t.Fatalf("unchanged repeat run = %#v", repeatedRun)
	}
	repeatedRunID := nestedString(t, repeatedRun, "run", "id")
	detail := execute(t, application, request(admin, "http", "rulary.run.get", map[string]any{"run_id": repeatedRunID, "offset": 0, "limit": 7}, "", ""))
	results, ok := detail["run"].(map[string]any)["results"].([]any)
	if !ok || len(results) != 7 {
		t.Fatalf("unchanged run lost inspection results: %#v", detail)
	}
	nextDetail := execute(t, application, request(admin, "http", "rulary.run.get", map[string]any{"run_id": repeatedRunID, "offset": 7, "limit": 7}, "", ""))
	nextResults := nextDetail["run"].(map[string]any)["results"].([]any)
	if len(nextResults) != 7 || results[0].(map[string]any)["company_id"] == nextResults[0].(map[string]any)["company_id"] {
		t.Fatalf("run result pagination failed: first=%#v next=%#v", results, nextResults)
	}

	staleRequest := request(admin, "http", "rulary.run.execute", runInput, "", "run-stale")
	stalePlan := preview(t, application, staleRequest)
	if _, err := application.DB.ExecContext(ctx, `UPDATE company_license SET license_address = license_address || '变更', updated_at = ? WHERE company_id = 'company_002'`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	staleRequest.PlanHash = stalePlan.PlanHash
	if _, err := application.Runtime.Execute(ctx, staleRequest); !action.IsCode(err, action.CodePlanStale) {
		t.Fatalf("stale execute error = %v", err)
	}

	if _, err := application.DB.ExecContext(ctx, `UPDATE rulary_ruleset_version SET version_number = 99 WHERE version_id = ?`, versionID); err == nil {
		t.Fatal("published RuleVersion was mutable")
	}

	author := resolve(t, application, "user_author")
	if _, err := application.Runtime.Preview(ctx, request(author, "http", "rulary.ruleset.publish", map[string]any{"ruleset_id": rulesetID}, "", "")); !action.IsCode(err, action.CodeAuthzDenied) {
		t.Fatalf("author publish error = %v", err)
	}
	agent, err := application.Identity.ResolveAgentToken(ctx, cfg.AgentToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := application.Runtime.Preview(ctx, request(agent, "mcp", "rulary.run.execute", map[string]any{
		"ruleset_version_id": versionID,
		"source":             map[string]any{"table": "company_license"},
		"target":             map[string]any{"table": "company_address_labels"},
		"limit":              51,
	}, "", "")); !action.IsCode(err, action.CodeLimitExceeded) {
		t.Fatalf("agent max_rows error = %v", err)
	}
	oversizedInput := map[string]any{
		"ruleset_version_id": versionID,
		"source":             map[string]any{"table": "company_license"},
		"target":             map[string]any{"table": "company_address_labels"},
		"limit":              51,
	}
	adminOversized := request(admin, "http", "rulary.run.execute", oversizedInput, "", "admin-oversized")
	adminPlan := preview(t, application, adminOversized)
	plans, err := module.ServiceAs[action.PlanStore](application.Host, module.ServicePlanStore)
	if err != nil {
		t.Fatal(err)
	}
	oversizedPlan, err := plans.Get(ctx, adminPlan.PlanHash)
	if err != nil {
		t.Fatal(err)
	}
	oversizedPlan.Hash = "test-agent-oversized-plan"
	oversizedPlan.ActorID = agent.ID
	if err := plans.Save(ctx, oversizedPlan); err != nil {
		t.Fatal(err)
	}
	agentExecute := request(agent, "mcp", "rulary.run.execute", oversizedInput, oversizedPlan.Hash, "agent-oversized")
	if _, err := application.Runtime.Execute(ctx, agentExecute); !action.IsCode(err, action.CodeLimitExceeded) {
		t.Fatalf("agent max_rows execute error = %v", err)
	}

	audit := execute(t, application, request(admin, "http", "audit.query", map[string]any{"limit": 200}, "", ""))
	events, ok := audit["events"].([]any)
	if !ok || len(events) < 10 {
		t.Fatalf("audit events = %#v", audit["events"])
	}
}

func TestRunPreviewBusinessResultMatchesAcrossChannels(t *testing.T) {
	application, cfg := bootstrap(t)
	defer application.Close()
	admin := resolve(t, application, "user_admin")
	created := execute(t, application, request(admin, "http", "rulary.ruleset.create", map[string]any{"name": "Cross channel", "spec": defaultSpec()}, "", "cross-create"))
	id := nestedString(t, created, "ruleset", "id")
	execute(t, application, request(admin, "http", "rulary.ruleset.validate", map[string]any{"ruleset_id": id}, "", "cross-validate"))
	publishRequest := request(admin, "http", "rulary.ruleset.publish", map[string]any{"ruleset_id": id}, "", "cross-publish")
	publishPlan := preview(t, application, publishRequest)
	publishRequest.PlanHash = publishPlan.PlanHash
	published := execute(t, application, publishRequest)
	versionID := nestedString(t, published, "version", "id")
	agent, err := application.Identity.ResolveAgentToken(context.Background(), cfg.AgentToken)
	if err != nil {
		t.Fatal(err)
	}
	input := map[string]any{
		"ruleset_version_id": versionID,
		"source":             map[string]any{"table": "company_license"},
		"target":             map[string]any{"table": "company_address_labels"},
		"limit":              10,
	}
	httpPreview := preview(t, application, request(admin, "http", "rulary.run.execute", input, "", ""))
	cliPreview := preview(t, application, request(admin, "cli", "rulary.run.execute", input, "", ""))
	mcpPreview := preview(t, application, request(agent, "mcp", "rulary.run.execute", input, "", ""))
	var httpSummary, cliSummary, mcpSummary any
	_ = json.Unmarshal(httpPreview.Summary, &httpSummary)
	_ = json.Unmarshal(cliPreview.Summary, &cliSummary)
	_ = json.Unmarshal(mcpPreview.Summary, &mcpSummary)
	if !reflect.DeepEqual(httpSummary, cliSummary) || !reflect.DeepEqual(httpSummary, mcpSummary) {
		t.Fatalf("channel summaries differ\nhttp=%#v\ncli=%#v\nmcp=%#v", httpSummary, cliSummary, mcpSummary)
	}
}

func TestPreviewPerformance1000Rows(t *testing.T) {
	ctx := context.Background()
	application, _ := bootstrap(t)
	defer application.Close()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := application.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	statement, err := tx.PrepareContext(ctx, `
		INSERT INTO company_license (company_id, company_name, license_address, updated_at)
		VALUES (?, ?, ?, ?)`)
	if err != nil {
		t.Fatal(err)
	}
	for index := 121; index <= 1000; index++ {
		address := fmt.Sprintf("郑州市中原区建设路%d号（园区%d号楼）", index, index)
		if index%3 == 0 {
			address += fmt.Sprintf("；（经营地址备案：郑州市高新区科学大道%d号）", index)
		}
		if _, err := statement.ExecContext(ctx, fmt.Sprintf("company_%04d", index), fmt.Sprintf("基准企业%04d", index), address, now); err != nil {
			t.Fatal(err)
		}
	}
	if err := statement.Close(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	admin := resolve(t, application, "user_admin")
	created := execute(t, application, request(admin, "http", "rulary.ruleset.create", map[string]any{
		"name": "1000-row benchmark", "spec": defaultSpec(),
	}, "", "benchmark-create"))
	rulesetID := nestedString(t, created, "ruleset", "id")
	execute(t, application, request(admin, "http", "rulary.ruleset.validate", map[string]any{"ruleset_id": rulesetID}, "", "validate-performance"))
	publishRequest := request(admin, "http", "rulary.ruleset.publish", map[string]any{"ruleset_id": rulesetID}, "", "benchmark-publish")
	publishPlan := preview(t, application, publishRequest)
	publishRequest.PlanHash = publishPlan.PlanHash
	published := execute(t, application, publishRequest)
	versionID := nestedString(t, published, "version", "id")

	previewRequest := request(admin, "http", "rulary.run.execute", map[string]any{
		"ruleset_version_id": versionID,
		"source":             map[string]any{"table": "company_license"},
		"target":             map[string]any{"table": "company_address_labels"},
		"limit":              1000,
	}, "", "")
	durations := make([]time.Duration, 0, 20)
	for range 20 {
		started := time.Now()
		planned := preview(t, application, previewRequest)
		durations = append(durations, time.Since(started))
		if planned.Impact.Rows != 1000 {
			t.Fatalf("preview impact rows = %d", planned.Impact.Rows)
		}
	}
	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p95 := durations[(len(durations)*95+99)/100-1]
	t.Logf("1000-row preview: p50=%s p95=%s max=%s", durations[len(durations)/2], p95, durations[len(durations)-1])
	if p95 > 3*time.Second {
		t.Fatalf("1000-row preview p95 %s exceeds 3s budget", p95)
	}
}

func bootstrap(t *testing.T) (*app.Application, config.Runtime) {
	t.Helper()
	dataDir := t.TempDir()
	cfg := config.Runtime{
		DataDir: dataDir, DatabasePath: filepath.Join(dataDir, "modary.db"),
		ListenAddress: "127.0.0.1:0", DemoPassword: "integration-password", AgentToken: "integration-agent-token",
	}
	application, err := app.Bootstrap(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return application, cfg
}

func resolve(t *testing.T, application *app.Application, id string) identity.Actor {
	t.Helper()
	actor, err := application.Identity.ResolveByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return actor
}

func request(actor identity.Actor, channel, actionID string, input any, planHash, idempotency string) action.Request {
	data, _ := json.Marshal(input)
	return action.Request{Actor: actor, Channel: channel, ActionID: actionID, WorkspaceID: actor.WorkspaceID, Input: data, PlanHash: planHash, IdempotencyKey: idempotency}
}

func execute(t *testing.T, application *app.Application, request action.Request) map[string]any {
	t.Helper()
	result, err := application.Runtime.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(result.Data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func preview(t *testing.T, application *app.Application, request action.Request) action.Preview {
	t.Helper()
	value, err := application.Runtime.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func nestedString(t *testing.T, value map[string]any, keys ...string) string {
	t.Helper()
	current := any(value)
	for _, key := range keys {
		current = current.(map[string]any)[key]
	}
	result, ok := current.(string)
	if !ok {
		t.Fatalf("%v is not a string", current)
	}
	return result
}

func nestedNumber(t *testing.T, value map[string]any, keys ...string) int {
	t.Helper()
	current := any(value)
	for _, key := range keys {
		current = current.(map[string]any)[key]
	}
	result, ok := current.(float64)
	if !ok {
		t.Fatalf("%v is not a number", current)
	}
	return int(result)
}

func defaultSpec() map[string]any {
	return map[string]any{
		"schema_version": "rulary.ruleset.f0", "id": "company-address", "name": "企业地址标签",
		"source":   map[string]any{"table": "company_license", "primary_key": "company_id", "field": "license_address"},
		"operator": map[string]any{"type": "rulary.address.extract_v1", "filing_marker": "经营地址备案", "parenthetical_note_target": "address_note"},
		"output":   map[string]any{"table": "company_address_labels", "unique_key": "company_id"},
	}
}
