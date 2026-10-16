.PHONY: build test race vet lint vuln fuzz check fmt toolchain

# go.mod is the single exact toolchain pin. Force that release even when the
# launcher is newer; normal Go toolchain download is allowed when it is absent.
GO_TOOLCHAIN := $(shell awk '$$1 == "go" { print "go" $$2; exit }' go.mod)
export GOTOOLCHAIN := $(GO_TOOLCHAIN)

build: toolchain
	go build -trimpath -o bin/atuin-ai-server ./cmd/atuin-ai-server

test: toolchain
	go test ./...

race: toolchain
	go test -race ./...

vet: toolchain
	go vet ./...

lint: toolchain
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 ./...
	# Staticcheck 2026.2 retired SA5011 upstream; retain nil-dereference analysis.
	go run golang.org/x/tools/go/analysis/passes/nilness/cmd/nilness@v0.52.0 ./...

vuln: toolchain
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...

fuzz: toolchain
	go test ./internal/atuin -run='^$$' -fuzz=FuzzDecode -fuzztime=5s -parallel=2
	go test ./internal/tools -run='^$$' -fuzz=FuzzValidateBatch -fuzztime=5s -parallel=2
	go test ./internal/safety -run='^$$' -fuzz=FuzzEscalate -fuzztime=5s -parallel=2
	go test ./internal/provider/codex -run='^$$' -fuzz=FuzzResponsesSSE -fuzztime=5s -parallel=2

check: toolchain test race vet lint vuln fuzz

fmt: toolchain
	go fmt ./cmd/... ./internal/...

toolchain:
	@test "$$(go env GOVERSION)" = "$(GO_TOOLCHAIN)"
