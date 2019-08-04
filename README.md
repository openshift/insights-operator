# insights-operator

This cluster operator gathers anonymized system configuration and reports it to Red Hat Insights. It is a part of the standard OpenShift distribution.

## Building

To build the operator, install Go 1.11 or above and run:

    make build

To test the operator against a remote cluster, run:

    bin/support-operator start --config=config/local.yaml --kubeconfig=$KUBECONFIG

where `$KUBECONFIG` has sufficiently high permissions against the target cluster.

## Roadmap

The current operator only collects global configuration. Future revisions will expand the set of config that can be gathered as well as add on-demand capture.