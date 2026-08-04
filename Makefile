GO ?= go
RELEASE_MODE ?= candidate
CONSUMER_DIR := examples/counter
ADMIN_WEB_DIR := starter/templates/admin/web
OFFICIAL_MODULE_DIRS := components/postgres components/governedpostgres components/oidc components/otel integration
PUBLISHED_COMPONENT_DIRS := components/postgres components/governedpostgres components/oidc components/otel
GO_COMMAND_ENV := GO111MODULE=on GOTOOLCHAIN=local GOENV=off GOWORK=off GOFLAGS=
CROSS_BUILD_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64
REPEAT_TIMEOUT ?= 8m
FUZZ_SMOKE_EXECUTIONS ?= 1000x
REPEAT_PACKAGES := \
	./action \
	./appcmd \
	./appkit \
	./database \
	./httpkit \
	./internal/actionruntime \
	./internal/callbackcontract \
	./internal/databasecontrol \
	./internal/jsonschema/... \
	./internal/jsonvalue \
	./internal/runtimecontrol \
	./internal/safeerr \
	./internal/sqlpolicy \
	./internal/transactionoutcome \
	./module \
	./projecttool \
	./task \
	./transport/httpapi
REPEAT_STARTER_TESTS := ^(TestCreateNeverOverwritesOrPatchesDestination|TestCreateRejectsUnsafeOrInvalidInputsBeforeWrites|TestRunCreatesAPIProfileWithFlagsAfterDestination|TestRunValidatesSyntaxBeforeCreation|TestRunAcceptsRepeatableAdminComponentSelection|TestRunHelpHasNoFilesystemSideEffects)$$
REPEAT_IDENTITY_TESTS := ^(TestProvisioningRollsBackAsOneTransaction|TestCredentialReadsClassifyConcurrentRevocation|TestConcurrentLoginProducesDistinctSessions|TestPasswordRotationCannotRaceAStaleLoginIntoAValidSession|TestPasswordVerificationConcurrencyIsBoundedAndCancelable)$$
REPEAT_RBAC_TESTS := ^(TestProvisioningRollsBackOnMissingRole|TestConcurrentReadsAndPolicyRefresh|TestAuthorizeUsesTheCallerTransaction)$$
REPEAT_AUDIT_TESTS := ^(TestAuditParticipatesInCallerTransaction|TestConcurrentRecord)$$
REPEAT_GOVERNED_TESTS := ^(TestConcurrentHostsSerializeSchemaAndModuleMigrations|TestQueueSchemaCannotBeSharedAcrossApplicationProfiles|TestSchemaRoleCannotBeReusedAcrossProfiles|TestConcurrentSwappedSchemaProfilesHaveOneWinner|TestTaskEnqueueSharesGovernedTransaction|TestTaskRunnerWorksCommittedJob|TestTaskRunnerUsesConfiguredRetryDelays|TestPostgresNestedTransactionSuccessJoinsOuterUnit|TestPostgresNestedTransactionFailureMarksOuterRollbackOnly|TestPostgresNestedTransactionPanicMarksOuterRollbackOnly|TestPlanAndIdempotencyRecordsSurviveRestartExactly|TestTransactionManagerCommitsRollsBackAndNests|TestRootTransactionNilPanicRollsBackAndPropagates|TestConcurrentIdempotencyReservationHasOneWinner|TestPostgresPreservesNestedRuntimeAndDatabaseTransactionMarkers)$$
REPEAT_CONSUMER_TESTS := ^(TestCustomCapabilityResolverLifetimeAndConcurrency|TestCopiedOutConsumerUsesTransactionalTaskRuntime|TestGovernedCounterAcrossRuntimeCLIHTTPMCPAndRestart|TestConsumerApplicationCommandServesAndDrains)$$

.PHONY: bootstrap format-check tidy-check diff-check docs-check react-admin-check admin-frontend verify check-generated neutrality \
	test test-framework test-consumer vet race repeat fuzz-smoke build \
	panicnil vulncheck cross-build native-platform copied-admin-acceptance copied-governed-acceptance copied-profile-acceptance acceptance-core acceptance \
	provider-acceptance container-acceptance ci-core-gates ci-gates ci-core ci \
	release-preflight release-readiness remote-consumer released-container-acceptance clean

