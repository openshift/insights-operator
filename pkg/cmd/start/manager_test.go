package start

import (
	"context"
	"crypto/tls"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
)

func Test_NewManager(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = configv1.AddToScheme(scheme)

	tests := []struct {
		name       string
		config     *rest.Config
		scheme     *runtime.Scheme
		namespace  string
		wantErr    bool
		errContain string
	}{
		{
			name:       "nil client config",
			config:     nil,
			scheme:     scheme,
			namespace:  "test-namespace",
			wantErr:    true,
			errContain: "failed to create Kubernetes client",
		},
		{
			name: "invalid client config - cannot fetch TLS profile",
			config: &rest.Config{
				Host: "https://invalid-host-that-does-not-exist:6443",
			},
			scheme:     scheme,
			namespace:  "test-namespace",
			wantErr:    true,
			errContain: "unable to get TLS profile from API server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cancelFunc := func() {}

			mgr, err := NewManager(ctx, tt.config, tt.scheme, tt.namespace, cancelFunc)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContain != "" {
					assert.Contains(t, err.Error(), tt.errContain)
				}
				assert.Nil(t, mgr)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, mgr)
			}
		})
	}
}

func Test_createManager(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)

	tlsConfigFn := func(cfg *tls.Config) {
		cfg.MinVersion = tls.VersionTLS12
	}

	tests := []struct {
		name        string
		config      *rest.Config
		tlsConfigFn func(*tls.Config)
		scheme      *runtime.Scheme
		namespace   string
		wantErr     bool
	}{
		{
			name:        "nil client config fails",
			config:      nil,
			tlsConfigFn: tlsConfigFn,
			scheme:      scheme,
			namespace:   "test-namespace",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := createManager(tt.config, tt.tlsConfigFn, tt.scheme, tt.namespace)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, mgr)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, mgr)
			}
		})
	}
}
