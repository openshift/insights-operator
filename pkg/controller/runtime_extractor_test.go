package controller

import (
	"context"
	"strings"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	fakeconfigclient "github.com/openshift/client-go/config/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func newFakeConfigClient(profile *configv1.TLSSecurityProfile) *fakeconfigclient.Clientset {
	apiserver := &configv1.APIServer{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster"},
		Spec: configv1.APIServerSpec{
			TLSSecurityProfile: profile,
		},
	}
	return fakeconfigclient.NewSimpleClientset(apiserver)
}

func Test_buildKubeRBACProxyArgs(t *testing.T) {
	tests := []struct {
		name                string
		profile             *configv1.TLSSecurityProfile
		expectError         bool
		expectCipherSuites  bool
		expectMinVersion    string
		expectCipherContain string
	}{
		{
			name: "Intermediate profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileIntermediateType,
			},
			expectCipherSuites: true,
			expectMinVersion:   string(configv1.TLSProfiles[configv1.TLSProfileIntermediateType].MinTLSVersion),
		},
		{
			name: "Old profile",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileOldType,
			},
			expectCipherSuites: true,
			expectMinVersion:   string(configv1.TLSProfiles[configv1.TLSProfileOldType].MinTLSVersion),
		},
		{
			name: "Modern profile (TLS 1.3 only, no cipher suites)",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileModernType,
			},
			expectCipherSuites: false,
			expectMinVersion:   string(configv1.TLSProfiles[configv1.TLSProfileModernType].MinTLSVersion),
		},
		{
			name: "Custom profile with valid ciphers",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers: []string{
							"ECDHE-RSA-AES128-GCM-SHA256",
							"ECDHE-RSA-AES256-GCM-SHA384",
						},
					},
				},
			},
			expectCipherSuites:  true,
			expectMinVersion:    "VersionTLS12",
			expectCipherContain: "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		},
		{
			name: "Custom profile with all unrecognized ciphers falls back to Intermediate",
			profile: &configv1.TLSSecurityProfile{
				Type: configv1.TLSProfileCustomType,
				Custom: &configv1.CustomTLSProfile{
					TLSProfileSpec: configv1.TLSProfileSpec{
						MinTLSVersion: configv1.VersionTLS12,
						Ciphers:       []string{"UNKNOWN-CIPHER-1", "UNKNOWN-CIPHER-2"},
					},
				},
			},
			expectCipherSuites: true,
			expectMinVersion:   string(configv1.TLSProfiles[configv1.TLSProfileIntermediateType].MinTLSVersion),
		},
		{
			name: "Custom profile with nil Custom field",
			profile: &configv1.TLSSecurityProfile{
				Type:   configv1.TLSProfileCustomType,
				Custom: nil,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := buildKubeRBACProxyArgs(tt.profile)
			if tt.expectError {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
			assert.NotEmpty(t, args)

			var foundMinVersion bool
			for _, arg := range args {
				if strings.HasPrefix(arg, "--tls-min-version=") {
					foundMinVersion = true
					assert.Equal(t, "--tls-min-version="+tt.expectMinVersion, arg)
				}
			}
			assert.True(t, foundMinVersion, "expected --tls-min-version arg")

			var foundCipherSuites bool
			for _, arg := range args {
				if strings.HasPrefix(arg, "--tls-cipher-suites=") {
					foundCipherSuites = true
					if tt.expectCipherContain != "" {
						assert.Contains(t, arg, tt.expectCipherContain)
					}
				}
			}
			assert.Equal(t, tt.expectCipherSuites, foundCipherSuites,
				"expected cipher suites presence to be %v", tt.expectCipherSuites)
		})
	}
}

func Test_parseDaemonSetManifest(t *testing.T) {
	ds, err := parseDaemonSetManifest(runtimeExtractorManifest)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "insights-runtime-extractor", ds.Name)
	assert.Equal(t, "openshift-insights", ds.Namespace)

	var found bool
	for _, c := range ds.Spec.Template.Spec.Containers {
		if c.Name == kubeRBACProxyContainerName {
			found = true
			break
		}
	}
	assert.True(t, found, "kube-rbac-proxy container should be present")
}

