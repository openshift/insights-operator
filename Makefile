# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Testing
GO_TEST_FLAGS = $(VERBOSE)
COVER_PROFILE = cover.out

# Build
CLIENTGO_VERSION := $(shell git rev-parse --short=7 HEAD)
CLIENTGO_COMMIT := "v1.0.0+$(shell git rev-parse HEAD)"
export LDFLAGS="-X k8s.io/client-go/pkg/version.gitCommit=${CLIENTGO_COMMIT} \
			-X k8s.io/client-go/pkg/version.gitVersion=${CLIENTGO_VERSION}"

# Configuration
KUBECONFIG ?= config/local.yaml
RUN_FLAGS ?= -v4

# Tools
CONTAINER_RUNTIME := $(shell command -v podman 2> /dev/null || echo docker)
GOLANGCI_LINT := $(GOBIN)/golangci-lint

export GO111MODULE=on
export GOFLAGS=-mod=vendor

.PHONY: help
help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

## --------------------------------------
## Tests
## --------------------------------------

# Run the tests
.PHONY: test
test: unit ## Run all the tests

# Run the unit tests
.PHONY: unit
unit: ## Run the unit tests
	go test ./... $(VERBOSE) -coverprofile $(COVER_PROFILE)

.PHONY: unit-cover
unit-cover:
	go test -coverprofile=$(COVER_PROFILE) $(GO_TEST_FLAGS) ./...
	go tool cover -func=$(COVER_PROFILE)

.PHONE: unit-verbose
unit-verbose:
	VERBOSE=-v make unit

## --------------------------------------
## Linting
## --------------------------------------

.PHONY: lint
lint: $(GOLANGCI_LINT) ## Executes the linting tool (vet, sec, and others)
	$(GOLANGCI_LINT) run

$(GOLANGCI_LINT):
	./.openshiftci/install-golangci-lint.sh

## --------------------------------------
## Build/Run
## --------------------------------------

.PHONY: run
run: ## Executes the insights operator
	go run ./cmd/insights-operator/main.go start \
		--config=$(KUBECONFIG) \
		$(RUN_FLAGS)

.PHONY: build
build: ## Compiles the insights operator
	go build -o ./bin/insights-operator ./cmd/insights-operator

.PHONY: build-debug
build-debug: ## Compiles the insights operator in debug mode
	go build $(GO_BUILD_FLAGS) -gcflags="all=-N -l" \
		-o ./bin/insights-operator ./cmd/insights-operator

## --------------------------------------
## Container
## --------------------------------------

.PHONY build-debug-container:
build-debug-container: ## Compiles the insights operator and its container image for debug
	$(CONTAINER_RUNTIME) build -t insights-operator -f ./Dockerfile.debug ../.

## --------------------------------------
## Tools
## --------------------------------------

.PHONY: docs
docs: ## Generate the documentation
	go run ./cmd/gendoc/main.go --out=./docs/gathered-data.md

.PHONY: changelog
changelog: check-github-token ## Updates the changelog entries
	go run ./cmd/changelog/main.go $(CHANGELOG_FROM) $(CHANGELOG_UNTIL)

## --------------------------------------
## Go Module
## --------------------------------------

.PHONY: vendor
vendor: ## Runs tiny, vendor and verify the module
	go mod tidy
	go mod vendor
	go mod verify

## --------------------------------------
## Checks (mostly "private" targets)
## --------------------------------------

.PHONY: check-github-token
check-github-token:
ifndef GITHUB_TOKEN
	$(error GITHUB_TOKEN is undefined)
endif
