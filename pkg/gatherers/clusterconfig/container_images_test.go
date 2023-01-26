package clusterconfig

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/openshift/insights-operator/pkg/record"
)

func Test_ContainerImages_Gather(t *testing.T) { //nolint: funlen,gocyclo
	const fakeNamespace = "fake-namespace"
	const fakeOpenshiftNamespace = "openshift-fake-namespace"

	mockContainers := []string{
		"registry.redhat.io/1",
		"registry.redhat.io/2",
		"registry.redhat.io/3",
	}

	// It is not possible to predict the order of the images.
	expectedPodsWithAge := PodsWithAge{
		"0001-01": RunningImages{
			0: 1,
			1: 1,
			2: 1,
		},
	}

	coreClient := kubefake.NewSimpleClientset()
	for index, containerImage := range mockContainers {
		_, err := coreClient.CoreV1().
			Pods(fakeNamespace).
			Create(context.Background(), &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: fakeNamespace,
					Name:      fmt.Sprintf("pod%d", index),
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							Name:  fmt.Sprintf("container%d", index),
							Image: containerImage,
						},
					},
					Phase: corev1.PodRunning,
				},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal("unable to create fake pod")
		}
	}

	const numberOfCrashlooping = 10
	expectedRecords := make([]string, numberOfCrashlooping)
	for i := 0; i < numberOfCrashlooping; i++ {
		podName := fmt.Sprintf("crashlooping%d", i)
		_, err := coreClient.CoreV1().
			Pods(fakeOpenshiftNamespace).
			Create(context.Background(), &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: podName,
				},
				Status: corev1.PodStatus{
					ContainerStatuses: []corev1.ContainerStatus{
						{
							RestartCount: int32(numberOfCrashlooping - i),
							LastTerminationState: corev1.ContainerState{
								Terminated: &corev1.ContainerStateTerminated{
									ExitCode: int32(i + 1),
								},
							},
						},
					},
				},
			}, metav1.CreateOptions{})
		if err != nil {
			t.Fatal("unable to create fake pod")
		}
		expectedRecords[i] = fmt.Sprintf("config/pod/%s/%s", fakeOpenshiftNamespace, podName)
	}

	ctx := context.Background()
	records, errs := gatherContainerImages(ctx, coreClient.CoreV1())
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}

	var containerInfo *ContainerInfo
	for _, rec := range records {
		if rec.Name != "config/running_containers" {
			continue
		}
		anonymizer, ok := rec.Item.(record.JSONMarshaller)
		if !ok {
			t.Fatal("reported running containers item has invalid type")
		}

		containers, ok := anonymizer.Object.(ContainerInfo)
		if !ok {
			t.Fatal("anonymized running containers data have wrong type")
		}

		containerInfo = &containers
	}

	if containerInfo == nil { // nolint: staticcheck
		t.Fatal("container info has not been reported")
	}

	if len(containerInfo.Images) != len(mockContainers) { // nolint: staticcheck
		t.Fatalf("expected %d unique images, got %d", len(mockContainers), len(containerInfo.Images))
	}

	if !reflect.DeepEqual(containerInfo.Containers, expectedPodsWithAge) {
		t.Fatalf("unexpected map of image counts: %#v", containerInfo.Containers)
	}

	for _, expectedRecordName := range expectedRecords {
		wasReported := false
		for _, reportedRecord := range records {
			if reportedRecord.Name == expectedRecordName {
				wasReported = true
				break
			}
		}
		if !wasReported {
			t.Fatalf("expected record '%s' was not reported", expectedRecordName)
		}
	}
}

func Test_ContainerImages_EnvVar_Obfuscation(t *testing.T) {
	// Given
	mock := []corev1.Container{
		{
			Env: []corev1.EnvVar{
				{Name: "NO_TARGET", Value: "original_value"},
				{Name: "HTTP_PROXY", Value: "original_value"},
				{Name: "HTTPS_PROXY", Value: "original_value"},
			},
		},
	}
	envOriginalValue := "original_value"

	// When
	obfuscateContainerEnvVars(mock)

	// Assert
	t.Run("Non target env vars keep their original value", func(t *testing.T) {
		test := mock[0].Env[0]
		if test.Value != envOriginalValue {
			t.Logf("\nexpected the variable '%s' to have 'original_value'\ngot: %s", test.Name, test.Value)
			t.Fail()
		}
	})
	t.Run("HTTP_PROXY is updated with obfuscated value", func(t *testing.T) {
		test := mock[0].Env[1]
		if test.Value == envOriginalValue {
			t.Logf("\nexpected the variable '%s' to have '<obfuscated>' value\ngot: %s", test.Name, test.Value)
			t.Fail()
		}
	})
	t.Run("HTTPS_PROXY is updated with obfuscated value", func(t *testing.T) {
		test := mock[0].Env[2]
		if test.Value == envOriginalValue {
			t.Logf("\nexpected the variable '%s' to have '<obfuscated>' value\ngot: %s", test.Name, test.Value)
			t.Fail()
		}
	})
}