func Test_patchKubeRBACProxyArgs(t *testing.T) {
	ds, err := parseDaemonSetManifest(runtimeExtractorManifest)
	if !assert.NoError(t, err) {
		return
	}

	tlsArgs := []string{
		"--tls-cipher-suites=TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		"--tls-min-version=VersionTLS12",
	}

	err = patchKubeRBACProxyArgs(ds, tlsArgs)
	if !assert.NoError(t, err) {
		return
	}

	var args []string
	for _, c := range ds.Spec.Template.Spec.Containers {
		if c.Name == kubeRBACProxyContainerName {
			args = c.Args
			break
		}
	}

	assert.Contains(t, args, "--secure-listen-address=:8443")
	assert.Contains(t, args, "--tls-cipher-suites=TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256")
	assert.Contains(t, args, "--tls-min-version=VersionTLS12")

	var tlsCipherCount, tlsVersionCount int
	for _, arg := range args {
		if strings.HasPrefix(arg, "--tls-cipher-suites=") {
			tlsCipherCount++
		}
		if strings.HasPrefix(arg, "--tls-min-version=") {
			tlsVersionCount++
		}
	}
	assert.Equal(t, 1, tlsCipherCount, "should have exactly one --tls-cipher-suites arg")
	assert.Equal(t, 1, tlsVersionCount, "should have exactly one --tls-min-version arg")
}

func Test_patchKubeRBACProxyArgs_missingContainer(t *testing.T) {
	ds := &appsv1.DaemonSet{
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "other"},
					},
				},
			},
		},
	}
	err := patchKubeRBACProxyArgs(ds, []string{"--tls-min-version=VersionTLS12"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func Test_reconcileRuntimeExtractorDaemonSet_create(t *testing.T) {
	kubeClient := fake.NewSimpleClientset()
	configClient := newFakeConfigClient(nil)

	err := reconcileRuntimeExtractorDaemonSet(context.Background(), kubeClient, configClient)
	if !assert.NoError(t, err) {
		return
	}

	ds, err := kubeClient.AppsV1().DaemonSets("openshift-insights").Get(
		context.Background(), "insights-runtime-extractor", metav1.GetOptions{})
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "insights-runtime-extractor", ds.Name)

	var args []string
	for _, c := range ds.Spec.Template.Spec.Containers {
		if c.Name == kubeRBACProxyContainerName {
			args = c.Args
		}
	}
	var hasCipherSuites, hasMinVersion bool
	for _, arg := range args {
		if strings.HasPrefix(arg, "--tls-cipher-suites=") {
			hasCipherSuites = true
		}
		if strings.HasPrefix(arg, "--tls-min-version=") {
			hasMinVersion = true
		}
	}
	assert.True(t, hasCipherSuites, "should have --tls-cipher-suites")
	assert.True(t, hasMinVersion, "should have --tls-min-version")
}

func Test_reconcileRuntimeExtractorDaemonSet_update(t *testing.T) {
	kubeClient := fake.NewSimpleClientset()
	configClient := newFakeConfigClient(nil)

	err := reconcileRuntimeExtractorDaemonSet(context.Background(), kubeClient, configClient)
	if !assert.NoError(t, err) {
		return
	}

	configClient = newFakeConfigClient(&configv1.TLSSecurityProfile{
		Type: configv1.TLSProfileOldType,
	})

	err = reconcileRuntimeExtractorDaemonSet(context.Background(), kubeClient, configClient)
	if !assert.NoError(t, err) {
		return
	}

	ds, err := kubeClient.AppsV1().DaemonSets("openshift-insights").Get(
		context.Background(), "insights-runtime-extractor", metav1.GetOptions{})
	if !assert.NoError(t, err) {
		return
	}

	for _, c := range ds.Spec.Template.Spec.Containers {
		if c.Name == kubeRBACProxyContainerName {
			for _, arg := range c.Args {
				if strings.HasPrefix(arg, "--tls-min-version=") {
					expectedVersion := string(configv1.TLSProfiles[configv1.TLSProfileOldType].MinTLSVersion)
					assert.Equal(t, "--tls-min-version="+expectedVersion, arg)
				}
			}
		}
	}
}

func Test_reconcileRuntimeExtractorDaemonSet_noUpdateWhenUnchanged(t *testing.T) {
	kubeClient := fake.NewSimpleClientset()
	configClient := newFakeConfigClient(nil)

	err := reconcileRuntimeExtractorDaemonSet(context.Background(), kubeClient, configClient)
	if !assert.NoError(t, err) {
		return
	}

	err = reconcileRuntimeExtractorDaemonSet(context.Background(), kubeClient, configClient)
	if !assert.NoError(t, err) {
		return
	}
}

func Test_isTLSArg(t *testing.T) {
	assert.True(t, isTLSArg("--tls-cipher-suites=foo"))
	assert.True(t, isTLSArg("--tls-min-version=VersionTLS12"))
	assert.False(t, isTLSArg("--tls-cert-file=/path"))
	assert.False(t, isTLSArg("--secure-listen-address=:8443"))
}
