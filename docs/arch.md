# Overview of Insights operator architecture and features

The main goal of the Insights Operator is to periodically gather anonymized data from the OCP cluster (mostly Kubernetes/OpenShift APIs and control plane components) and upload it to `console.redhat.com` for Insights analysis.

Insights Operator does not manage any application. As usual with operator applications, most of the code is structured in the `pkg` package and `pkg/controller/operator.go` hosts the operator controller. Typically operator controllers read configuration and start some periodical tasks.

## How the Insights operator reads configuration
The Insights Operator's configuration is a combination of the file [config/pod.yaml](../config/pod.yaml)(basically default configuration hardcoded in the image) and configuration stored in the `support` secret in the `openshift-config` namespace. The secret doesn't exist by default, but when it does, it overrides default settings which IO reads from the `config/pod.yaml`.
The `support` secret provides following configuration attributes:
- `endpoint` - upload endpoint - default is `https://console.redhat.com/api/ingress/v1/upload`,
- `interval` - data gathering & uploading frequency - default is `2h`
- `httpProxy`, `httpsProxy`, `noProxy` eventually to set custom proxy, which overrides cluster proxy just for the Insights Operator
- `username`, `password` - if set, the insights client upload will be authenticated by basic authorization using the username/password. By default, it uses the token (see below) from the `pull-secret` secret.
- `enableGlobalObfuscation` - to enable the global obfuscation of the IP addresses and the cluster domain name. Default value is `false`
- `reportEndpoint` - download endpoint. From this endpoint, the Insights operator downloads the latest Insights analysis. Default value is `https://console.redhat.com/api/insights-results-aggregator/v2/cluster/%s/reports` (where `%s` must be replaced with the cluster ID)
- `reportPullingDelay` - the delay between data upload and download. Default value is `60s`
- `reportPullingTimeout` - timeout for the Insights download request.
- `reportMinRetryTime` - the time after which the request is retried. Default value is `30s`
- `scaEndpoint` - the endpoing for downloading the Simple Content Access(SCA) entitlements. Default value is `https://api.openshift.com/api/accounts_mgmt/v1/certificates`
- `scaInterval` - frequency of the SCA entitlements download. Default value is `8h`.
- `scaPullDisabled` - flag to disable the SCA entitlements download. Default value is `false`.
- `clusterTransferEndpoint` - the endpoint for checking & download cluster transfer data (updated `pull-secret` data). Default values is `https://api.openshift.com/api/accounts_mgmt/v1/cluster_transfers/`
- `clusterTransferInterval`  - frequency of checking available cluster transfers. Default value is `24h`.
- `conditionalGathererEndpoint` - the endpoing providing conditional gathering rules definitions. Default value is `https://console.redhat.com/api/gathering/gathering_rules`.

Content example of the `support` secret:

```shell script
oc get secret support -n openshift-config -o=yaml
```

```yaml
apiVersion: v1
data:
  endpoint: aHR0cHM6Ly9jbG91ZC5yZWRoYXQuY29tL2FwaS9pbmdyZXNzL3YxL3VwbG9hZA==
  interval: Mmg=
kind: Secret
metadata:
  creationTimestamp: "2020-10-05T05:37:34Z"
  name: support
  namespace: openshift-config
  resourceVersion: "823414"
  selfLink: /api/v1/namespaces/openshift-config/secrets/support
  uid: 0e522987-4c02-479d-8d10-e4f551e60b65
type: Opaque
```

```shell script
oc get secret support -n openshift-config -o=json | jq -r .data.endpoint | base64 -d
```
```
https://console.redhat.com/api/ingress/v1/upload
```

```shell script
oc get secret support -n openshift-config -o=json | jq -r .data.interval | base64 -d
```
```
2h
```

To configure authentication to `console.redhat.com` Insights Operator reads preconfigured token from the `pull-secret` secret (where are the cluster-wide tokens stored) in the `openshift-config` namespace. The token to `console.redhat.com` is stored in `.dockerjsonconfig` under `cloud.openshift.com` key (for historical reasons) in its `auth` attribute.

```shell script
oc get secret/pull-secret -n openshift-config -o json | jq -r ".data | .[]" | base64 --decode | jq
```

