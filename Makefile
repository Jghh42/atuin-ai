.PHONY: build test race vet lint vuln check fmt

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

check: test race vet lint vuln

fmt:
	gofmt -w cmd internal
