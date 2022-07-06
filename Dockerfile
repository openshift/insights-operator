FROM registry.ci.openshift.org/ocp/builder:rhel-8-golang-1.18-openshift-4.12 AS builder
WORKDIR /go/src/github.com/openshift/insights-operator
COPY . .
RUN make build

FROM registry.ci.openshift.org/ocp/4.12:base
COPY --from=builder /go/src/github.com/openshift/insights-operator/bin/insights-operator /usr/bin/
COPY config/pod.yaml /etc/insights-operator/server.yaml
COPY manifests /manifests
LABEL io.openshift.release.operator=true
ENTRYPOINT ["/usr/bin/insights-operator"]
