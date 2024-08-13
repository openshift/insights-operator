# Use go build instead of make build to target linux/amd64
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o ./bin/insights-operator -ldflags="${GO_LDFLAGS}" ./cmd/insights-operator/main.go

podman build --platform linux/amd64 -t quay.io/jmesnil/insights-operator-with-runtime -f Dockerfile-insights-operator-with-runtime-extractor .
podman push quay.io/jmesnil/insights-operator-with-runtime

oc apply -f ./manifests/06a-deployment-with-runtime.yaml
oc apply -f ./manifests/010-clusterrole-insights-runtime-extractor.yaml
oc apply -f ./manifests/010-insights-runtime-extractor-service.yaml
oc apply -f ./manifests/010-insights-runtime-extractor-proxy-configmap.yaml
oc apply -f ./manifests/010-insights-runtime-extractor.yaml
