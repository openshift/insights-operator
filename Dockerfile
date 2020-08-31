FROM registry.svc.ci.openshift.org/ocp/builder:rhel-8-golang-openshift-4.6 AS builder
WORKDIR /go/src/github.com/openshift/insights-operator
COPY . .
RUN make build

FROM registry.svc.ci.openshift.org/ocp/4.6:base
COPY --from=builder /go/src/github.com/openshift/insights-operator/bin/insights-operator /usr/bin/
COPY config/pod.yaml /etc/insights-operator/server.yaml
COPY manifests /manifests
LABEL io.openshift.release.operator=true
ENTRYPOINT ["/usr/bin/insights-operator"]