bootstrap:
	$(GO_COMMAND_ENV) $(GO) mod download
	@set -eu; for directory in $(PUBLISHED_COMPONENT_DIRS); do cd "$$directory" && $(GO_COMMAND_ENV) $(GO) mod download; cd - >/dev/null; done
	cd integration && $(GO_COMMAND_ENV) $(GO) mod download
	cd $(CONSUMER_DIR) && $(GO_COMMAND_ENV) $(GO) mod download

format-check:
	@unformatted="$$(git ls-files --cached --others --exclude-standard -z -- '*.go' | xargs -0 sh -c 'for file do if test -f "$$file"; then gofmt -l "$$file"; fi; done' sh)"; \
	if test -n "$$unformatted"; then printf 'gofmt required:\n%s\n' "$$unformatted"; exit 1; fi

tidy-check:
	$(GO_COMMAND_ENV) $(GO) mod tidy -diff
	@set -eu; for directory in $(PUBLISHED_COMPONENT_DIRS); do cd "$$directory" && $(GO_COMMAND_ENV) $(GO) mod tidy -diff; cd - >/dev/null; done
	cd integration && $(GO_COMMAND_ENV) $(GO) mod tidy -diff
	cd $(CONSUMER_DIR) && $(GO_COMMAND_ENV) $(GO) mod tidy -diff

diff-check:
	./scripts/check-source-diff.sh

docs-check:
	MODARY_DOCS_ROOT=. ./scripts/check-docs.sh
	./scripts/check-doc-links.sh

react-admin-check:
	./scripts/check-react-admin.sh

admin-frontend:
	cd $(ADMIN_WEB_DIR) && pnpm install --frozen-lockfile
	cd $(ADMIN_WEB_DIR) && pnpm lint
	cd $(ADMIN_WEB_DIR) && pnpm typecheck
	cd $(ADMIN_WEB_DIR) && pnpm test
	cd $(ADMIN_WEB_DIR) && pnpm build:variants
	cd $(ADMIN_WEB_DIR) && pnpm assets:check
	cd $(ADMIN_WEB_DIR) && pnpm audit:prod

verify:
	cd $(CONSUMER_DIR) && $(GO_COMMAND_ENV) $(GO) run ./tools/modary verify

check-generated:
	cd $(CONSUMER_DIR) && $(GO_COMMAND_ENV) $(GO) run ./tools/modary generate --check
	cd $(CONSUMER_DIR) && $(GO_COMMAND_ENV) $(GO) run ./tools/modary check

neutrality:
	./scripts/check-neutrality.sh

test: test-framework test-consumer

test-framework:
	$(GO_COMMAND_ENV) $(GO) test -count=1 ./...
	@set -eu; for directory in $(PUBLISHED_COMPONENT_DIRS); do cd "$$directory" && $(GO_COMMAND_ENV) $(GO) test -count=1 ./...; cd - >/dev/null; done
	cd integration && $(GO_COMMAND_ENV) $(GO) test -count=1 ./...

test-consumer:
	cd $(CONSUMER_DIR) && MODARY_EXTERNAL_CONSUMER_COPIED_OUT=0 $(GO_COMMAND_ENV) $(GO) test -count=1 -v ./...

panicnil:
	GODEBUG=panicnil=1 $(GO_COMMAND_ENV) $(GO) test -count=1 ./...
	@set -eu; for directory in $(PUBLISHED_COMPONENT_DIRS); do cd "$$directory" && GODEBUG=panicnil=1 $(GO_COMMAND_ENV) $(GO) test -count=1 ./...; cd - >/dev/null; done
	cd integration && GODEBUG=panicnil=1 $(GO_COMMAND_ENV) $(GO) test -count=1 ./...

vet:
	$(GO_COMMAND_ENV) $(GO) vet ./...
	@set -eu; for directory in $(PUBLISHED_COMPONENT_DIRS); do cd "$$directory" && $(GO_COMMAND_ENV) $(GO) vet ./...; cd - >/dev/null; done
	cd integration && $(GO_COMMAND_ENV) $(GO) vet ./...
	cd $(CONSUMER_DIR) && $(GO_COMMAND_ENV) $(GO) vet ./...

