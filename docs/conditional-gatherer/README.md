# Conditional Gatherer

Conditional gatherer is a special gatherer which uses a set of rules describing which gathering functions to activate.
The conditional rules are defined in the [insights-operator-gathering-conditions GitHub repository](https://github.com/RedHatInsights/insights-operator-gathering-conditions). This content is consumed and exposed by the [insights-operator-gathering-conditions-service](https://github.com/RedHatInsights/insights-operator-gathering-conditions-service). A new version of the `insights-operator-gathering-conditions` requires a new release version of the `insights-operator-gathering-conditions-service`.
The Insights Operator connects to this service and consumes the conditional rules from it. The connection endpoint is defined in the [pod.yaml config file](../../config/pod.yaml) in the `conditionalGathererEndpoint` attribute (the value can be overriden in the `support` secret). Authentication is required and the `pull-secret` token is used for this purpose.

## Validation of the conditional rules

The Insights Operator internally validates the conditional rules JSON against the JSON schema. [The schema](../../pkg/gatherers/conditional/gathering_rules.schema.json) is available in the `pkg/gatherers/conditional` package. You can see that this schema refers to the second [gathering_rule.schema.json](../../pkg/gatherers/conditional/gathering_rule.schema.json). This second schema defines the more important restrictions on the specific rules. **If the validation fails, no conditional data will be gathered!**!

The following are some examples of validation failures (which will show up in the log):

Non-existing gathering fumction:
```
E0808 16:29:51.864716  241084 parsing.go:22] skipping a rule because of an error: unable to create params for conditional.GatheringFunctionName: containers_log {[{alert_is_firing 0xc0004639c0 <nil>}] map[]}
```

Missing alert name:
```
E0808 16:38:09.453211  242327 periodic.go:137] conditional failed after 14ms with: got invalid config for conditional gatherer: 0.conditions.0: Must validate at least one schema (anyOf), 0.conditions.0: alert is required
```

Gathering function missing required parameter:
```
E0808 16:41:35.184585  242636 periodic.go:137] conditional failed after 20ms with: got invalid config for conditional gatherer: 0.gathering_functions.containers_logs.tail_lines: Must be greater than or equal to 1
```

Failed to parse the provided cluster version:

```
E0809 10:02:16.383643   37430 conditional_gatherer.go:140] error checking conditions for a gathering rule: Could not parse Range "4-11.12": Could not parse version "4-11.12" in "4-11.12": No Major.Minor.Patch elements found
```

One of the common conditions type (see below) is the `alert_is_firing`. This condition depends on availability of Prometheus alerts - i.e. connection to the in-cluster Prometheus instance. If the connection is not available, this may manifest in the log as follows for example:

```
E0809 11:56:48.491346   46838 conditional_gatherer.go:226] unable to update alerts cache: open /var/run/configmaps/service-ca-bundle/service-ca.crt: no such file or directory
```

If the error message `there are no conditional rules` is shown, that means that there are no conditional rules or that the format used to assign the conditional rules had an invalid json format. Log message will look something like:

```
E0915 09:01:03.349317   26966 periodic.go:158] conditional failed after 2ms with: got invalid config for conditional gatherer: there are no conditional rules
```

## Basic structure of the conditional rules

From the schemas mentioned above, you can see that each rule consists of `conditions` array and `gathering_functions` object. The `conditions` array defines conditions that must be met and the `gathering_functions` object tells what functions are called in the Insights Operator source code. The current conditions are defined in the [`pkg/gatherers/conditional/conditions.go`](../../pkg/gatherers/conditional/conditions.go) (see the `ConditionType` and its use) and the gathering functions are defined in the [`pkg/gatherers/conditional/gathering_functions.go`](../../pkg/gatherers/conditional/gathering_functions.go)


## Manual Testing

To test that conditional gatherer provides some data, follow the next steps:

1. Downscale CVO, CMO and Prometheus CRD:
```bash
oc scale deployment -n openshift-cluster-version cluster-version-operator --replicas=0
oc scale deployment -n openshift-monitoring cluster-monitoring-operator --replicas=0
oc patch prometheus -n openshift-monitoring k8s --type "json" -p '[{"op": "replace", "path": "/spec/replicas", "value": 1}]'
```

2. Backup prometheus rules:
```bash
oc get prometheusrule -n openshift-cluster-samples-operator samples-operator-alerts -o json > prometheus-rules.back.json
```

3. Make SamplesImagestreamImportFailing alert to fire by setting `SamplesImagestreamImportFailing`'s
`expr` value to `1 > bool 0` and `for` to `1s`:
```bash
echo '{
    "apiVersion": "monitoring.coreos.com/v1",
    "kind": "PrometheusRule",
    "metadata": {
        "name": "samples-operator-alerts",
        "namespace": "openshift-cluster-samples-operator"
    },
    "spec": {
        "groups": [
            {
                "name": "SamplesOperator",
                "rules": [
                    {
                        "alert": "SamplesImagestreamImportFailing",
                        "annotations": {
                            "message": "Always firing"
                        },
                        "expr": "1 > bool 0",
                        "for": "1s",
                        "labels": {
                            "severity": "warning"
                        }
                    }
                ]
            }
        ]
    }
}' | oc apply -f -
```

4. Wait for the alert to fire:
```bash
export PROMETHEUS_HOST=$(oc get route -n openshift-monitoring prometheus-k8s -o jsonpath='{@.spec.host}')
export INSECURE_PROMETHEUS_TOKEN=$(oc get secret $(oc get sa prometheus-k8s -n openshift-monitoring -o json | jq .secrets[0].name | tr --delete \") -n openshift-monitoring -o json | jq .metadata.annotations.\"openshift.io/token-secret.value\" | tr --delete \")
curl -g -s -k -H 'Cache-Control: no-cache' -H "Authorization: Bearer $INSECURE_PROMETHEUS_TOKEN" "https://$PROMETHEUS_HOST/api/v1/query" --data-urlencode 'query=ALERTS{alertstate="firing",alertname="SamplesImagestreamImportFailing"}' | jq ".data.result[]"
```

The output should be:
```json
{
  "metric": {
    "__name__": "ALERTS",
    "alertname": "SamplesImagestreamImportFailing",
    "alertstate": "firing",
    "severity": "critical"
  },
  "value": [
    1652363876.855,
    "1"
  ]
}
```

5. Make metrics work by forwarding the endpoint and setting INSECURE_PROMETHEUS_TOKEN environment variable:
```bash
export INSECURE_PROMETHEUS_TOKEN=$(oc get secret $(oc get sa prometheus-k8s -n openshift-monitoring -o json | jq .secrets[0].name | tr --delete \") -n openshift-monitoring -o json | jq .metadata.annotations.\"openshift.io/token-secret.value\")
```
```bash
# run this command in a separate terminal
sudo kubefwd svc -n openshift-monitoring -d openshift-monitoring.svc -l app.kubernetes.io/instance=k8s --kubeconfig $KUBECONFIG
```

6. Run the operator and wait for an archive containing `conditional/` directory.

7. Restore the backup:
```bash
oc apply -f prometheus-rules.back.json
```

8. Fix CVO back
```bash
oc scale deployment -n openshift-cluster-version cluster-version-operator --replicas=1
```

## Using Locally Started Service

1. Run the service following the instructions here
   https://github.com/RedHatInsights/insights-operator-gathering-conditions-service
2. Set `conditionalGathererEndpoint` in `config/local.yaml` to `http://localhost:8081/api/gathering/gathering_rules`
3. Enjoy your conditional rules from the local service

## Using a Mock Server

1. Start a mock server:
```bash
git clone https://github.com/RedHatInsights/insights-operator-gathering-conditions.git
cd insights-operator-gathering-conditions/
./build.sh
python3 -m http.server --directory build/
```

2. Set `conditionalGathererEndpoint` in `config/local.yaml` to `http://localhost:8000/rules.json`
3. Enjoy your conditional rules from the mock service

## Using Stage Endpoint

0. Be connected to Red Hat network or configure a proxy for stage version of console.redhat.com
1. Set up the stage endpoint in `config/local.yaml`
2. Configure authentication through support secret
```bash
echo '{
  "apiVersion": "v1",
  "kind": "Secret",
  "metadata": {
    "namespace": "openshift-config",
    "name": "support"
  },
  "type": "Opaque",
  "data": {
    "username": "'(echo $STAGE_USERNAME | base64 --wrap=0)'",
    "password": "'(echo $STAGE_PASSWORD | base64 --wrap=0)'"
  }
}' | oc apply -f -
```

3. Enjoy your conditional rules from the stage endpoint
