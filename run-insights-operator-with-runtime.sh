if [[ -z "${CS_NAMESPACE}"  ]]; then
  CS_NAMESPACE="openshift-insights"
fi

operator_pod=$(kubectl get pods -n openshift-insights   --selector=app=insights-operator-with-runtime --no-headers  -o custom-columns=":metadata.name")

echo $operator_pod

kubectl exec --namespace $CS_NAMESPACE $operator_pod -- /usr/bin/insights-operator gather --config /etc/insights-operator/server.yaml

kubectl cp -n $CS_NAMESPACE $operator_pod:/var/lib/insights-operator/ /tmp/insights-operator