vulncheck:
	@set -eu; \
	scanner_directory="$$(mktemp -d /tmp/modary-govulncheck.XXXXXX)"; \
	trap 'rm -rf "$$scanner_directory"' EXIT HUP INT TERM; \
	GOBIN="$$scanner_directory" $(GO_COMMAND_ENV) $(GO) install golang.org/x/vuln/cmd/govulncheck@v1.6.0; \
	scanner="$$scanner_directory/govulncheck"; \
	"$$scanner" ./...; \
	for directory in $(PUBLISHED_COMPONENT_DIRS); do (cd "$$directory" && "$$scanner" ./...); done; \
	(cd integration && "$$scanner" ./...); \
	(cd $(CONSUMER_DIR) && "$$scanner" ./...)

race:
	$(GO_COMMAND_ENV) $(GO) test -count=1 -race ./...
	@set -eu; for directory in $(PUBLISHED_COMPONENT_DIRS); do cd "$$directory" && $(GO_COMMAND_ENV) $(GO) test -count=1 -race ./...; cd - >/dev/null; done
	cd integration && $(GO_COMMAND_ENV) $(GO) test -count=1 -race ./...
	cd $(CONSUMER_DIR) && MODARY_EXTERNAL_CONSUMER_COPIED_OUT=1 $(GO_COMMAND_ENV) $(GO) test -count=1 -race ./...

repeat:
	@set -eu; \
	packages='$(REPEAT_PACKAGES)'; \
	if test "$$($(GO_COMMAND_ENV) $(GO) env GOOS)" = darwin; then \
		packages="$$packages ./internal/filepolicy"; \
	fi; \
	$(GO_COMMAND_ENV) $(GO) test -timeout=$(REPEAT_TIMEOUT) -shuffle=on -count=20 $$packages
	$(GO_COMMAND_ENV) $(GO) test -timeout=$(REPEAT_TIMEOUT) -shuffle=on -count=20 ./starter -run='$(REPEAT_STARTER_TESTS)'
	cd components/postgres && $(GO_COMMAND_ENV) $(GO) test -timeout=$(REPEAT_TIMEOUT) -shuffle=on -count=5 .
	cd components/postgres && $(GO_COMMAND_ENV) $(GO) test -timeout=$(REPEAT_TIMEOUT) -shuffle=on -count=5 ./identitystore -run='$(REPEAT_IDENTITY_TESTS)'
	cd components/postgres && $(GO_COMMAND_ENV) $(GO) test -timeout=$(REPEAT_TIMEOUT) -shuffle=on -count=5 ./rbac -run='$(REPEAT_RBAC_TESTS)'
	cd components/postgres && $(GO_COMMAND_ENV) $(GO) test -timeout=$(REPEAT_TIMEOUT) -shuffle=on -count=5 ./sqlaudit -run='$(REPEAT_AUDIT_TESTS)'
	cd components/governedpostgres && $(GO_COMMAND_ENV) $(GO) test -timeout=$(REPEAT_TIMEOUT) -shuffle=on -count=5 . -run='$(REPEAT_GOVERNED_TESTS)'
	cd integration && $(GO_COMMAND_ENV) $(GO) test -timeout=$(REPEAT_TIMEOUT) -shuffle=on -count=5 ./...
	cd $(CONSUMER_DIR) && MODARY_EXTERNAL_CONSUMER_COPIED_OUT=1 $(GO_COMMAND_ENV) $(GO) test -timeout=$(REPEAT_TIMEOUT) -shuffle=on -count=10 ./... -run='$(REPEAT_CONSUMER_TESTS)'

