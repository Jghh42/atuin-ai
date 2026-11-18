.PHONY: build test race vet lint vuln fuzz check fmt fmt-check toolchain e2e differential compatibility packaging-test package package-dev resource resource-release resource-test resource-isolation-test reasoning-e2e reasoning-resource reasoning-harness-test

# go.mod is the single exact toolchain pin. Force that release even when the
# launcher is newer; normal Go toolchain download is allowed when it is absent.
GO_TOOLCHAIN := $(shell awk '$$1 == "go" { print "go" $$2; exit }' go.mod)
export GOTOOLCHAIN := $(GO_TOOLCHAIN)

# Explicit roots keep ignored package output (dist/, which can hold extracted
# source trees) out of tests, vet, lint and vulnerability scans.
PACKAGES := ./cmd/... ./internal/... ./tests/...

build: toolchain
	go build -trimpath -o bin/atuin-ai-server ./cmd/atuin-ai-server

test: toolchain
	go test $(PACKAGES)

race: toolchain
	go test -race $(PACKAGES)

vet: toolchain
	go vet $(PACKAGES)

lint: toolchain
	go run honnef.co/go/tools/cmd/staticcheck@v0.8.2 $(PACKAGES)
	# Staticcheck 2026.2 retired SA5011 upstream; retain nil-dereference analysis.
	go run golang.org/x/tools/go/analysis/passes/nilness/cmd/nilness@v0.52.0 $(PACKAGES)

vuln: toolchain
	go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 $(PACKAGES)

fuzz: toolchain
	go test ./internal/atuin -run='^$$' -fuzz=FuzzDecode -fuzztime=5s -parallel=2
	go test ./internal/tools -run='^$$' -fuzz=FuzzValidateBatch -fuzztime=5s -parallel=2
	go test ./internal/safety -run='^$$' -fuzz=FuzzEscalate -fuzztime=5s -parallel=2
	go test ./internal/provider/codex -run='^$$' -fuzz=FuzzResponsesSSE -fuzztime=5s -parallel=2

check: toolchain fmt-check test race vet lint vuln fuzz packaging-test resource-test reasoning-harness-test

fmt: toolchain
	go fmt $(PACKAGES)

fmt-check: toolchain
	test -z "$$(gofmt -l cmd internal tests)"

# Opt-in release gates: missing prerequisites FAIL rather than green skip.
e2e: toolchain
	@test -n "$$ATUIN_E2E_MANIFEST" || { echo 'BLOCKED: build pinned Atuin with tests/e2e/build-atuin.py; set ATUIN_E2E_MANIFEST'; exit 1; }
	python3 -B -I tests/harness_test.py Confinement
	go test -v ./tests/e2e -run '^TestPinnedCLI$$' -count=1 -timeout=10m

differential: toolchain
	@test -n "$$ATUIN_ORACLE_MANIFEST" || { echo 'BLOCKED: build pinned oracle with tests/differential/build-oracle.py; set ATUIN_ORACLE_MANIFEST'; exit 1; }
	go test -v ./tests/differential -run '^TestOriginalReferenceOracle$$' -count=1 -timeout=5m

compatibility: e2e differential

reasoning-harness-test:
	python3 -B -I tests/reasoning/sampler_test.py

# Separate enabled-reasoning proofs; never alter the CLI20/oracle14 contracts.
reasoning-e2e: build
	@test -n "$$ATUIN_E2E_MANIFEST" -a -n "$(OUT)" || { echo 'BLOCKED: pinned ATUIN_E2E_MANIFEST and fresh OUT required'; exit 1; }
	go build -o bin/reasoning-resourcehelper ./tests/resourcehelper
	ATUIN_REASONING_SERVER="$(abspath bin/atuin-ai-server)" ATUIN_REASONING_HELPER="$(abspath bin/reasoning-resourcehelper)" ATUIN_REASONING_OUT="$(abspath $(OUT))" go test -v ./tests/e2e -run '^TestPinnedReasoningCLI$$' -count=1 -timeout=3m

reasoning-resource: toolchain
	@test -n "$(BINARY)" -a -n "$(CLI)" -a -n "$(OUT)" || { echo 'BLOCKED: explicit development BINARY, pinned CLI and fresh OUT required'; exit 1; }
	go build -o bin/reasoning-resourcehelper ./tests/resourcehelper
	python3 -B -I tests/reasoning/run.py --stress --server "$(BINARY)" --helper "$(abspath bin/reasoning-resourcehelper)" --cli "$(CLI)" --out "$(OUT)"

# Fresh ignored output directory required; nothing publishes or creates tags.
# VERSION must be an existing exact-HEAD tag for clean release-mode packaging.
packaging-test:
	python3 -B -I tests/packaging_test.py

# Explicit hard resource gates: default DEVELOPMENT only; resource-release opts
# into independently verified CLEAN TAGGED artifacts. Same namespace/budgets.
resource-test:
	python3 -B -I tests/resources/resource_test.py

resource-isolation-test:
	python3 -B -I tests/resources/namespace_test.py

resource resource-release: toolchain resource-test resource-isolation-test
	@test -n "$(ARTIFACTS)" -a -n "$(OUT)" || { echo 'BLOCKED: set ARTIFACTS to the target-selected verified package and OUT to fresh evidence directory'; exit 1; }
	python3 -B -I tests/resources/run.py $(if $(filter resource-release,$@),--release) --artifacts "$(ARTIFACTS)" --out "$(OUT)"

package: toolchain
	sh scripts/package.sh --version "$(VERSION)" --out "$(OUT)"

package-dev: toolchain
	sh scripts/package.sh --version "$(VERSION)" --out "$(OUT)" --development

toolchain:
	@test "$$(go env GOVERSION)" = "$(GO_TOOLCHAIN)"
