build:
	go build -ldflags "-X github.com/openshift/support-operator/vendor/k8s.io/client-go/pkg/version.gitCommit=$$(git rev-parse HEAD) -X github.com/openshift/support-operator/vendor/k8s.io/client-go/pkg/version.gitVersion=v1.0.0+$$(git rev-parse --short=7 HEAD)" -o bin/support-operator ./cmd/support-operator
.PHONY: build

test:
	go test ./...
.PHONY: test

vendor:
	glide up -v --skip-test
.PHONY: vendor
