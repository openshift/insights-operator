package clusterconfig

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	batchv1fake "k8s.io/client-go/kubernetes/typed/batch/v1/fake"
)

//nolint: funlen
func Test_SAPPods(t *testing.T) {
	// Initialize the fake dynamic client.
	var datahubYAML = `apiVersion: installers.datahub.sap.com/v1alpha1
kind: DataHub
metadata:
    name: example-datahub
    namespace: example-namespace
`

	datahubsResource := schema.GroupVersionResource{Group: "installers.datahub.sap.com", Version: "v1alpha1", Resource: "datahubs"}
	datahubsClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), map[schema.GroupVersionResource]string{
		datahubsResource: "DataHubsList",
	})

	decUnstructured := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	testDatahub := &unstructured.Unstructured{}

	_, _, err := decUnstructured.Decode([]byte(datahubYAML), nil, testDatahub)
	if err != nil {
		t.Fatal("unable to decode datahub YAML", err)
	}

	// Initialize the remaining K8s/OS fake clients.
	coreClient := kubefake.NewSimpleClientset()
	jobsClient := &batchv1fake.FakeBatchV1{Fake: &coreClient.Fake}

	_, _ = coreClient.CoreV1().Pods("example-namespace").Create(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-pod1",
			Namespace: "example-namespace",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodFailed,
		},
	}, metav1.CreateOptions{})

	records, errs := gatherSAPPods(context.Background(), datahubsClient, coreClient.CoreV1(), jobsClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 0 records because there is no datahubs resource in the namespace.
	if len(records) != 0 {
		t.Fatalf("unexpected number or records in the first run: %d", len(records))
	}

	// Create the DataHubs resource and now the SCCs and CRBs should be gathered.
	_, _ = datahubsClient.
		Resource(datahubsResource).
		Namespace("example-namespace").
		Create(context.Background(), testDatahub, metav1.CreateOptions{})

	records, errs = gatherSAPPods(context.Background(), datahubsClient, coreClient.CoreV1(), jobsClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 1 record because the pod is now in the same namespace as a datahubs resource.
	if len(records) != 1 {
		t.Fatalf("unexpected number or records in the second run: %d", len(records))
	}

	// Create a failed job.
	_, err = jobsClient.Jobs("example-namespace").Create(context.Background(), &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:         "example-job1",
			GenerateName: "example-namespace",
		},
		Status: batchv1.JobStatus{
			Failed:    1,
			Succeeded: 0,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("unable to create job: %#v", err)
	}

	// Add a failed pod to the failed job.
	_, _ = coreClient.CoreV1().Pods("example-namespace").Create(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-pod2",
			Namespace: "example-namespace",
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "Job",
				Name: "example-job1",
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodFailed,
		},
	}, metav1.CreateOptions{})

	records, errs = gatherSAPPods(context.Background(), datahubsClient, coreClient.CoreV1(), jobsClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// 2 records because the second pod belongs to a failed job.
	if len(records) != 2 {
		t.Fatalf("unexpected number or records in the third run: %d", len(records))
	}

	// Create a successful job.
	_, err = jobsClient.Jobs("example-namespace").Create(context.Background(), &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:         "example-job2",
			GenerateName: "example-namespace",
		},
		Status: batchv1.JobStatus{
			Failed:    1,
			Succeeded: 1,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("unable to create job: %#v", err)
	}

	// Add a failed pod to the successful job.
	_, _ = coreClient.CoreV1().Pods("example-namespace").Create(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-pod3",
			Namespace: "example-namespace",
			OwnerReferences: []metav1.OwnerReference{{
				Kind: "Job",
				Name: "example-job2",
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodFailed,
		},
	}, metav1.CreateOptions{})

	records, errs = gatherSAPPods(context.Background(), datahubsClient, coreClient.CoreV1(), jobsClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// Still 2 records because the third pod belongs to a successful job.
	if len(records) != 2 {
		t.Fatalf("unexpected number or records in the fourth run: %d", len(records))
	}

	// Create a healthy successful pod.
	_, _ = coreClient.CoreV1().Pods("example-namespace").Create(context.Background(), &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example-pod4",
			Namespace: "example-namespace",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodSucceeded,
		},
	}, metav1.CreateOptions{})

	records, errs = gatherSAPPods(context.Background(), datahubsClient, coreClient.CoreV1(), jobsClient)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %#v", errs)
	}
	// Still 2 records because the fourth pod is successful.
	if len(records) != 2 {
		t.Fatalf("unexpected number or records in the fifth run: %d", len(records))
	}
}
