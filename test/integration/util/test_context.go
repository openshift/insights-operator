package util

import (
	"context"
	"fmt"
	"os"

	"k8s.io/client-go/tools/clientcmd"
)

// InitTest initializes the test context from kubeconfig
// This sets up clients for interacting with the cluster
func InitTest(ctx context.Context) error {
	// Verify KUBECONFIG is set
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		return fmt.Errorf("KUBECONFIG environment variable must be set")
	}

	// Verify kubeconfig is valid
	_, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	return nil
}
