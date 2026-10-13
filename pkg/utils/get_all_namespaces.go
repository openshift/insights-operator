package utils

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

func GetAllNamespaces(ctx context.Context, coreClient corev1client.CoreV1Interface) (*corev1.NamespaceList, error) {
	ns, err := coreClient.Namespaces().List(ctx, metav1.ListOptions{Limit: maxNamespacesLimit})
	if err != nil {
		return nil, err
	}
	return ns, nil
}
