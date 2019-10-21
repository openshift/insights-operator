build:
	go build -ldflags "-X github.com/openshift/insights-operator/vendor/k8s.io/client-go/pkg/version.gitCommit=$$(git rev-parse HEAD) -X github.com/openshift/insights-operator/vendor/k8s.io/client-go/pkg/version.gitVersion=v1.0.0+$$(git rev-parse --short=7 HEAD)" -o bin/insights-operator ./cmd/insights-operator
.PHONY: build

test:
	go test ./...
.PHONY: test

vendor:
	go mod tidy
	go mod vendor
	go mod verify
.PHONY: vendor