```json
{
  "auths": {
    ...
    "cloud.openshift.com": {
      "email": "cee-ops-admins@redhat.com",
      "auth": "BASE64-ENCODED-JWT-TOKEN-REMOVED"
    },
    ...
  }
}
```

The configuration secrets are periodically refreshed by the [configobserver](../pkg/config/configobserver/configobserver.go). Any code can register to receive signal through channel by using `config.ConfigChanged()`, like for example in the `insightsuploader.go`. It will then get notified when config changes.

```go
configCh, cancelFn := c.configurator.ConfigChanged()
```

Internally the configObserver has an array of subscribers, so all of them will get the signal.

## How the Insights operator schedules tasks
A commonly used pattern in the Insights Operator is that the task is run as a go routine and performs its own cycle of periodic actions.
These actions are mostly started from the `operator.go`. They are usually using `wait.Until` - runs function periodically after short delay until end is signalled.
There are these main tasks scheduled:
- Gatherer
- Uploader
- Downloader (Report gatherer)
- Config Observer
- Disk Pruner
- SCA controller
- Cluster transfer controller

## Gathering the data

Insights operator defines three types of gatherers (see below). Each of them must implement the [Interface](../pkg/gatherers/interface.go#L11) and they are initialized by calling `gather.CreateAllGatherers` in `operator.go`. The actual gathering is triggered in `Run` method in `pkg/controller/periodic/periodic.go`, but not every gatherer is triggered every time ( for example, see the [CustomPeriodGatherer type](../pkg/gatherers/interface.go#L21)).

Each gatherer includes one or more gathering functions. Gathering functions are defined as a map, where the key is the name of the function and the value is the [GatheringClosure type](../pkg/gatherers/interface.go#L34). They are executed concurrently in the `HandleTasksConcurrently` function in `pkg/gather/task_processing.go`.
One of the attributes of the `GatheringClosure` type is the function that returns the values: `([]record.Record, []error)`. The slice of the records is the result of gathering function. The actual data is in the `Item` attribute of the `Record`. This `Item` is of type `Marshalable` (see the interface in the [record.go](../pkg/record/record.go)) and there are two JSON marshallers used to serialize the data - `JSONMarshaller` and `ResourceMarshaller` which allows you to save few bytes by omitting the `managedFields` during the serialization.
Errors, warnings or panics that occurred during  given gathering  function are logged in the "metadata" part of the Insights operator archive. See [sample archive example](../docs/insights-archive-sample/insigths-operator/gathers.json)

### Clusterconfig gatherer

Defined in [clusterconfig_gatherer.go](../pkg/gatherers/clusterconfig/clusterconfig_gatherer.go). This gatherer is ran regularly (2h by default) and gathers various data related to cluster config (see [gathered-data doc](../docs/gathered-data.md) for more details).

The data from this gatherer is stored under `/config` directory in the archive.

### Workloads gatherer

Defined in [workloads_gatherer.go](../pkg/gatherers/workloads/workloads_gatherer.go). This gatherer only runs every 12 hours and the interval is not configurable. This is done because running the gatherer more often would significantly increase data in the archive, that is assumed will not change very often. There is only one gathering function in this gatherer and it gathers workload fingerprint data (SHA of the images, fingerprints of namespaces as number of pods in namespace, fingerprints of containers as first command and first argument).

The data from this gatherer is stored in the `/config/workload_info.json` file in the archive, but please note that not every archive contains this data.

### Conditional gatherer

Defined in [conditional_gatherer.go](../pkg/gatherers/conditional/conditional_gatherer.go). This gatherer is ran regularly (2h by default), but it only gathers some data when a corresponding condition is met. The conditions and corresponding gathering functions are defined in an external service (https://console.redhat.com/api/gathering/gathering_rules). A typical example of a condition is when an alert is firing. This also means that this gatherer relies on the availability of Prometheus metrics and alerts.

The data from this gatherer is stored under the `/conditional` directory in the archive.

## Downloading and exposing Insights Analysis
After every successful upload of archive, the operator waits (see the `reportPullingDelay` config attribute) and
then it tries to download the latest Insights analysis result of the latest archive (created by the Insights pipeline
in `console.redhat.com`). The report is verified by checking the `LastCheckedAt` timestamp (see
`pkg/insights/insightsreport/types.go`). If the latest Insights result is not yet available (e.g. the pipeline may be
delayed) or there has been some error response, the download request is repeated (see the `reportMinRetryTime` config
attribute). The successfully downloaded Insights report is parsed and the numbers of corresponding hitting Insights
recommendations are exposed via `health_statuses_insights` Prometheus metric.

Code: Example of reported metrics:
```prometheus
# HELP health_statuses_insights [ALPHA] Information about the cluster health status as detected by Insights tooling.
# TYPE health_statuses_insights gauge
health_statuses_insights{metric="critical"} 0
health_statuses_insights{metric="important"} 0
health_statuses_insights{metric="low"} 1
health_statuses_insights{metric="moderate"} 1
health_statuses_insights{metric="total"} 2
```

### Metrics

- `health_statuses_insights`, information about the cluster health status based on the last downloaded report, corresponding to its number of hitting recommendations grouped by severity.
- `insightsclient_request_send_total`, tracks the number of archives sent.
- `insightsclient_request_recvreport_total`, tracks the number of Insights reports received/downloaded.
- `insightsclient_last_gather_time`, the time of the last Insights data gathering.
- `insights_recommendation_active`, expose Insights recommendations as Prometheus alerts.

> **Note**
> The metrics are registered by [the `MustRegisterMetrics` function](../pkg/insights/metrics.go)

### Alerts

- `InsightsDisabled`, Insights operator is disabled.
- `SimpleContentAccessNotAvailable`, simple content access certificates are not available.
- `InsightsRecommendationActive`, an Insights recommendation is active for this cluster.

> **Note**
> The alerts are defined [here](../manifests/08-prometheus_rule.yaml)

### Scheduling and running of Uploader
The `operator.go` starts background task defined in `pkg/insights/insightsuploader/insightsuploader.go`. The insights uploader periodically checks if there is any data to upload. If no data is found, the uploader continues with next cycle.
The uploader triggers the `wait.Until` function, which waits until the configuration changes or it is time to upload. After start of the operator, there is some waiting time before the very first upload. This time is defined by `initialDelay`. If no error occurred while sending the POST request, then the next uploader check is defined as `wait.Jitter(interval, 1.2)`, where interval is the gathering interval.

## How Uploader authenticates to console.redhat.com
The HTTP communication with the external service (e.g uploading the Insights archive or downloading the Insights analysis) is defined in the [insightsclient package](../pkg/insights/insightsclient/). The HTTP transport is encrypted with TLS (see the `clientTransport()` function defined in the `pkg/insights/insightsclient/insightsclient.go`. This function (and the `prepareRequest` function) uses `pkg/authorizer/clusterauthorizer.go` to respect the proxy settings and to authorize (i.e add the authorization header with respective token value) the requests. The user defined certificates in the `/var/run/configmaps/trusted-ca-bundle/ca-bundle.crt` are taken into account (see the cluster wide proxy setting in the [OCP documentation](https://docs.openshift.com/container-platform/latest/networking/enable-cluster-wide-proxy.html)).

## Summarising the content before upload
Summarizer is defined by `pkg/recorder/diskrecorder/diskrecorder.go` and is merging all existing archives. That is, it merges together all archives with name matching pattern `insights-*.tar.gz`, which weren't removed and which are newer than the last check time. Then mergeReader is taking one file after another and adding all of them to archive under their path.
If the file names are unstable (for example reading from Api with Limit and reaching the Limit), it could merge together more files than specified in Api limit.

## Scheduling the ConfigObserver
Another background task is from `pkg/config/configobserver/configobserver.go`. The observer creates `configObserver` by calling `configObserver.New`, which sets default observing interval to 5 minutes.
The `Start` method runs again `wait.Until` every 5 minutes and reads both `support` and `pull-secret` secrets.

## Scheduling diskpruner and what it does
By default Insights Operator Gather is calling diskrecorder to save newly collected data in a new file, but doesn't remove old. This is the task of diskpruner. Observer calls `recorder.PeriodicallyPrune()` function. It is again using wait.Until pattern and runs approximately after every second interval.
Internally it calls `diskrecorder.Prune` with `maxAge = interval*6*24` (with 2h it is 12 days) everything older is going to be removed from the archive path (by default `/tmp/insights-operator`).

## How the Insights operator sets operator status
The operator status is based on K8s [Pod conditions](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-conditions).
Code: How Insights Operator status conditions looks like:
```shell script
oc get co insights -o=json | jq '.status.conditions'
```
```json
[
  {
    "lastTransitionTime": "2022-05-16T06:41:35Z",
    "message": "Monitoring the cluster",
    "reason": "AsExpected",
    "status": "False",
    "type": "Progressing"
  },
  {
    "lastTransitionTime": "2022-05-16T06:41:35Z",
    "message": "Insights works as expected",
    "reason": "AsExpected",
    "status": "False",
    "type": "Degraded"
  },
  {
    "lastTransitionTime": "2022-05-18T08:11:58Z",
    "message": "SCA certs successfully updated in the etc-pki-entitlement secret",
    "reason": "Updated",
    "status": "True",
    "type": "SCAAvailable"
  },
  {
    "lastTransitionTime": "2022-05-18T08:11:58Z",
    "message": "no available cluster transfer",
    "reason": "NoClusterTransfer",
    "status": "False",
    "type": "ClusterTransferAvailable"
  },
  {
    "lastTransitionTime": "2022-05-16T06:41:35Z",
    "message": "Insights operator can be upgraded",
    "reason": "InsightsUpgradeable",
    "status": "True",
    "type": "Upgradeable"
  },
  {
    "lastTransitionTime": "2022-05-16T06:41:35Z",
    "reason": "AsExpected",
    "status": "False",
    "type": "Disabled"
  },
  {
    "lastTransitionTime": "2022-05-16T06:41:35Z",
    "message": "Insights works as expected",
    "reason": "AsExpected",
    "status": "True",
    "type": "Available"
  }
]
```
A condition is defined by its type. You may notice that there are some non-standard clusteroperator conditions. They are:
- `SCAAvailable` - based on the SCA (Simple Content Access) controller in `pkg/ocm/sca/sca.go` and provides information about the status of downloading the SCA entitlements.
- `ClusterTransferAvailable` - based on the cluster transfer controller in `pkg/ocm/clustertransfer/cluster_transfer.go` and provides information about the availability of cluster transfers.
- `Disabled` - indicates whether data gathering is disabled or enabled.

In addition to the above clusteroperator conditions, there are some intermediate clusteroperator conditions. These are:
- `UploadDegraded` - this condition occurs when there is any unsuccessful upload of the Insights data (if the number of the upload attemp is equal or greater than 5 then the operator is marked as **Degraded**). Example is:
  ```json
    {
      "lastTransitionTime": "2022-05-18T10:12:23Z",
      "message": "Unable to report: gateway server reported unexpected error code: 404 (request=d358f839bc1c451389f3911ec8427f5e): 404 page not found\n",
      "reason": "UploadFailed",
      "status": "True",
      "type": "UploadDegraded"
     },

  ```
- `InsightsDownloadDegraded` - this condition occurs when there is any unsuccessful download of the Insights analysis. Example is:
  ```json
    {
      "lastTransitionTime": "2022-05-18T10:17:49Z",
      "message": "Couldn't download the latest report: not found: https://console.redhat.com/api/insights-results-aggregator/v2/cluster/ab0aab3b-d0c5-43bb-9b83-7de8185c8d98/reportsa (request=2e278890ef7d46c8b42ac735f1ba4476): 404 page not found\n",
      "reason": "NotAvailable",
      "status": "True",
      "type": "InsightsDownloadDegraded"
    },
  ```


The status is updated by `pkg/controller/status/status.go`. Status has a background task, which periodically updates
the operator status from its internal list of sources. Any component which wants to participate in operator's status adds a
SimpleReporter, which returns its actual status. The Simple reporter is defined in `controllerstatus`.

Code: In `operator.go` components are adding their reporters to Status Sources:
```go
statusReporter.AddSources(uploader)
```

This periodic status updater calls `updateStatus `which sets the operator status after calling merge to all the provided sources.
The uploader `updateStatus` determines if it is safe to upload, if cluster operator status is healthy. It relies on fact that `updateStatus` is called on start of status cycle.

## How is Insights Operator using various API Clients
Internally Insights operator talks to Kubernetes API server over HTTP REST queries. Each query is authenticated by a Bearer token,
to simulate see an actual Rest query being used, you can try:

```shell script
oc get pods -A -v=9
```
```
I1006 12:26:33.972634   66541 loader.go:375] Config loaded from file:  /home/mkunc/.kube/config
I1006 12:26:33.977546   66541 round_trippers.go:423] curl -k -v -XGET  -H "Accept: application/json;as=Table;v=v1;g=meta.k8s.io,application/json;as=Table;v=v1beta1;g=meta.k8s.io,application/json" -H "User-Agent: oc/4.5.0 (linux/amd64) kubernetes/9933eb9" -H "Authorization: Bearer Xy9HoVzNdsRifGr3oCIl7pfxwkeqE2u058avw6o969w" 'https://api.sharedocp4upi43.lab.upshift.rdu2.redhat.com:6443/api/v1/pods?limit=500'
I1006 12:26:36.075230   66541 round_trippers.go:443] GET https://api.sharedocp4upi43.lab.upshift.rdu2.redhat.com:6443/api/v1/pods?limit=500 200 OK in 2097 milliseconds
I1006 12:26:36.075284   66541 round_trippers.go:449] Response Headers:
I1006 12:26:36.075300   66541 round_trippers.go:452]     Audit-Id: 53ad17b9-c3fe-4166-9693-2bacf60f7dcc
I1006 12:26:36.075313   66541 round_trippers.go:452]     Cache-Control: no-cache, private
I1006 12:26:36.075326   66541 round_trippers.go:452]     Content-Type: application/json
I1006 12:26:36.075347   66541 round_trippers.go:452]     Vary: Accept-Encoding
I1006 12:26:36.075370   66541 round_trippers.go:452]     Date: Tue, 06 Oct 2020 10:26:36 GMT
I1006 12:26:36.467245   66541 request.go:1068] Response Body: {"kind":"Table","apiVersion":"meta.k8s.io/v1","metadata":{"selfLink":"/api/v1/pods"
... CUT HERE
```

But adding Bearer token and creating Rest query is all handled automatically for us by using Clients, which are generated, type safe golang libraries,
like [github.com/openshift/client-go](github.com/openshift/client-go) or [github.com/kubernetes/client-go](github.com/kubernetes/client-go).
Both these libraries are generated by automation, which specifies from which Api repo and which Api Group it generates it.

All clients are created near/at where they are going to be used, we pass around the configs that were created from the KUBECONFIG envvar defined in cluster.
Reason for doing this is that there are many clients every one of which is cheap to create and passing around the config is simple while also not changing much over time.
On the other hand its quite cumbersome to pass around a bunch of clients, the number of which is changing by the day, with no benefit.

## How are the credentials used in clients
In IO deployment [manifest](manifests/06-deployment.yaml) is specified service account operator (serviceAccountName: operator). This is the account under which insights operator runs or reads its configuration or also reads the metrics.
Because Insights Operator needs quite powerful credentials to access cluster-wide resources, it has one more service account called gather. It is created
in [manifest](manifests/03-clusterrole.yaml).

Code: To verify if gather account has right permissions to call verb list from apigroup machinesets I can use:

```shell script
kubectl auth can-i list machinesets --as=system:serviceaccount:openshift-insights:gather
```
```
yes
```

This account is used to impersonate any clients which are being used in Gather Api calls. The impersonated account is set in operator go:
Code: In Operator.go specific Api client is using impersonated account
```go
	gatherKubeConfig := rest.CopyConfig(controller.KubeConfig)
	if len(s.Impersonate) > 0 {
		gatherKubeConfig.Impersonate.UserName = s.Impersonate
	}
  // .. and later on this impersonated client is used to create another clients
  gatherConfigClient, err := configv1client.NewForConfig(gatherKubeConfig)
```

Code: The impersonated account is specified in config/pod.yaml (or config/local.yaml) using:
```yaml
impersonate: system:serviceaccount:openshift-insights:gather
```

To test where the client has right permissions, the command mentioned above with verb, api and service account can be used.

Note: I was only able to test missing permissions on OCP 4.3, the versions above seems like always passing fine. Maybe higher versions
don't have RBAC enabled.

Code: Example error returned from Api, in this case downloading Get config from imageregistry.
```
configs.imageregistry.operator.openshift.io "cluster" is forbidden: User "system:serviceaccount:openshift-insights:gather" cannot get resource "configs" in API group "imageregistry.operator.openshift.io" at the cluster scope
```

## How API extensions works
If any cloud native application wants to add some Kubernetes Api endpoint, it needs to define it using [K8s Api extensions](https://kubernetes.io/docs/concepts/extend-kubernetes/) and it would need to define Custom Resource Definition. Openshift itself defines them for [github.com/openshift/api](github.com/openshift/api) (ClusterOperators, Proxy, Image, ..). Thus for using api of Openshift, we need to use Openshift's client-go generated client.
If we would need to use Api of some other Operators, we would need to find if Operator is defining Api.

Typically when operator defines a new CRD type, this type would be defined inside of its repo (for example [Machine Config Operator's MachineConfig](https://github.com/openshift/machine-config-operator/tree/master/pkg/apis/machineconfiguration.openshift.io)).

To talk to specific Api, we need to have generated clientset and generated lister types from the CRD type. There might be three possibilities:
- Operator doesn't generate clientset nor lister types
- Operator generate only lister types
- Operator generates both, clientset and lister types

Machine Config Operator defines:
- its Lister types [here](https://github.com/openshift/machine-config-operator/tree/master/pkg/generated/listers/machineconfiguration.openshift.io/v1)
- its ClientSet [here](https://github.com/openshift/machine-config-operator/blob/master/pkg/generated/clientset/versioned/clientset.go)

Normally such a generation is not intended for other consumers, unless it is prepared in a separate api library. For example
[Operators Lifecycle Manager](https://github.com/operator-framework/operator-lifecycle-manager) defines its CRD types [here](https://github.com/operator-framework/api/tree/master/pkg/operators/v1alpha1). Operators framework is exposing in Api only CRD and lister types, not ClientSet.

One problem with adding new operator to go.mod is that usually other operator will have its own reference to k8s/api (and related k8s/library-go), which might be different then what Insights Operator is using, which could cause issues during compilation (when referenced Operator is using Api from new k8s api).

If it is impossible to reference, or operator doesn't expose generated Lister or ClientSet types in all these cases when we don't have type safe
Api, we can still use non type safe custom build types called [dynamic client](k8s.io/client-go/dynamic). There are two cases, when Lister types exists, but no ClientSet, or when no Lister types exists at all both have examples [here](https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.2/pkg/client#example-Client-List).
Such a client is used in [GatherMachineSet](pkg/gather/clusterconfig/clusterconfig.go).

## Configuring what to gather
In the yaml config there is a field named `gather` it expects a list of strings, each string is an id that is connected to a gather function. Adding such an id to the list means that that certain gather function needs to be run.
If nothing is set in the `gather` list then no gathering will take place and an error will be raised.
There is a special id named `ALL` which if in the list then every gather function will be run.
The id of each gather function can be found in the `docs/gathered-data.md` beside the `Id in config:` text for each section.

#### Example for using special id `ALL`
```yaml
gather:
  - ALL
```

#### Example for using individual ids
```yaml
gather:
 - pdbs
 - metrics
 - operators
 - container_images
 - nodes
 - config_maps
 - version
 - id
 - infrastructures
 - networks
 - authentication
 - image_registries
 - image_pruners
 - feature_gates
 - oauths
 - ingress
 - proxies
 - certificate_signing_requests
 - crds
 - host_subnets
 - machine_sets
 - install_plans
 - service_accounts
 - machine_config_pools
 - container_runtime_configs
 - stateful_sets
```
