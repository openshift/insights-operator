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

## Custom Resource Definitions

Insights Operator manages the following custom resource:

* **ReportOverview:**: resource which store the overview of a report for this cluster from Insights Smart Proxy service. The overview contains the number of
  rule hits grouped by severity: low, moderate, important and critical. This custom resource will be written by the Insights Operator and can be consumed by
  any service in the cluster.

## Accessing Prometheus metrics provided by Insights Operator

It is possible to read Prometheus metrics provided by Insights Operator. For example if the IO runs locally, the following command migth be used:

``
curl --cert k8s.crt --key k8s.key -k https://localhost:8443/metrics
``

### Certificate and key needed to access Prometheus metrics

Certificate and key are required to access Prometheus metrics (instead 404 Forbidden is returned). It is possible to generate these two files from Kubernetes config file. Certificate is stored in `users/admin/client-cerfificate-data` and key in `users/admin/client-key-data`. Please note that these values are encoded by using Base64 encoding, so it is needed to decode them, for example by `base64 -d`.

There's a tool named `gen_cert_key.py` that can be used to automatically generate both files. It is stored in `tools` subdirectory.

#### Usage:

```
gen_cert_file.py kubeconfig.yaml
```

### Fetching metrics from Prometheus endpoint

```
sudo kubefwd svc -n openshift-monitoring -d openshift-monitoring.svc -l prometheus=k8s
curl --cert k8s.crt --key k8s.key  -k 'https://prometheus-k8s.openshift-monitoring.svc:9091/metrics'
```

### Debugging prometheus metrics without valid CA

Get a bearer token
```
oc sa get-token prometheus-k8s -n openshift-monitoring
```
Change in pkg/controller/operator.go after creating metricsGatherKubeConfig (about line 86)
```
metricsGatherKubeConfig.Insecure = true
metricsGatherKubeConfig.BearerToken = "paste your token here"
metricsGatherKubeConfig.CAFile = "" // by default it is "/var/run/secrets/kubernetes.io/serviceaccount/service-ca.crt"
metricsGatherKubeConfig.CAData = []byte{}
```


### Formatting archive json files
This formats .json files from folder with extracted archive.
```
find . -type f -name '*.json' -print | while read line; do cat "$line" | jq > "$line.tmp" && mv "$line.tmp" "$line"; done
```
