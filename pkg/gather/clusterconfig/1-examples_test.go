package clusterconfig

import (
	"fmt"
)

func ExampleGatherMostRecentMetrics_Test() {
	b, err := ExampleMostRecentMetrics()
	if err != nil {
		fmt.Print(err)
	}
	fmt.Print(string(b))
	// Output:
	// [{"Name":"config/metrics","Captured":"0001-01-01T00:00:00Z","Fingerprint":"","Item":"SGVsbG8sIGNsaWVudAojIEFMRVJUUyAyLzEwMDAKSGVsbG8sIGNsaWVudAo="}]
}

func ExampleGatherClusterOperators_Test() {
	b, err := ExampleClusterOperators()
	if err != nil {
		fmt.Print(err)
	}
	fmt.Print(string(b))
	// Output:
	// [{"Name":"config/clusteroperator/","Captured":"0001-01-01T00:00:00Z","Fingerprint":"","Item":{"metadata":{"creationTimestamp":null},"spec":{},"status":{"conditions":[{"type":"Degraded","status":"","lastTransitionTime":null}],"extension":null}}}]
}

func ExampleGatherNodes_Test() {
	b, err := ExampleNodes()
	if err != nil {
		fmt.Print(err)
	}
	fmt.Print(string(b))
	// Output:
	// [{"Name":"config/node/","Captured":"0001-01-01T00:00:00Z","Fingerprint":"","Item":{"metadata":{"creationTimestamp":null},"spec":{},"status":{"conditions":[{"type":"Ready","status":"False","lastHeartbeatTime":null,"lastTransitionTime":null}],"daemonEndpoints":{"kubeletEndpoint":{"Port":0}},"nodeInfo":{"machineID":"","systemUUID":"","bootID":"","kernelVersion":"","osImage":"","containerRuntimeVersion":"","kubeletVersion":"","kubeProxyVersion":"","operatingSystem":"","architecture":""}}}}]
}
