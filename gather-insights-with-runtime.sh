if [[ -z "${CS_NAMESPACE}"  ]]; then
  CS_NAMESPACE="openshift-insights"
fi

operator_pod=$(kubectl get pods -n openshift-insights   --selector=app=insights-operator-with-runtime --no-headers  -o custom-columns=":metadata.name")

echo Gathering insights data from $operator_pod

TEMPDIR=/tmp/insights-operator

# trigger the gather operation
kubectl exec --namespace $CS_NAMESPACE $operator_pod -- /usr/bin/insights-operator gather --config /etc/insights-operator/server.yaml
# and copy the gathered date to /tmp/insights-operator
kubectl cp -n $CS_NAMESPACE $operator_pod:/var/lib/insights-operator/ $TEMPDIR

echo Gathered data are stored in $TEMPDIR