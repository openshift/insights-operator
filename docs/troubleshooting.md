# Potential problems

## lookup prometheus-k8s.openshift-monitoring.svc on * no such host

### Problem:

Metrics aren't gathered and the following line appears in the docs:

```
Unable to retrieve most recent metrics: 
Get "https://prometheus-k8s.openshift-monitoring.svc:9091/federate?match%5B%5D=etcd_object_counts&match%5B%5D=cluster_installer&match%5B%5D=namespace%3Acontainer_cpu_usage_seconds_total%3Asum_rate&match%5B%5D=namespace%3Acontainer_memory_usage_bytes%3Asum": 
dial tcp: lookup prometheus-k8s.openshift-monitoring.svc on 10.45.248.15:53: no such host
```

### Solution:

It's required to have this endpoint available on your host in some way. 

1. Forward ports, for example by kubefwd:

`kubefwd svc -n openshift-monitoring -d openshift-monitoring.svc -l prometheus=k8s --kubeconfig $KUBECONFIG`

where `$KUBECONFIG` holds the location of kubeconfig for your cluster

2. Login to your cluster in `oc` and get the token:

`oc sa get-token prometheus-k8s -n openshift-monitoring`

3. Add the following lines to `pkg/controller/operator.go` after the current configuration(around line 91):

```golang
metricsGatherKubeConfig.Insecure = true
metricsGatherKubeConfig.BearerToken = "token from the previous step"
metricsGatherKubeConfig.CAFile = ""
metricsGatherKubeConfig.CAData = []byte{}
```
