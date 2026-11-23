TOOLS_BIN_DIR := $(CURDIR)/bin
BINARY        := $(TOOLS_BIN_DIR)/insights-operator-tests

GIT_COMMIT     := $(shell git rev-parse --short HEAD)
BUILD_DATE     := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
GIT_TREE_STATE := $(shell if git diff --quiet; then echo clean; else echo dirty; fi)

LDFLAGS := -X 'github.com/openshift-eng/openshift-tests-extension/pkg/version.CommitFromGit=$(GIT_COMMIT)' \
           -X 'github.com/openshift-eng/openshift-tests-extension/pkg/version.BuildDate=$(BUILD_DATE)' \
           -X 'github.com/openshift-eng/openshift-tests-extension/pkg/version.GitTreeState=$(GIT_TREE_STATE)'

# GO_COMPLIANCE_POLICY="exempt_all" must only be used for test related binaries.
# It prevents various FIPS compliance policies from being applied to this compilation.
.PHONY: build
build: ## Build the OpenShift Tests Extension binary
	@mkdir -p $(TOOLS_BIN_DIR)
	CGO_ENABLED=0 GO_COMPLIANCE_POLICY="exempt_all" go build -mod=vendor -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/...

.PHONY: list-test-names
list-test-names: build ## List all registered test names
	@$(BINARY) list -o names

# update-metadata regenerates
# .openshift-tests-extension/openshift_payload_insights-operator.json, which
# records the name and lifecycle of every test the binary ships. Run this
# after adding, renaming or removing a test.
.PHONY: update-metadata
update-metadata: build ## Regenerate the registered-test metadata
	$(BINARY) update

# verify-metadata fails if the committed metadata is stale with respect to the
# tests in the binary. It checks for untracked files as well as modifications,
# so newly generated metadata is not mistaken for up-to-date metadata.
.PHONY: verify-metadata
verify-metadata: update-metadata ## Fail if the committed metadata is stale
	@if [ -n "$$(git status --porcelain -- .openshift-tests-extension)" ]; then \
		echo "ERROR: .openshift-tests-extension is out of date." >&2; \
		echo "Run 'make update-metadata' and commit the result." >&2; \
		git --no-pager diff -- .openshift-tests-extension; \
		exit 1; \
	fi

.PHONY: test
test: ## Run unit tests (e.g. smoke specs that don't need a cluster)
	go test -mod=vendor ./...

.PHONY: test-integration
test-integration: ## Run the integration suite against a live cluster (requires KUBECONFIG)
	@if [ -z "$$KUBECONFIG" ]; then \
		echo "Error: KUBECONFIG environment variable must be set"; \
		exit 1; \
	fi
	go test -mod=vendor -v ./... -ginkgo.v -ginkgo.show-node-events

.PHONY: test-integration-focus
test-integration-focus: ## Run a specific integration test (use FOCUS="test pattern")
	@if [ -z "$$KUBECONFIG" ]; then \
		echo "Error: KUBECONFIG environment variable must be set"; \
		exit 1; \
	fi
	@if [ -z "$$FOCUS" ]; then \
		echo "Error: FOCUS environment variable must be set"; \
		exit 1; \
	fi
	go test -mod=vendor -v ./... -ginkgo.focus="$(FOCUS)" -ginkgo.v

# SUITE defaults to insights-operator/all; override with SUITE=insights-operator/gathering etc.
SUITE ?= insights-operator/all

.PHONY: run-suite
run-suite: build ## Build the OTE binary and run a suite against a live cluster (requires KUBECONFIG; override SUITE=...)
	@if [ -z "$$KUBECONFIG" ]; then \
		echo "Error: KUBECONFIG environment variable must be set"; \
		exit 1; \
	fi
	$(BINARY) run-suite $(SUITE)

.PHONY: verify
verify: tidy fmt vet test verify-metadata ## Run all verification checks

.PHONY: tidy
tidy: ## Run go mod tidy, vendor and verify
	go mod tidy
	go mod vendor
	go mod verify

.PHONY: fmt
fmt: ## Check formatting
	@files=$$(find . -name '*.go' -not -path './vendor/*' -exec gofmt -l {} \;); \
	if [ -n "$$files" ]; then \
		echo "Files that need formatting:"; \
		echo "$$files"; \
		echo "Run: find . -name '*.go' -not -path './vendor/*' -exec gofmt -w {} \\;"; \
		exit 1; \
	fi

.PHONY: vet
vet: ## Run go vet
	go vet -mod=vendor ./...
