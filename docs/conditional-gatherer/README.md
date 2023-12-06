# Conditional Gathering

The conditional gathering system is a specialized gatherer that utilizes a set of rules to determine which gathering functions to activate. These rules are defined in the [insights-operator-gathering-conditions GitHub repository](https://github.com/RedHatInsights/insights-operator-gathering-conditions) and are consumed and exposed by the [insights-operator-gathering-conditions-service](https://github.com/RedHatInsights/insights-operator-gathering-conditions-service). Any updates or changes to the conditional rules necessitate a new release version of the `insights-operator-gathering-conditions-service`.

The Insights Operator establishes a connection with this service to consume the conditional rules, and the connection endpoint is specified in the [pod.yaml config file](../../config/pod.yaml) under the `conditionalGathererEndpoint` attribute. It's important to note that the value of this attribute can be overridden in the `support` secret. Authentication is mandatory, and the `pull-secret` token is utilized for this purpose.

If you are looking for extra develoment information, please, [check this document](./development.md).

## Conditions and Gathering Functions

The conditional gathering system allows you to tailor data collection based on specific conditions and gathering functions. Conditions define when data gathering should be triggered, and gathering functions specify how data should be collected.

Here is an example JSON structure for defining conditions:

```json
{
    "conditions": [
        {
            "type": "cluster_version_matches",
            "cluster_version_matches": {
                "version": "> 4.12.0"
            }
        }
    ],
    "gathering_functions": {
        "pod_logs": {
            "resource_filter": {
                "namespaces": ["openshift-monitoring"],
                "pod_name_regex_filter": "thanos-"
            },
            "log_filter": {
                "messages_to_search": ["connect: connection refused"],
                "tail_lines": 100,
                "previous": false
            }
        }
    }
}
```

## Conditions

Conditions determine when data gathering should be triggered within the conditional data gathering system. These conditions are configured based on specific events or states within the OpenShift cluster.

For example, the `cluster_version_matches` condition allows you to trigger data gathering based on the OpenShift cluster version meeting specific criteria. Understand and utilize these conditions to precisely control when data collection activities occur, ensuring they align with your cluster's operational requirements.

- [cluster_version_matches](#cluster-version-matches)
- [alert_is_firing](#alert-is-firing)

### Alert is Firing

The `alert_is_firing` condition enables data gathering based on Prometheus alert status. You can configure the condition as shown below:

```json
{
    "type": "alert_is_firing",
    "alert": {
        "name": "KubePodCrashLooping"
    }
}
```

In this example, data gathering will be initiated when the Prometheus alert named `KubePodCrashLooping` is firing.

### Cluster Version Matches

The `cluster_version_matches` condition allows you to trigger data gathering based on the OpenShift cluster version. You can define the condition as follows:

```json
{
    "type": "cluster_version_matches",
    "cluster_version_matches": {
        "version": "> 4.12.0"
    }
}
```

In this example, data gathering will be triggered if the OpenShift version is greater than `4.12.0`.

## Gathering Functions

Gathering functions are predefined tasks within the conditional gathering system that perform specific data collection operations when triggered by a matching condition. These functions allows  you to customize data collection according to your specific requirements. Leverage different functions for various conditions to tailor the gathering process to meet your unique needs.

### Pod Logs

The `pod_logs` gathering function is a powerful tool to collect logs from specific pods based on various criteria. Here's an example configuration:

```json
{
    "pod_logs": {
        "resource_filter": {
            "namespaces": ["openshift-monitoring"],
            "pod_name_regex_filter": "thanos-"
        },
        "log_filter": {
            "messages_to_search": ["connect: connection refused"],
            "tail_lines": 100,
            "previous": false
        }
    }
}
```

In this example:

- `resource_filter.namespaces`: Specifies the namespaces to search for resources.
- `resource_filter.pod_name_regex_filter`: Uses a regex to filter pod names.
- `log_filter.messages_to_search`: Lists messages to search for in the logs.
- `log_filter.tail_lines`: Specifies the number of lines to retrieve from the end of logs.

Here is a detailed table with all the possible filters for each `pod_logs` parameters:

#### Resource filter
The `resource_filter` structure allows users to filter containers for log gathering. It includes the following fields:

| Field                         | Type   | Description                                                  | Required |
|-------------------------------|--------|--------------------------------------------------------------|----------|
| `namespace`                   | string | The namespace for log gathering.                             | Required |
| `namespaces`                  | array  | An array of namespaces (future replacement for `namespace`). | Optional |
| `label_selector`              | string | Label selector for filtering containers.                     | Optional |
| `field_selector`              | string | Field selector for filtering containers.                     | Optional |
| `container_name_regex_filter` | string | Regex filter for container names.                            | Optional |
| `pod_name_regex_filter`       | string | Regex filter for pod names.                                  | Optional |
| `max_namespace_containers`    | int    | Maximum number of containers to include.                     | Optional |


#### Log filter
The `log_filter` structure enables users to filter log messages. It includes the following fields:

| Field                   | Type   | Description                                           | Required |
|-------------------------|--------|-------------------------------------------------------|----------|
| `messages_to_search`    | array  | Array of messages to search for.                      | Required |
| `is_regex_search`       | bool   | Boolean flag for regex search.                        | Optional |
| `since_seconds`         | int64  | Time duration for log search (in seconds).            | Optional |
| `limit_bytes`           | int64  | Maximum size of log data to collect.                  | Optional |
| `tail_lines`            | int64  | Number of lines to retrieve from the end of logs.     | Optional |
| `previous`              | bool   | Boolean flag for retrieving previous logs.            | Optional |
