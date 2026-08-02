package consumer_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"example.com/modary-counter-consumer/internal/project"
	"example.com/modary-counter-consumer/modules/counter"
	"github.com/iiwish/modary/action"
	"github.com/iiwish/modary/adapters/localidentity"
	postgresadapter "github.com/iiwish/modary/adapters/postgres"
	"github.com/iiwish/modary/adapters/rbac"
	"github.com/iiwish/modary/adapters/sqlaudit"
	"github.com/iiwish/modary/appcmd"
	"github.com/iiwish/modary/appkit"
	"github.com/iiwish/modary/identity"
	"github.com/iiwish/modary/module"
	"github.com/iiwish/modary/scope"
	"github.com/iiwish/modary/task"
	"github.com/iiwish/modary/transport/httpapi"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

var consumerSchemaSequence atomic.Uint64

func TestCopiedOutConsumerUsesTransactionalTaskRuntime(t *testing.T) {
	config := postgresTestConfig(t)
	definition := mustDefinition(t, config)
	producer := startApplication(t, definition)
	actor := resolveActor(t, producer, project.PrimaryActorID)
	input := counterInput(0, 7)
	preview := previewCounter(t, producer, actor, "test", "task-preview", input)
	executeCounter(t, producer, actor, "test", "task-execute", input, preview.PlanHash, "task-once")
	shutdownApplication(t, producer)

	worker := startApplication(t, definition)
	t.Cleanup(func() {
		if worker.Ready() {
			shutdownApplication(t, worker)
		}
	})

	worked := make(chan task.Job, 1)
	runner, err := worker.Tasks().NewRunner(task.HandlerFunc(func(_ context.Context, job task.Job) error {
		worked <- job
		return nil
	}), task.RunnerOptions{Queues: []task.Queue{{Name: task.DefaultQueue, MaxWorkers: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := runner.Stop(stopCtx); err != nil {
			t.Errorf("stop task runner: %v", err)
		}
	})

	select {
	case job := <-worked:
		if job.Kind != counter.IncrementedTaskKind || job.Queue != task.DefaultQueue || job.Attempt != 1 {
			t.Fatalf("worked task = %#v", job)
		}
		var payload struct {
			Scope   scope.Execution `json:"scope"`
			Value   int64           `json:"value"`
			Version int64           `json:"version"`
		}
		if err := json.Unmarshal(job.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Scope != project.PrimaryScope || payload.Value != 7 || payload.Version != 1 {
			t.Fatalf("worked task payload = %#v", payload)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Counter Action committed without producing its durable task")
	}
}

func TestGovernedCounterAcrossRuntimeCLIHTTPMCPAndRestart(t *testing.T) {
	ctx := context.Background()
	config := postgresTestConfig(t)
	definition := mustDefinition(t, config)

	application := startApplication(t, definition)
	primary := resolveActor(t, application, project.PrimaryActorID)
	secondary := resolveActor(t, application, project.SecondaryActorID)

	firstInput := counterInput(0, 5)
	firstPreview := previewCounter(t, application, primary, "test", "runtime-preview-1", firstInput)
	firstResult := executeCounter(
		t, application, primary, "test", "runtime-execute-1", firstInput, firstPreview.PlanHash, "runtime-shared",
	)
	assertCounterState(t, firstResult, counter.State{Value: 5, Version: 1})
	_, err := application.Runtime().Preview(ctx, action.Request{
		RequestID: "runtime-version-conflict", Actor: primary, Channel: "test",
		ActionID: counter.ActionID, Scope: primary.Scope, Input: counterInput(0, 1),
	})
	assertCounterConflictError(t, err, 1, 0, "runtime-version-conflict")
	replayed := executeCounter(
		t, application, primary, "test", "runtime-replay-1", firstInput, firstPreview.PlanHash, "runtime-shared",
	)
	if !bytes.Equal(firstResult.Data, replayed.Data) || firstResult.Summary != replayed.Summary {
		t.Fatalf("idempotent replay = %#v, want %#v", replayed, firstResult)
	}

	secondaryInput := counterInput(0, 2)
	secondaryPreview := previewCounter(t, application, secondary, "test", "runtime-preview-scope", secondaryInput)
	secondaryResult := executeCounter(
		t, application, secondary, "test", "runtime-execute-scope",
		secondaryInput, secondaryPreview.PlanHash, "runtime-shared",
	)
	assertCounterState(t, secondaryResult, counter.State{Value: 2, Version: 1})

	mismatchedScope := action.Request{
		RequestID: "runtime-wrong-scope",
		Actor:     primary,
		Channel:   "test",
		ActionID:  counter.ActionID,
		Scope:     project.SecondaryScope,
		Input:     counterInput(0, 1),
	}
	if _, err := application.Runtime().Preview(ctx, mismatchedScope); !action.IsCode(err, action.CodeAuthzDenied) {
		t.Fatalf("cross-scope Preview error = %v, want %s", err, action.CodeAuthzDenied)
	}

	unbound := identity.Actor{
		ID: "unbound-actor", Type: "user", DisplayName: "Unbound",
		Scope: project.PrimaryScope,
	}
	if _, err := application.Runtime().Preview(ctx, action.Request{
		RequestID: "runtime-default-deny",
		Actor:     unbound,
		Channel:   "test",
		ActionID:  counter.ActionID,
		Scope:     unbound.Scope,
		Input:     counterInput(1, 1),
	}); !action.IsCode(err, action.CodeAuthzDenied) {
		t.Fatalf("default-deny Preview error = %v, want %s", err, action.CodeAuthzDenied)
	}

	staleInput := counterInput(1, 1)
	staleFirst := previewCounter(t, application, primary, "test", "runtime-stale-preview-1", staleInput)
	staleSecond := previewCounter(t, application, primary, "test", "runtime-stale-preview-2", staleInput)
	staleWinner := executeCounter(
		t, application, primary, "test", "runtime-stale-winner",
		staleInput, staleFirst.PlanHash, "runtime-stale-winner",
	)
	assertCounterState(t, staleWinner, counter.State{Value: 6, Version: 2})
	if _, err := application.Runtime().Execute(ctx, action.Request{
		RequestID:      "runtime-stale-loser",
		Actor:          primary,
		Channel:        "test",
		ActionID:       counter.ActionID,
		Scope:          primary.Scope,
		Input:          staleInput,
		PlanHash:       staleSecond.PlanHash,
		IdempotencyKey: "runtime-stale-loser",
	}); !action.IsCode(err, action.CodePlanStale) {
		t.Fatalf("stale Execute error = %v, want %s", err, action.CodePlanStale)
	}
	shutdownApplication(t, application)

	// This produces value 10 in a persisted plan. It is the external regression
	// for canonical integral plan payloads remaining directly decodable into
	// ordinary Go int64 fields across application restart.
	runCLIConformance(t, definition, 2, 4)

	application = startApplication(t, definition)
	handler, err := project.NewHTTPHandler(context.Background(), application)
	if err != nil {
		t.Fatalf("NewHTTPHandler() error = %v", err)
	}
	runExplicitMountConformance(t, application)
	runHTTPConformance(t, handler, 3, 2)
	runMCPConformance(t, handler, 4, 3)
	shutdownApplication(t, application)

	application = startApplication(t, definition)
	primary = resolveActor(t, application, project.PrimaryActorID)
	restartPreview := previewCounter(
		t, application, primary, "test", "restart-preview", counterInput(5, 1),
	)
	var summary struct {
		CurrentValue   int64 `json:"current_value"`
		CurrentVersion int64 `json:"current_version"`
		NextValue      int64 `json:"next_value"`
		NextVersion    int64 `json:"next_version"`
	}
	if err := json.Unmarshal(restartPreview.Summary, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.CurrentValue != 15 || summary.CurrentVersion != 5 ||
		summary.NextValue != 16 || summary.NextVersion != 6 {
		t.Fatalf("restart Preview summary = %#v", summary)
	}
	shutdownApplication(t, application)

	assertDurableStateAndAudit(t, config)
}

func TestOfficialAdaptersHaveEmptyProvisioningByDefault(t *testing.T) {
	config := postgresTestConfig(t)
	postgresModule, err := postgresadapter.Module(postgresadapter.Options{
		URL: config.DatabaseURL, ApplicationSchema: config.ApplicationSchema, QueueSchema: config.QueueSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	identityModule, err := localidentity.Module(localidentity.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rbacModule, err := rbac.Module(rbac.Options{})
	if err != nil {
		t.Fatal(err)
	}
	application := startApplication(t, appkit.Definition{
		Metadata: appkit.Metadata{ID: "empty-consumer", Name: "Empty Consumer", Version: "0.1.0"},
		Modules: []module.Registration{
			postgresModule,
			identityModule,
			rbacModule,
			sqlaudit.Module(sqlaudit.Options{}),
		},
	})
	shutdownApplication(t, application)

	db := openDatabase(t, config)
	defer db.Close()
	for _, table := range []string{
		"modary_identity_principal",
		"modary_identity_password",
		"modary_identity_bearer",
		"modary_identity_session",
		"modary_rbac_role",
		"modary_rbac_role_permission",
		"modary_rbac_binding",
		"modary_audit_event",
	} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != 0 {
			t.Errorf("%s contains %d implicitly provisioned rows", table, count)
		}
	}
	var counterTableExists bool
	if err := db.QueryRow(`SELECT to_regclass('consumer_counter') IS NOT NULL`).Scan(&counterTableExists); err != nil {
		t.Fatal(err)
	}
	if counterTableExists {
		t.Fatal("official adapters created a consumer-domain table")
	}
}

func TestConsumerApplicationCommandServesAndDrains(t *testing.T) {
	definition := mustDefinition(t, postgresTestConfig(t))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	options := project.CommandOptions()
	options.ListenAddress = listener.Addr().String()
	options.Stdout = io.Discard
	options.Stderr = io.Discard
	options.ShutdownTimeout = 2 * time.Second
	options.Listener = func(context.Context, string, string) (net.Listener, error) {
		return listener, nil
	}
	result := make(chan error, 1)
	go func() {
		result <- appcmd.Run(ctx, []string{"serve"}, func() (appkit.Definition, error) {
			return definition, nil
		}, options)
	}()

	client := &http.Client{Timeout: time.Second}
	healthURL := "http://" + listener.Addr().String() + "/healthz"
	readyDeadline := time.Now().Add(20 * time.Second)
	var response *http.Response
	for {
		response, err = client.Get(healthURL)
		if err == nil {
			break
		}
		select {
		case runErr := <-result:
			t.Fatalf("application command exited before readiness: %v", runErr)
		default:
		}
		if time.Now().After(readyDeadline) {
			cancel()
			<-result
			t.Fatalf("served health request before readiness deadline: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	body, readErr := io.ReadAll(response.Body)
	closeErr := response.Body.Close()
	if readErr != nil || closeErr != nil {
		cancel()
		<-result
		t.Fatalf("read served health response: %v", errors.Join(readErr, closeErr))
	}
	if response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(`"counter-console"`)) {
		cancel()
		<-result
		t.Fatalf("served health = %d %s", response.StatusCode, body)
	}
	cancel()
	select {
	case err := <-result:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("appcmd serve drain error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("appcmd serve did not drain after cancellation")
	}
}

func runCLIConformance(t *testing.T, definition appkit.Definition, expectedVersion, amount int64) {
	t.Helper()
	input := counterInput(expectedVersion, amount)
	directory := t.TempDir()
	inputPath := filepath.Join(directory, "input.json")
	if err := os.WriteFile(inputPath, input, 0o600); err != nil {
		t.Fatal(err)
	}
	tokenPath := filepath.Join(directory, "token")
	if err := os.WriteFile(tokenPath, []byte(project.PrimaryBearerToken), 0o600); err != nil {
		t.Fatal(err)
	}
	conflictPath := filepath.Join(directory, "conflict.json")
	conflictVersion := expectedVersion - 1
	if err := os.WriteFile(conflictPath, counterInput(conflictVersion, amount), 0o600); err != nil {
		t.Fatal(err)
	}
	var conflictOutput bytes.Buffer
	conflictErr := appcmd.RunAction(context.Background(), []string{
		"run", counter.ActionID,
		"--token-file", tokenPath,
		"--input", conflictPath,
		"--preview",
		"--request-id", "cli-version-conflict",
	}, definition, appcmd.Options{Stdout: &conflictOutput, Stderr: io.Discard})
	assertCounterConflictError(t, conflictErr, expectedVersion, conflictVersion, "cli-version-conflict")
	if conflictOutput.Len() != 0 {
		t.Fatalf("CLI conflict wrote stdout: %q", conflictOutput.String())
	}
	wantConflictText := "preview Action " + counter.ActionID + ": " +
		counter.ErrorVersionConflict + ": " + counterConflictMessage(expectedVersion, conflictVersion)
	if conflictErr == nil || conflictErr.Error() != wantConflictText {
		t.Fatalf("CLI conflict error = %v, want %q", conflictErr, wantConflictText)
	}
	var previewOutput bytes.Buffer
	if err := appcmd.RunAction(context.Background(), []string{
		"run", counter.ActionID,
		"--token-file", tokenPath,
		"--input", inputPath,
		"--preview",
		"--request-id", "cli-preview",
	}, definition, appcmd.Options{Stdout: &previewOutput, Stderr: io.Discard}); err != nil {
		t.Fatalf("CLI Preview error = %v", err)
	}
	var preview action.Preview
	if err := json.Unmarshal(previewOutput.Bytes(), &preview); err != nil {
		t.Fatalf("decode CLI Preview: %v", err)
	}
	if preview.PlanHash == "" {
		t.Fatal("CLI Preview returned no plan hash")
	}

	args := []string{
		"run", counter.ActionID,
		"--token-file", tokenPath,
		"--input", inputPath,
		"--plan", preview.PlanHash,
		"--idempotency-key", "cli-once",
		"--request-id", "cli-execute",
	}
	var first bytes.Buffer
	if err := appcmd.RunAction(
		context.Background(), args, definition,
		appcmd.Options{Stdout: &first, Stderr: io.Discard},
	); err != nil {
		t.Fatalf("CLI Execute error = %v", err)
	}
	var result action.Result
	if err := json.Unmarshal(first.Bytes(), &result); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}
	assertCounterState(t, result, counter.State{Value: 10, Version: 3})

	args[len(args)-1] = "cli-replay"
	var replay bytes.Buffer
	if err := appcmd.RunAction(
		context.Background(), args, definition,
		appcmd.Options{Stdout: &replay, Stderr: io.Discard},
	); err != nil {
		t.Fatalf("CLI replay error = %v", err)
	}
	if !bytes.Equal(first.Bytes(), replay.Bytes()) {
		t.Fatalf("CLI idempotent replay differs:\nfirst: %s\nreplay: %s", first.Bytes(), replay.Bytes())
	}
}

func runHTTPConformance(t *testing.T, handler http.Handler, expectedVersion, amount int64) {
	t.Helper()
	root := httpRequest(t, handler, http.MethodGet, "/", "", nil, nil)
	if root.Code != http.StatusOK || !strings.Contains(root.Body.String(), "Counter Console") {
		t.Fatalf("static UI = %d %s", root.Code, root.Body.String())
	}
	health := httpRequest(t, handler, http.MethodGet, "/healthz", "", nil, map[string]string{
		"Accept": "application/json",
	})
	if health.Code != http.StatusOK || !strings.Contains(health.Body.String(), `"id":"counter-console"`) {
		t.Fatalf("health = %d %s", health.Code, health.Body.String())
	}
	login := httpRequest(
		t, handler, http.MethodPost, "/api/auth/login",
		fmt.Sprintf(`{"username":%q,"password":%q}`, project.PrimaryUsername, project.PrimaryPassword),
		nil,
		map[string]string{"Content-Type": "application/json", "Accept": "application/json"},
	)
	if login.Code != http.StatusOK {
		t.Fatalf("login = %d %s", login.Code, login.Body.String())
	}
	var session struct {
		CSRFToken string `json:"csrf_token"`
	}
	if err := json.Unmarshal(login.Body.Bytes(), &session); err != nil {
		t.Fatal(err)
	}
	cookies := login.Result().Cookies()
	if len(cookies) != 1 || session.CSRFToken == "" {
		t.Fatalf("login response cookies=%#v body=%s", cookies, login.Body.String())
	}
	headers := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
		"X-CSRF-Token": session.CSRFToken,
	}
	conflictVersion := expectedVersion - 1
	headers["X-Request-ID"] = "http-version-conflict"
	conflict := httpRequest(
		t, handler, http.MethodPost, "/api/actions/"+counter.ActionID+"/preview",
		fmt.Sprintf(`{"input":%s}`, counterInput(conflictVersion, amount)), cookies, headers,
	)
	delete(headers, "X-Request-ID")
	if conflict.Code != http.StatusConflict {
		t.Fatalf("HTTP conflict = %d %s", conflict.Code, conflict.Body.String())
	}
	var conflictEnvelope struct {
		Error     publicCounterError `json:"error"`
		RequestID string             `json:"request_id"`
	}
	if err := json.Unmarshal(conflict.Body.Bytes(), &conflictEnvelope); err != nil {
		t.Fatal(err)
	}
	assertPublicCounterConflict(t, conflictEnvelope.Error, expectedVersion, conflictVersion)
	if conflictEnvelope.RequestID != "http-version-conflict" ||
		conflictEnvelope.Error.RequestID != conflictEnvelope.RequestID {
		t.Fatalf("HTTP conflict request context = %#v", conflictEnvelope)
	}
	input := counterInput(expectedVersion, amount)
	preview := httpRequest(
		t, handler, http.MethodPost, "/api/actions/"+counter.ActionID+"/preview",
		fmt.Sprintf(`{"input":%s}`, input), cookies, headers,
	)
	if preview.Code != http.StatusOK {
		t.Fatalf("HTTP Preview = %d %s", preview.Code, preview.Body.String())
	}
	var previewEnvelope struct {
		Preview action.Preview `json:"preview"`
	}
	if err := json.Unmarshal(preview.Body.Bytes(), &previewEnvelope); err != nil {
		t.Fatal(err)
	}
	executeBody := fmt.Sprintf(
		`{"input":%s,"plan_hash":%q,"idempotency_key":"http-once"}`,
		input,
		previewEnvelope.Preview.PlanHash,
	)
	executed := httpRequest(
		t, handler, http.MethodPost, "/api/actions/"+counter.ActionID+"/execute",
		executeBody, cookies, headers,
	)
	if executed.Code != http.StatusOK {
		t.Fatalf("HTTP Execute = %d %s", executed.Code, executed.Body.String())
	}
	var result struct {
		Data json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(executed.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	assertCounterData(t, result.Data, counter.State{Value: 12, Version: 4})
	replayed := httpRequest(
		t, handler, http.MethodPost, "/api/actions/"+counter.ActionID+"/execute",
		executeBody, cookies, headers,
	)
	if replayed.Code != http.StatusOK {
		t.Fatalf("HTTP replay = %d %s", replayed.Code, replayed.Body.String())
	}
	var replayResult struct {
		Data json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(replayed.Body.Bytes(), &replayResult); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Data, replayResult.Data) {
		t.Fatalf("HTTP replay result = %s, want %s", replayResult.Data, result.Data)
	}
}

func runMCPConformance(t *testing.T, handler http.Handler, expectedVersion, amount int64) {
	t.Helper()
	initialize := mcpRequest(t, handler, false, `{
		"jsonrpc":"2.0",
		"id":1,
		"method":"initialize",
		"params":{
			"protocolVersion":"2025-11-25",
			"capabilities":{},
			"clientInfo":{"name":"counter-test","version":"1.0.0"}
		}
	}`)
	if initialize.Code != http.StatusOK {
		t.Fatalf("MCP initialize = %d %s", initialize.Code, initialize.Body.String())
	}
	var initialized struct {
		Result struct {
			ProtocolVersion string `json:"protocolVersion"`
			ServerInfo      struct {
				Name    string `json:"name"`
				Title   string `json:"title"`
				Version string `json:"version"`
			} `json:"serverInfo"`
		} `json:"result"`
	}
	if err := json.Unmarshal(initialize.Body.Bytes(), &initialized); err != nil {
		t.Fatal(err)
	}
	if initialized.Result.ProtocolVersion != httpapi.MCPProtocolVersion ||
		initialized.Result.ServerInfo.Name != "counter-console" ||
		initialized.Result.ServerInfo.Title != "Counter Console" ||
		initialized.Result.ServerInfo.Version != "0.1.0" {
		t.Fatalf("MCP initialize result = %#v", initialized.Result)
	}
	acknowledged := mcpRequest(t, handler, true, `{
		"jsonrpc":"2.0","method":"notifications/initialized","params":{}
	}`)
	if acknowledged.Code != http.StatusAccepted || acknowledged.Body.Len() != 0 {
		t.Fatalf("MCP initialized notification = %d %q", acknowledged.Code, acknowledged.Body.String())
	}

	listed := mcpRequest(t, handler, true, `{
		"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}
	}`)
	if listed.Code != http.StatusOK {
		t.Fatalf("MCP tools/list = %d %s", listed.Code, listed.Body.String())
	}
	var catalog struct {
		Result struct {
			Tools []struct {
				Name string            `json:"name"`
				Meta map[string]string `json:"_meta"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &catalog); err != nil {
		t.Fatal(err)
	}
	var previewTool, executeTool string
	for _, tool := range catalog.Result.Tools {
		if tool.Meta["modary/actionId"] != counter.ActionID {
			continue
		}
		switch tool.Meta["modary/operation"] {
		case "preview":
			previewTool = tool.Name
		case "execute":
			executeTool = tool.Name
		}
	}
	if previewTool == "" || executeTool == "" {
		t.Fatalf("MCP Counter tools = %#v", catalog.Result.Tools)
	}

	conflictVersion := expectedVersion - 1
	conflict := mcpRequest(t, handler, true, fmt.Sprintf(`{
		"jsonrpc":"2.0",
		"id":3,
		"method":"tools/call",
		"params":{"name":%q,"arguments":{"input":%s}}
	}`, previewTool, counterInput(conflictVersion, amount)))
	if conflict.Code != http.StatusOK {
		t.Fatalf("MCP conflict = %d %s", conflict.Code, conflict.Body.String())
	}
	var conflictEnvelope struct {
		Error  json.RawMessage `json:"error"`
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Structured json.RawMessage `json:"structuredContent"`
			IsError    bool            `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(conflict.Body.Bytes(), &conflictEnvelope); err != nil {
		t.Fatal(err)
	}
	if len(conflictEnvelope.Error) != 0 || !conflictEnvelope.Result.IsError ||
		len(conflictEnvelope.Result.Structured) != 0 || len(conflictEnvelope.Result.Content) != 1 ||
		conflictEnvelope.Result.Content[0].Type != "text" {
		t.Fatalf("MCP conflict envelope = %#v; body=%s", conflictEnvelope, conflict.Body.String())
	}
	var conflictDetail publicCounterError
	if err := json.Unmarshal([]byte(conflictEnvelope.Result.Content[0].Text), &conflictDetail); err != nil {
		t.Fatalf("decode MCP conflict detail: %v; text=%q", err, conflictEnvelope.Result.Content[0].Text)
	}
	assertPublicCounterConflict(t, conflictDetail, expectedVersion, conflictVersion)
	if conflictDetail.RequestID == "" {
		t.Fatal("MCP conflict has no generated request id")
	}

	input := counterInput(expectedVersion, amount)
	previewed := mcpRequest(t, handler, true, fmt.Sprintf(`{
		"jsonrpc":"2.0",
		"id":4,
		"method":"tools/call",
		"params":{"name":%q,"arguments":{"input":%s}}
	}`, previewTool, input))
	if previewed.Code != http.StatusOK {
		t.Fatalf("MCP Preview = %d %s", previewed.Code, previewed.Body.String())
	}
	var previewEnvelope struct {
		Result struct {
			Structured struct {
				Preview action.Preview `json:"preview"`
			} `json:"structuredContent"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(previewed.Body.Bytes(), &previewEnvelope); err != nil {
		t.Fatal(err)
	}
	if previewEnvelope.Result.IsError || previewEnvelope.Result.Structured.Preview.PlanHash == "" {
		t.Fatalf("MCP Preview result = %#v", previewEnvelope.Result)
	}
	executed := mcpRequest(t, handler, true, fmt.Sprintf(`{
		"jsonrpc":"2.0",
		"id":5,
		"method":"tools/call",
		"params":{
			"name":%q,
			"arguments":{
				"input":%s,
				"plan_hash":%q,
				"idempotency_key":"mcp-once"
			}
		}
	}`, executeTool, input, previewEnvelope.Result.Structured.Preview.PlanHash))
	if executed.Code != http.StatusOK {
		t.Fatalf("MCP Execute = %d %s", executed.Code, executed.Body.String())
	}
	var resultEnvelope struct {
		Result struct {
			Structured struct {
				Result json.RawMessage `json:"result"`
			} `json:"structuredContent"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(executed.Body.Bytes(), &resultEnvelope); err != nil {
		t.Fatal(err)
	}
	if resultEnvelope.Result.IsError {
		t.Fatalf("MCP Execute returned tool error: %s", executed.Body.String())
	}
	assertCounterData(t, resultEnvelope.Result.Structured.Result, counter.State{Value: 15, Version: 5})
}

func runExplicitMountConformance(t *testing.T, application *appkit.Application) {
	t.Helper()
	health, err := httpapi.NewHealth(application)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle("/healthz", health)
	for _, path := range []string{"/", "/api/actions", "/mcp"} {
		response := httpRequest(t, mux, http.MethodGet, path, "", nil, nil)
		if response.Code != http.StatusNotFound {
			t.Errorf("unmounted route %s returned %d", path, response.Code)
		}
	}
}

func assertDurableStateAndAudit(t *testing.T, config project.Config) {
	t.Helper()
	db := openDatabase(t, config)
	defer db.Close()
	assertStoredState(t, db, project.PrimaryScope, counter.State{Value: 15, Version: 5})
	assertStoredState(t, db, project.SecondaryScope, counter.State{Value: 2, Version: 1})

	decisions := make(map[string]int)
	rows, err := db.Query(`
		SELECT decision, COUNT(*)
		FROM modary_audit_event
		WHERE action_id = $1
		GROUP BY decision`, counter.ActionID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var decision string
		var count int
		if err := rows.Scan(&decision, &count); err != nil {
			t.Fatal(err)
		}
		decisions[decision] = count
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if decisions["allowed"] != 6 {
		t.Errorf("allowed audit events = %d, want 6", decisions["allowed"])
	}
	if decisions["idempotent_replay"] != 3 {
		t.Errorf("idempotent replay audit events = %d, want 3", decisions["idempotent_replay"])
	}
	if decisions["previewed"] != 8 {
		t.Errorf("preview audit events = %d, want 8", decisions["previewed"])
	}
	if decisions["denied"] != 2 {
		t.Errorf("denied audit events = %d, want 2", decisions["denied"])
	}
	if decisions["rejected"] != 5 {
		t.Errorf("rejected audit events = %d, want 5", decisions["rejected"])
	}
	if decisions["failed"] != 0 {
		t.Errorf("failed audit events = %d, want 0", decisions["failed"])
	}
	var errorCode, errorKind, reason string
	if err := db.QueryRow(`
		SELECT error_code, error_kind, reason
		FROM modary_audit_event
		WHERE request_id = $1`, "runtime-stale-loser").Scan(&errorCode, &errorKind, &reason); err != nil {
		t.Fatal(err)
	}
	if errorCode != action.CodePlanStale || errorKind != string(action.ErrorKindConflict) || reason != "Counter changed after Preview" {
		t.Errorf("stale audit error = %q/%q %q", errorCode, errorKind, reason)
	}
	assertCounterConflictAudit(t, db)
}

type publicCounterError struct {
	Code      string           `json:"error_code"`
	Kind      action.ErrorKind `json:"error_kind"`
	Message   string           `json:"human_readable_reason"`
	ActionID  string           `json:"action_id"`
	RequestID string           `json:"request_id"`
}

func counterConflictMessage(currentVersion, expectedVersion int64) string {
	return fmt.Sprintf("Counter version is %d, expected %d", currentVersion, expectedVersion)
}

func assertCounterConflictError(
	t *testing.T,
	err error,
	currentVersion, expectedVersion int64,
	requestID string,
) {
	t.Helper()
	if err == nil || action.ErrorCode(err) != counter.ErrorVersionConflict ||
		action.ErrorKindOf(err) != action.ErrorKindConflict {
		t.Fatalf("Counter conflict error = %v, kind=%q", err, action.ErrorKindOf(err))
	}
	var governed *action.Error
	if !errors.As(err, &governed) || governed == nil {
		t.Fatalf("Counter conflict does not retain action.Error: %v", err)
	}
	if governed.Code != counter.ErrorVersionConflict || governed.Kind != action.ErrorKindConflict ||
		governed.Message != counterConflictMessage(currentVersion, expectedVersion) ||
		governed.ActionID != counter.ActionID || governed.RequestID != requestID {
		t.Fatalf("Counter conflict detail = %#v", governed)
	}
}

func assertPublicCounterConflict(t *testing.T, got publicCounterError, currentVersion, expectedVersion int64) {
	t.Helper()
	if got.Code != counter.ErrorVersionConflict || got.Kind != action.ErrorKindConflict ||
		got.Message != counterConflictMessage(currentVersion, expectedVersion) ||
		got.ActionID != counter.ActionID {
		t.Fatalf("public Counter conflict = %#v", got)
	}
}

func assertCounterConflictAudit(t *testing.T, db *sql.DB) {
	t.Helper()
	wantMessages := map[string]string{
		"test": counterConflictMessage(1, 0),
		"cli":  counterConflictMessage(2, 1),
		"http": counterConflictMessage(3, 2),
		"mcp":  counterConflictMessage(4, 3),
	}
	wantRequestIDs := map[string]string{
		"test": "runtime-version-conflict",
		"cli":  "cli-version-conflict",
		"http": "http-version-conflict",
	}
	rows, err := db.Query(`
		SELECT channel, decision, error_kind, reason, request_id
		FROM modary_audit_event
		WHERE action_id = $1 AND error_code = $2
		ORDER BY channel`, counter.ActionID, counter.ErrorVersionConflict)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	seen := make(map[string]bool)
	for rows.Next() {
		var channel, decision, kind, reason, requestID string
		if err := rows.Scan(&channel, &decision, &kind, &reason, &requestID); err != nil {
			t.Fatal(err)
		}
		message, ok := wantMessages[channel]
		if !ok || seen[channel] || decision != "rejected" || kind != string(action.ErrorKindConflict) ||
			reason != message || requestID == "" {
			t.Fatalf("Counter conflict audit = channel=%q decision=%q kind=%q reason=%q request=%q", channel, decision, kind, reason, requestID)
		}
		if wantRequestID := wantRequestIDs[channel]; wantRequestID != "" && requestID != wantRequestID {
			t.Fatalf("Counter conflict audit request id for %s = %q, want %q", channel, requestID, wantRequestID)
		}
		seen[channel] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != len(wantMessages) {
		t.Fatalf("Counter conflict audit channels = %#v, want %#v", seen, wantMessages)
	}
}

func startApplication(t *testing.T, definition appkit.Definition) *appkit.Application {
	t.Helper()
	application, err := appkit.Start(context.Background(), definition, appkit.Options{})
	if err != nil {
		t.Fatalf("appkit.Start() error = %v", err)
	}
	return application
}

func shutdownApplication(t *testing.T, application *appkit.Application) {
	t.Helper()
	if err := application.Shutdown(context.Background()); err != nil {
		t.Fatalf("Application.Shutdown() error = %v", err)
	}
}

func resolveActor(t *testing.T, application *appkit.Application, actorID string) identity.Actor {
	t.Helper()
	identities, err := application.Identities()
	if err != nil {
		t.Fatal(err)
	}
	actor, err := identities.ResolveByID(context.Background(), actorID)
	if err != nil {
		t.Fatal(err)
	}
	return actor
}

func previewCounter(
	t *testing.T,
	application *appkit.Application,
	actor identity.Actor,
	channel action.Channel,
	requestID string,
	input json.RawMessage,
) action.Preview {
	t.Helper()
	preview, err := application.Runtime().Preview(context.Background(), action.Request{
		RequestID: requestID,
		Actor:     actor,
		Channel:   channel,
		ActionID:  counter.ActionID,
		Scope:     actor.Scope,
		Input:     input,
	})
	if err != nil {
		t.Fatalf("Preview(%s) error = %v", requestID, err)
	}
	return preview
}

func executeCounter(
	t *testing.T,
	application *appkit.Application,
	actor identity.Actor,
	channel action.Channel,
	requestID string,
	input json.RawMessage,
	planHash, idempotencyKey string,
) action.Result {
	t.Helper()
	result, err := application.Runtime().Execute(context.Background(), action.Request{
		RequestID:      requestID,
		Actor:          actor,
		Channel:        channel,
		ActionID:       counter.ActionID,
		Scope:          actor.Scope,
		Input:          input,
		PlanHash:       planHash,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		t.Fatalf("Execute(%s) error = %v", requestID, err)
	}
	return result
}

func counterInput(expectedVersion, amount int64) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(
		`{"amount":%d,"expected_version":%d}`,
		amount,
		expectedVersion,
	))
}

func assertCounterState(t *testing.T, result action.Result, expected counter.State) {
	t.Helper()
	assertCounterData(t, result.Data, expected)
	if len(result.References) != 1 || result.References[0].Kind != "counter" {
		t.Fatalf("Counter references = %#v", result.References)
	}
}

func assertCounterData(t *testing.T, data json.RawMessage, expected counter.State) {
	t.Helper()
	var actual counter.State
	if err := json.Unmarshal(data, &actual); err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("Counter state = %#v, want %#v", actual, expected)
	}
}

func assertStoredState(t *testing.T, db *sql.DB, execution scope.Execution, expected counter.State) {
	t.Helper()
	var actual counter.State
	if err := db.QueryRow(`
		SELECT value, version FROM consumer_counter
		WHERE scope_kind = $1 AND scope_id = $2`,
		execution.Kind,
		execution.ID,
	).Scan(&actual.Value, &actual.Version); err != nil {
		t.Fatal(err)
	}
	if actual != expected {
		t.Fatalf("stored Counter %s = %#v, want %#v", execution, actual, expected)
	}
}

func openDatabase(t *testing.T, config project.Config) *sql.DB {
	t.Helper()
	parsed, err := pgx.ParseConfig(config.DatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed.RuntimeParams["search_path"] = quoteTestIdentifier(config.ApplicationSchema)
	db := stdlib.OpenDB(*parsed)
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}

func postgresTestConfig(t *testing.T) project.Config {
	t.Helper()
	url := os.Getenv("MODARY_TEST_DATABASE_URL")
	if url == "" {
		url = os.Getenv("MODARY_DATABASE_URL")
	}
	if url == "" {
		url = "postgres://modary:modary-test-password@127.0.0.1:55432/modary_test?sslmode=disable"
	}
	admin, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.PingContext(context.Background()); err != nil {
		_ = admin.Close()
		if strings.Contains(err.Error(), "connection refused") {
			t.Skipf("PostgreSQL integration service unavailable: %v", err)
		}
		t.Fatal(err)
	}
	suffix := consumerSchemaSequence.Add(1)
	config := project.Config{
		DatabaseURL: url, ApplicationSchema: fmt.Sprintf("counter_test_%d", suffix),
		QueueSchema: fmt.Sprintf("counter_queue_test_%d", suffix),
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + quoteTestIdentifier(config.QueueSchema) + ` CASCADE`)
		_, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + quoteTestIdentifier(config.ApplicationSchema) + ` CASCADE`)
		_ = admin.Close()
	})
	return config
}

func quoteTestIdentifier(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func httpRequest(
	t *testing.T,
	handler http.Handler,
	method, target, body string,
	cookies []*http.Cookie,
	headers map[string]string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func mcpRequest(t *testing.T, handler http.Handler, initialized bool, body string) *httptest.ResponseRecorder {
	t.Helper()
	headers := map[string]string{
		"Accept":        "application/json, text/event-stream",
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + project.PrimaryBearerToken,
	}
	if initialized {
		headers["MCP-Protocol-Version"] = httpapi.MCPProtocolVersion
	}
	return httpRequest(t, handler, http.MethodPost, "/mcp", body, nil, headers)
}

func TestActionErrorsRemainInspectableAcrossTheExternalBoundary(t *testing.T) {
	err := action.WithRequest(
		action.NewError(action.CodePlanStale, "stale"),
		action.Request{ActionID: counter.ActionID, Scope: project.PrimaryScope},
		counter.Permission,
	)
	if !action.IsCode(err, action.CodePlanStale) {
		t.Fatalf("Action error classification lost: %v", err)
	}
	var typed *action.Error
	if !errors.As(err, &typed) || typed.Scope == nil || *typed.Scope != project.PrimaryScope {
		t.Fatalf("Action error context = %#v", typed)
	}
}