fuzz-smoke:
	$(GO_COMMAND_ENV) $(GO) test ./projecttool -run='^$$' -fuzz=FuzzParseManifestFailsClosed -fuzztime=$(FUZZ_SMOKE_EXECUTIONS) -parallel=1
	$(GO_COMMAND_ENV) $(GO) test ./internal/jsonvalue -run='^$$' -fuzz=FuzzDecodeFailsClosed -fuzztime=$(FUZZ_SMOKE_EXECUTIONS) -parallel=1
	$(GO_COMMAND_ENV) $(GO) test ./internal/jsonschema -run='^$$' -fuzz=FuzzCompileAndValidateFlagFailsClosed -fuzztime=$(FUZZ_SMOKE_EXECUTIONS) -parallel=1
	$(GO_COMMAND_ENV) $(GO) test ./transport/httpapi -run='^$$' -fuzz=FuzzProtocolJSONDecodersFailClosed -fuzztime=$(FUZZ_SMOKE_EXECUTIONS) -parallel=1
	@if test "$$($(GO_COMMAND_ENV) $(GO) env GOOS)" = darwin; then \
		$(GO_COMMAND_ENV) $(GO) test ./internal/filepolicy -run='^$$' -fuzz=FuzzParseExtendedSecurityResponse -fuzztime=$(FUZZ_SMOKE_EXECUTIONS) -parallel=1; \
		$(GO_COMMAND_ENV) $(GO) test ./internal/filepolicy -run='^$$' -fuzz=FuzzParseKauthFileSecurity -fuzztime=$(FUZZ_SMOKE_EXECUTIONS) -parallel=1; \
	fi

build:
	$(GO_COMMAND_ENV) $(GO) build ./...
	@set -eu; for directory in $(PUBLISHED_COMPONENT_DIRS); do cd "$$directory" && $(GO_COMMAND_ENV) $(GO) build ./...; cd - >/dev/null; done
	cd integration && $(GO_COMMAND_ENV) $(GO) build ./...
	cd $(CONSUMER_DIR) && $(GO_COMMAND_ENV) $(GO) build ./...

cross-build:
	@set -eu; \
	for target in $(CROSS_BUILD_TARGETS); do \
		goos="$${target%/*}"; goarch="$${target#*/}"; \
		GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 $(GO_COMMAND_ENV) $(GO) build ./...; \
		for directory in $(PUBLISHED_COMPONENT_DIRS); do (cd "$$directory" && GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 $(GO_COMMAND_ENV) $(GO) build ./...); done; \
		(cd integration && GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 $(GO_COMMAND_ENV) $(GO) build ./...); \
		(cd $(CONSUMER_DIR) && GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 $(GO_COMMAND_ENV) $(GO) build ./...); \
	done; \
	cross_test_dir="$$(mktemp -d /tmp/modary-cross-tests.XXXXXX)"; \
	trap 'rm -rf "$$cross_test_dir"' EXIT HUP INT TERM; \
	for goarch in amd64 arm64; do \
		GOOS=windows GOARCH="$$goarch" CGO_ENABLED=0 $(GO_COMMAND_ENV) $(GO) test -c -o "$$cross_test_dir/appcmd-$$goarch.test.exe" ./appcmd; \
		GOOS=windows GOARCH="$$goarch" CGO_ENABLED=0 $(GO_COMMAND_ENV) $(GO) test -c -o "$$cross_test_dir/projecttool-$$goarch.test.exe" ./projecttool; \
		(cd components/governedpostgres && GOOS=windows GOARCH="$$goarch" CGO_ENABLED=0 $(GO_COMMAND_ENV) $(GO) test -c -o "$$cross_test_dir/governedpostgres-$$goarch.test.exe" .); \
		GOOS=darwin GOARCH="$$goarch" CGO_ENABLED=0 $(GO_COMMAND_ENV) $(GO) test -c -o "$$cross_test_dir/filepolicy-$$goarch.test" ./internal/filepolicy; \
	done

native-platform: format-check tidy-check test panicnil vet build race fuzz-smoke

copied-admin-acceptance:
	@test -n "$(MODARY_TEST_DATABASE_URL)" || { printf '%s\n' 'MODARY_TEST_DATABASE_URL is required for copied-out Admin acceptance'; exit 2; }
	MODARY_EXTERNAL_ADMIN_ACCEPTANCE=1 MODARY_ACCEPTANCE_GO="$(GO)" MODARY_ACCEPTANCE_PNPM=pnpm $(GO_COMMAND_ENV) $(GO) test -timeout=20m -count=1 -v ./starter -run='^TestCopiedOutAdminProfiles$$'

copied-governed-acceptance:
	@test -n "$(MODARY_TEST_DATABASE_URL)" || { printf '%s\n' 'MODARY_TEST_DATABASE_URL is required for copied-out Governed acceptance'; exit 2; }
	MODARY_ACCEPTANCE_GO="$(GO)" $(GO_COMMAND_ENV) $(GO) test -timeout=15m -count=1 -v ./starter -run='^TestCreateGovernedProfileBuildsAndConsumesTransactionalWork$$'

