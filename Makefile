GOCMD ?= go
GORUN ?= $(GOCMD) run
GOBUILD ?= $(GOCMD) build
GOBUILDFLAGS ?= -mod=vendor -ldflags "-X k8s.io/client-go/pkg/version.gitCommit=$$(git rev-parse HEAD) -X k8s.io/client-go/pkg/version.gitVersion=v1.0.0+$$(git rev-parse --short=7 HEAD)"
GOBUILDDEBUGFLAGS ?= -gcflags="all=-N -l"
GOTEST ?= $(GOCMD) test
GOGET ?= $(GOCMD) get
GOMOD ?= $(GOCMD) mod
GOFMT ?= gofmt
GOLINT ?= golint
CONTAINER_RUNTIME ?= podman

.PHONY: run
run:
	$(GORUN) ./cmd/insights-operator/main.go start \
		--config=config/local.yaml \
		--kubeconfig=$(KUBECONFIG) \
		-v4

.PHONY: build
build:
	$(GOBUILD) $(GOBUILDFLAGS) -o bin/insights-operator ./cmd/insights-operator

build-debug:
	$(GOBUILD) $(GOBUILDFLAGS) $(GOBUILDDEBUGFLAGS) -o bin/insights-operator ./cmd/insights-operator

build-debug-container:
	$(CONTAINER_RUNTIME) build -t insights-operator -f debug.Dockerfile .

.PHONY: test-unit
test-unit:
	$(GOTEST) $$(go list ./... | grep -v /tests/) $(TEST_OPTIONS)

vet:
	@echo ">> vetting code"
	$(GOCMD) vet $$(go list ./... | egrep -v '/vendor/|/tests/integration')

lint:
	@echo ">> linting code"
	$(GOLINT) $$(go list ./... | egrep -v '/vendor/|/tests/integration') 

.PHONY: gen-doc
gen-doc:
	@echo ">> generating documentation"
	$(GORUN) cmd/gendoc/main.go --out=docs/gathered-data.md

.PHONY: vendor
vendor:
	$(GOMOD) tidy
	$(GOMOD) vendor
	$(GOMOD) verify
