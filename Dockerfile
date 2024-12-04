FROM registry.ci.openshift.org/ocp/builder:rhel-9-golang-1.23-openshift-4.19 AS builder
WORKDIR /go/src/github.com/openshift/insights-operator
COPY . .
ENV GOEXPERIMENT=strictfipsruntime
RUN make build

FROM registry.ci.openshift.org/ocp/4.19:base-rhel9
COPY --from=builder /go/src/github.com/openshift/insights-operator/bin/insights-operator /usr/bin/
COPY config/pod.yaml /etc/insights-operator/server.yaml
COPY manifests /manifests
LABEL io.openshift.release.operator=true
ENTRYPOINT ["/usr/bin/insights-operator"]
