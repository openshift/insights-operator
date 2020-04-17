# insights-operator

This cluster operator gathers anonymized system configuration and reports it to Red Hat Insights. It is a part of the standard OpenShift distribution. The data collected allows for debugging in the event of cluster failures or unanticipated errors.

## Reported data

* ClusterVersion
* ClusterOperator objects
* All non-secret global config (hostnames and URLs anonymized)

The list of all collected data with description, location in produced archive and link to Api and some examples is at [docs/gathered-data.md](docs/gathered-data.md)

The resulting data is packed in .tar.gz archive with folder structure indicated in the document. Example of such archive is at [docs/insights-archive-sample](docs/insights-archive-sample).

## Building

To build the operator, install Go 1.11 or above and run:

    make build

To test the operator against a remote cluster, run:

    bin/insights-operator start --config=config/local.yaml --kubeconfig=$KUBECONFIG

where `$KUBECONFIG` has sufficiently high permissions against the target cluster.

## Roadmap

The current operator only collects global configuration. Future revisions will expand the set of config that can be gathered as well as add on-demand capture.

## Contributing

Please make sure to run `make test` to check all changes made in the source code.

## Testing

Unit tests can be started by the following command:

    make test

It is also possible to specify CLI options for Go test. For example, if you need to disable test results caching, use the following command:

    make test TEST_OPTIONS=-count=1

## Issue Tracking

Insights Operator is part of Red Hat OpenShift Container Platform. For product-related issues, please
file a ticket [in Red Hat Bugzilla](https://bugzilla.redhat.com/enter_bug.cgi?product=OpenShift%20Container%20Platform&component=Insights%20Operator) for "Insights Operator" component.

## Generating the document with gathered data
The document docs/gathered-data contains list of collected data, the Api and some examples. The document is generated from package sources by looking for Gather... methods.
If for any GatherXXX method exists its method which returns example with name ExampleXXX, the generated example is added to document with the size in bytes.


To start generating the document run:
```
make gen-doc
```