copied-profile-acceptance: copied-admin-acceptance copied-governed-acceptance

provider-acceptance:
	$(GO_COMMAND_ENV) GO="$(GO)" ./scripts/provider-acceptance.sh

acceptance-core: format-check tidy-check diff-check docs-check react-admin-check admin-frontend test panicnil vet vulncheck verify check-generated neutrality build cross-build

acceptance: acceptance-core copied-profile-acceptance

ci-core-gates: acceptance-core race repeat fuzz-smoke
	$(MAKE) neutrality check-generated diff-check

ci-gates: acceptance race repeat fuzz-smoke
	$(MAKE) neutrality check-generated diff-check

ci-core:
	@set -eu; \
	before="$$(mktemp /tmp/modary-source-before.XXXXXX)"; \
	after="$$(mktemp /tmp/modary-source-after.XXXXXX)"; \
	trap 'rm -f "$$before" "$$after"' EXIT HUP INT TERM; \
	./scripts/source-state.sh >"$$before"; \
	gate_status=0; \
	$(MAKE) ci-core-gates || gate_status=$$?; \
	./scripts/source-state.sh >"$$after"; \
	if ! cmp -s "$$before" "$$after"; then \
		printf '%s\n' 'CI core gates modified repository source state:' >&2; \
		diff -u "$$before" "$$after" >&2 || true; \
		exit 1; \
	fi; \
	exit "$$gate_status"

ci:
	@set -eu; \
	before="$$(mktemp /tmp/modary-source-before.XXXXXX)"; \
	after="$$(mktemp /tmp/modary-source-after.XXXXXX)"; \
	trap 'rm -f "$$before" "$$after"' EXIT HUP INT TERM; \
	./scripts/source-state.sh >"$$before"; \
	gate_status=0; \
	$(MAKE) ci-gates || gate_status=$$?; \
	./scripts/source-state.sh >"$$after"; \
	if ! cmp -s "$$before" "$$after"; then \
		printf '%s\n' 'CI gates modified repository source state:' >&2; \
		diff -u "$$before" "$$after" >&2 || true; \
		exit 1; \
	fi; \
	exit "$$gate_status"

release-preflight: format-check tidy-check diff-check docs-check
	@test -n "$(VERSION)" || { printf '%s\n' 'VERSION is required'; exit 2; }
	./scripts/release-preflight.sh "$(VERSION)" "$(RELEASE_MODE)"

release-readiness: release-preflight
	$(MAKE) ci
	$(MAKE) provider-acceptance
	$(MAKE) container-acceptance VERSION="$(VERSION)"

container-acceptance:
	@test -n "$(VERSION)" || { printf '%s\n' 'VERSION is required'; exit 2; }
	@test -n "$(MODARY_TEST_DATABASE_URL)" || { printf '%s\n' 'MODARY_TEST_DATABASE_URL is required'; exit 2; }
	$(GO_COMMAND_ENV) GO="$(GO)" MODARY_TEST_DATABASE_URL="$(MODARY_TEST_DATABASE_URL)" MODARY_CONTAINER_ACCEPTANCE_MODE=source ./scripts/released-container-acceptance.sh "$(VERSION)"

remote-consumer:
	@test -n "$(VERSION)" || { printf '%s\n' 'VERSION is required'; exit 2; }
	./scripts/remote-consumer.sh "$(VERSION)"

released-container-acceptance:
	@test -n "$(VERSION)" || { printf '%s\n' 'VERSION is required'; exit 2; }
	@test -n "$(MODARY_TEST_DATABASE_URL)" || { printf '%s\n' 'MODARY_TEST_DATABASE_URL is required'; exit 2; }
	$(GO_COMMAND_ENV) GO="$(GO)" MODARY_TEST_DATABASE_URL="$(MODARY_TEST_DATABASE_URL)" ./scripts/released-container-acceptance.sh "$(VERSION)"

clean:
	rm -rf $(CONSUMER_DIR)/dist $(CONSUMER_DIR)/data coverage
