FROM registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.23-openshift-4.19 AS builder
RUN GOFLAGS='' go install github.com/go-delve/delve/cmd/dlv@v1.24.0
WORKDIR /go/src/github.com/openshift/insights-operator
COPY . .
RUN make build-debug

FROM registry.ci.openshift.org/ocp/4.19:base-rhel9
COPY --from=builder /go/src/github.com/openshift/insights-operator/bin/insights-operator /usr/bin/
COPY --from=builder /usr/bin/dlv /usr/bin/
COPY config/pod.yaml /etc/insights-operator/server.yaml
COPY manifests /manifests
LABEL io.openshift.release.operator=true
EXPOSE 40000 40000
ENTRYPOINT ["dlv", "--listen=:40000", "--headless=true", "--api-version=2", "--accept-multiclient", "exec", "/usr/bin/insights-operator", "--"]