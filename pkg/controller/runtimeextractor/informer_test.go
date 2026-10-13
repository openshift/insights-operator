package runtimeextractor

import (
	"context"
	"testing"
	"time"

	"github.com/openshift/library-go/pkg/operator/events"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/clock"
)

func Test_NewResourceInformer(t *testing.T) {
	fakeClient := fake.NewClientset()
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	recorder := events.NewInMemoryRecorder("test", clock.RealClock{})

	informer, err := NewResourceInformer(recorder, informerFactory)
	if err != nil {
		t.Fatalf("Failed to create resource informer: %v", err)
	}

	if informer == nil {
		t.Fatal("Expected non-nil informer")
	}

	modifiedCh := informer.ResourceModified()
	if modifiedCh == nil {
		t.Fatal("Expected non-nil modification channel")
	}
}

func Test_ResourceInformer_DaemonSetUpdate(t *testing.T) {
	// Create fake client with initial DaemonSet
	initialDS := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       runtimeExtractorName,
			Namespace:  runtimeExtractorNamespace,
			Generation: 1,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	fakeClient := fake.NewClientset(initialDS)
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	recorder := events.NewInMemoryRecorder("test", clock.RealClock{})

	informer, err := NewResourceInformer(recorder, informerFactory)
	if err != nil {
		t.Fatalf("Failed to create resource informer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start informer
	informerFactory.Start(ctx.Done())
	go informer.Run(ctx, 1)

	// Wait for cache sync
	informerFactory.WaitForCacheSync(ctx.Done())

	// Update DaemonSet (simulate external modification)
	updatedDS := initialDS.DeepCopy()
	updatedDS.Generation = 2 // Generation change indicates spec change
	updatedDS.Spec.Template.Spec.HostPID = true

	_, err = fakeClient.AppsV1().DaemonSets(runtimeExtractorNamespace).Update(ctx, updatedDS, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update DaemonSet: %v", err)
	}

	// Verify notification was sent
	select {
	case <-informer.ResourceModified():
		// Success - received modification notification
	case <-time.After(2 * time.Second):
		t.Fatal("Expected modification notification but didn't receive one")
	}
}

func Test_ResourceInformer_DaemonSetDeletion(t *testing.T) {
	// Create fake client with initial DaemonSet
	initialDS := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      runtimeExtractorName,
			Namespace: runtimeExtractorNamespace,
		},
	}

	fakeClient := fake.NewClientset(initialDS)
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	recorder := events.NewInMemoryRecorder("test", clock.RealClock{})

	informer, err := NewResourceInformer(recorder, informerFactory)
	if err != nil {
		t.Fatalf("Failed to create resource informer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start informer
	informerFactory.Start(ctx.Done())
	go informer.Run(ctx, 1)

	// Wait for cache sync
	informerFactory.WaitForCacheSync(ctx.Done())

	// Delete DaemonSet (simulate external deletion)
	err = fakeClient.AppsV1().DaemonSets(runtimeExtractorNamespace).Delete(ctx, runtimeExtractorName, metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Failed to delete DaemonSet: %v", err)
	}

	// Verify notification was sent
	select {
	case <-informer.ResourceModified():
		// Success - received modification notification
	case <-time.After(2 * time.Second):
		t.Fatal("Expected deletion notification but didn't receive one")
	}
}

func Test_ResourceInformer_IgnoresOtherResources(t *testing.T) {
	// Create fake client with a DaemonSet in different namespace
	otherDS := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "other-daemonset",
			Namespace:  "other-namespace",
			Generation: 1,
		},
	}

	fakeClient := fake.NewClientset(otherDS)
	informerFactory := informers.NewSharedInformerFactory(fakeClient, 0)
	recorder := events.NewInMemoryRecorder("test", clock.RealClock{})

	informer, err := NewResourceInformer(recorder, informerFactory)
	if err != nil {
		t.Fatalf("Failed to create resource informer: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Start informer
	informerFactory.Start(ctx.Done())
	go informer.Run(ctx, 1)

	// Wait for cache sync
	informerFactory.WaitForCacheSync(ctx.Done())

	// Update the other DaemonSet
	updatedDS := otherDS.DeepCopy()
	updatedDS.Generation = 2

	_, err = fakeClient.AppsV1().DaemonSets("other-namespace").Update(ctx, updatedDS, metav1.UpdateOptions{})
	if err != nil {
		t.Fatalf("Failed to update DaemonSet: %v", err)
	}

	// Verify NO notification was sent (we don't care about other resources)
	select {
	case <-informer.ResourceModified():
		t.Fatal("Should not receive notification for other resources")
	case <-time.After(500 * time.Millisecond):
		// Success - no notification received
	}
}
