build:
	go build -ldflags "-X github.com/openshift/insights-operator/vendor/k8s.io/client-go/pkg/version.gitCommit=$$(git rev-parse HEAD) -X github.com/openshift/insights-operator/vendor/k8s.io/client-go/pkg/version.gitVersion=v1.0.0+$$(git rev-parse --short=7 HEAD)" -o bin/insights-operator ./cmd/insights-operator
.PHONY: build

test-unit:
	go test $$(go list ./... | grep -v /test/) $(TEST_OPTIONS)
.PHONY: test-unit

test-e2e:
	go test ./test/integration -timeout 2h $(TEST_OPTIONS)
.PHONY: test-e2e

vet:
	go vet $$(go list ./... | grep -v /vendor/)

lint:
	golint $$(go list ./... | grep -v /vendor/)

gen-doc:
	go run cmd/gendoc/main.go --out=docs/gathered-data.md

vendor:
	go mod tidy
	go mod vendor
	go mod verify
.PHONY: vendor
