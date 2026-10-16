.PHONY: build test race vet lint vuln fuzz check fmt

build:
	go build -trimpath -o bin/atuin-ai-server ./cmd/atuin-ai-server

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

lint:
	go run honnef.co/go/tools/cmd/staticcheck@v0.7.0 ./...

vuln:
	go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...

fuzz:
	go test ./internal/atuin -run='^$$' -fuzz=FuzzDecode -fuzztime=5s -parallel=2
	go test ./internal/tools -run='^$$' -fuzz=FuzzValidateBatch -fuzztime=5s -parallel=2
	go test ./internal/safety -run='^$$' -fuzz=FuzzEscalate -fuzztime=5s -parallel=2
	go test ./internal/provider/codex -run='^$$' -fuzz=FuzzResponsesSSE -fuzztime=5s -parallel=2

check: test race vet lint vuln fuzz

fmt:
	gofmt -w cmd internal
