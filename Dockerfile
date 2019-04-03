FROM registry.svc.ci.openshift.org/ocp/builder:golang-1.10 AS builder
WORKDIR /go/src/github.com/openshift/support-operator
COPY . .
RUN make build

FROM registry.svc.ci.openshift.org/ocp/4.0:base
COPY --from=builder /go/src/github.com/openshift/support-operator/bin/support-operator /usr/bin/
COPY config/pod.yaml /etc/support-operator/server.yaml
COPY manifests /manifests
ENTRYPOINT ["/usr/bin/support-operator"]
