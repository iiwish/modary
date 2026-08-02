GO ?= go
RELEASE_MODE ?= candidate
CONSUMER_DIR := examples/counter
ADMIN_WEB_DIR := starter/templates/admin/web
GO_COMMAND_ENV := GO111MODULE=on GOTOOLCHAIN=local GOENV=off GOWORK=off GOFLAGS=
CROSS_BUILD_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64
REPEAT_PACKAGES := \
	./action \
	./adapters/... \
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
	./starter \
	./task \
	./transport/httpapi

.PHONY: bootstrap format-check tidy-check diff-check docs-check react-admin-check admin-frontend verify check-generated neutrality \
	test test-framework test-consumer vet race repeat fuzz-smoke build \
	panicnil vulncheck cross-build native-platform acceptance ci-gates ci \
	release-preflight release-readiness remote-consumer clean

bootstrap:
	$(GO_COMMAND_ENV) $(GO) mod download
	cd $(CONSUMER_DIR) && $(GO_COMMAND_ENV) $(GO) mod download

format-check:
	@unformatted="$$(git ls-files --cached --others --exclude-standard -z -- '*.go' | xargs -0 sh -c 'for file do if test -f "$$file"; then gofmt -l "$$file"; fi; done' sh)"; \
	if test -n "$$unformatted"; then printf 'gofmt required:\n%s\n' "$$unformatted"; exit 1; fi

tidy-check:
	$(GO_COMMAND_ENV) $(GO) mod tidy -diff
	cd $(CONSUMER_DIR) && $(GO_COMMAND_ENV) $(GO) mod tidy -diff

diff-check:
	./scripts/check-source-diff.sh

docs-check:
	./scripts/check-docs.sh
	./scripts/check-doc-links.sh

react-admin-check:
	./scripts/check-react-admin.sh

admin-frontend:
	cd $(ADMIN_WEB_DIR) && pnpm install --frozen-lockfile
	cd $(ADMIN_WEB_DIR) && pnpm lint
	cd $(ADMIN_WEB_DIR) && pnpm typecheck
	cd $(ADMIN_WEB_DIR) && pnpm test
	cd $(ADMIN_WEB_DIR) && pnpm build
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

test-consumer:
	cd $(CONSUMER_DIR) && MODARY_EXTERNAL_CONSUMER_COPIED_OUT=0 $(GO_COMMAND_ENV) $(GO) test -count=1 -v ./...

panicnil:
	GODEBUG=panicnil=1 $(GO_COMMAND_ENV) $(GO) test -count=1 ./...

vet:
	$(GO_COMMAND_ENV) $(GO) vet ./...
	cd $(CONSUMER_DIR) && $(GO_COMMAND_ENV) $(GO) vet ./...

vulncheck:
	$(GO_COMMAND_ENV) $(GO) run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
	cd $(CONSUMER_DIR) && $(GO_COMMAND_ENV) $(GO) run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...

race:
	$(GO_COMMAND_ENV) $(GO) test -count=1 -race ./...
	cd $(CONSUMER_DIR) && MODARY_EXTERNAL_CONSUMER_COPIED_OUT=1 $(GO_COMMAND_ENV) $(GO) test -count=1 -race ./...

repeat:
	@set -eu; \
	packages='$(REPEAT_PACKAGES)'; \
	if test "$$($(GO_COMMAND_ENV) $(GO) env GOOS)" = darwin; then \
		packages="$$packages ./internal/filepolicy"; \
	fi; \
	$(GO_COMMAND_ENV) $(GO) test -shuffle=on -count=20 $$packages
	cd $(CONSUMER_DIR) && MODARY_EXTERNAL_CONSUMER_COPIED_OUT=1 $(GO_COMMAND_ENV) $(GO) test -shuffle=on -count=20 ./...

fuzz-smoke:
	$(GO_COMMAND_ENV) $(GO) test ./projecttool -run='^$$' -fuzz=FuzzParseManifestFailsClosed -fuzztime=5s -parallel=1
	$(GO_COMMAND_ENV) $(GO) test ./internal/jsonvalue -run='^$$' -fuzz=FuzzDecodeFailsClosed -fuzztime=5s -parallel=1
	$(GO_COMMAND_ENV) $(GO) test ./internal/jsonschema -run='^$$' -fuzz=FuzzCompileAndValidateFlagFailsClosed -fuzztime=5s -parallel=1
	$(GO_COMMAND_ENV) $(GO) test ./transport/httpapi -run='^$$' -fuzz=FuzzProtocolJSONDecodersFailClosed -fuzztime=5s -parallel=1
	@if test "$$($(GO_COMMAND_ENV) $(GO) env GOOS)" = darwin; then \
		$(GO_COMMAND_ENV) $(GO) test ./internal/filepolicy -run='^$$' -fuzz=FuzzParseExtendedSecurityResponse -fuzztime=5s -parallel=1; \
		$(GO_COMMAND_ENV) $(GO) test ./internal/filepolicy -run='^$$' -fuzz=FuzzParseKauthFileSecurity -fuzztime=5s -parallel=1; \
	fi

build:
	$(GO_COMMAND_ENV) $(GO) build ./...
	cd $(CONSUMER_DIR) && $(GO_COMMAND_ENV) $(GO) build ./...

cross-build:
	@set -eu; \
	for target in $(CROSS_BUILD_TARGETS); do \
		goos="$${target%/*}"; goarch="$${target#*/}"; \
		GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 $(GO_COMMAND_ENV) $(GO) build ./...; \
		(cd $(CONSUMER_DIR) && GOOS="$$goos" GOARCH="$$goarch" CGO_ENABLED=0 $(GO_COMMAND_ENV) $(GO) build ./...); \
	done; \
	cross_test_dir="$$(mktemp -d /tmp/modary-cross-tests.XXXXXX)"; \
	trap 'rm -rf "$$cross_test_dir"' EXIT HUP INT TERM; \
	for goarch in amd64 arm64; do \
		GOOS=windows GOARCH="$$goarch" CGO_ENABLED=0 $(GO_COMMAND_ENV) $(GO) test -c -o "$$cross_test_dir/appcmd-$$goarch.test.exe" ./appcmd; \
		GOOS=windows GOARCH="$$goarch" CGO_ENABLED=0 $(GO_COMMAND_ENV) $(GO) test -c -o "$$cross_test_dir/projecttool-$$goarch.test.exe" ./projecttool; \
		GOOS=windows GOARCH="$$goarch" CGO_ENABLED=0 $(GO_COMMAND_ENV) $(GO) test -c -o "$$cross_test_dir/postgres-$$goarch.test.exe" ./adapters/postgres; \
		GOOS=darwin GOARCH="$$goarch" CGO_ENABLED=0 $(GO_COMMAND_ENV) $(GO) test -c -o "$$cross_test_dir/filepolicy-$$goarch.test" ./internal/filepolicy; \
	done

native-platform: format-check tidy-check test panicnil vet build race fuzz-smoke

acceptance: format-check tidy-check diff-check docs-check react-admin-check admin-frontend test panicnil vet vulncheck verify check-generated neutrality build cross-build

ci-gates: acceptance race repeat fuzz-smoke
	$(MAKE) neutrality check-generated diff-check

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

remote-consumer:
	@test -n "$(VERSION)" || { printf '%s\n' 'VERSION is required'; exit 2; }
	./scripts/remote-consumer.sh "$(VERSION)"

clean:
	rm -rf $(CONSUMER_DIR)/dist $(CONSUMER_DIR)/data coverage
