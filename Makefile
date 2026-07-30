.PHONY: bootstrap generate action-catalog verify test test-go test-go-strict test-performance test-web test-e2e build release-linux acceptance release-acceptance run clean

bootstrap:
	pnpm install
	go mod download

generate:
	go run ./cmd/modary generate

action-catalog:
	MODARY_DATA_DIR=/tmp/modary-action-catalog MODARY_DATABASE_PATH=/tmp/modary-action-catalog/modary.db go run ./cmd/modary action catalog --output internal/generated/action_schemas.json

verify:
	go run ./cmd/modary verify

test: test-go test-web

test-go:
	go test ./...

test-go-strict:
	go vet ./...
	go test -race ./...

test-performance:
	GOMAXPROCS=2 go test ./tests/integration -run '^TestPreviewPerformance1000Rows$$' -count=1 -v

test-web:
	pnpm --filter @modary/console test
	pnpm --filter @modary/console typecheck

test-e2e:
	pnpm --filter @modary/console test:e2e

build:
	pnpm --filter @modary/console build
	go run ./cmd/modary generate
	mkdir -p dist
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/modary-rulary ./cmd/modary

release-linux: build
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/modary-rulary-linux-amd64 ./cmd/modary
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o dist/modary-rulary-linux-arm64 ./cmd/modary

acceptance: verify generate action-catalog test test-e2e build

release-acceptance: acceptance test-go-strict test-performance release-linux
	docker build --platform linux/arm64 -f Dockerfile.release -t modary-f0:arm64 .
	docker build --platform linux/amd64 -f Dockerfile.release -t modary-f0:amd64 .
	./scripts/benchmark-release.zsh modary-f0:arm64 linux/arm64
	PORT=18083 ./scripts/benchmark-release.zsh modary-f0:amd64 linux/amd64
	./scripts/benchmark-preview.zsh modary-f0:amd64 linux/amd64
	./scripts/release-smoke.zsh modary-f0:arm64 linux/arm64

run:
	go run ./cmd/modary serve

clean:
	rm -rf dist web/dist
