FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.17-openshift-4.11 AS builder
RUN go get github.com/go-delve/delve/cmd/dlv
WORKDIR /go/src/github.com/openshift/insights-operator
COPY . .
RUN make build-debug

FROM registry.ci.openshift.org/ocp/4.11:base
COPY --from=builder /go/src/github.com/openshift/insights-operator/bin/insights-operator /usr/bin/
COPY --from=builder /usr/bin/dlv /usr/bin/
COPY config/pod.yaml /etc/insights-operator/server.yaml
COPY manifests /manifests
LABEL io.openshift.release.operator=true
EXPOSE 40000 40000
ENTRYPOINT ["dlv", "--listen=:40000", "--headless=true", "--api-version=2", "--accept-multiclient", "exec", "/usr/bin/insights-operator", "--"]
