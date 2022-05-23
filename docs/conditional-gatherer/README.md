# Conditional Gatherer

Conditional gatherer is a special gatherer which uses a set of rules describing which gathering functions to activate.
More details can be found in `pkg/gatherers/conditional/conditional_gatherer.go`.

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
export PROMETHEUS_HOST=(oc get route -n openshift-monitoring prometheus-k8s -o jsonpath='{@.spec.host}')
export INSECURE_PROMETHEUS_TOKEN=(oc sa get-token prometheus-k8s -n openshift-monitoring)
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
export INSECURE_PROMETHEUS_TOKEN=(oc sa get-token prometheus-k8s -n openshift-monitoring)
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
