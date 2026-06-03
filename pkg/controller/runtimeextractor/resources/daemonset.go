package resources

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	utiltls "github.com/openshift/controller-runtime-common/pkg/tls"
	"github.com/openshift/library-go/pkg/crypto"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/yaml"
)

const (
	daemonSetName      = "insights-runtime-extractor"
	daemonSetNamespace = "openshift-insights"

	// Environment variables containinig container image references for runtime-extractor
	// related services. These ENVs are populated by the CVO operator.
	extractorImageEnv     = "RELATED_IMAGE_INSIGHTS_RUNTIME_EXTRACTOR"
	extractorDefaultImage = "quay.io/openshift/origin-insights-runtime-extractor:latest"

	exporterImageEnv     = "RELATED_IMAGE_INSIGHTS_RUNTIME_EXPORTER"
	exporterDefaultImage = "quay.io/openshift/origin-insights-runtime-exporter:latest"

	proxyImageEnv              = "RELATED_IMAGE_KUBE_RBAC_PROXY"
	proxyDefaultImage          = "quay.io/openshift/origin-kube-rbac-proxy:latest"
	kubeRbacProxyContainerName = "kube-rbac-proxy"

	envImageErrMsg = "Failed to get image from environment variable %s, using default image %s"
)

//go:embed manifests/runtime-extractor-daemonset.yaml
var runtimeExtractorDaemonSetYAML []byte

// loadRuntimeExtractorDaemonSet loads the embedded DaemonSet YAML and unmarshals it
func loadRuntimeExtractorDaemonSet() (*appsv1.DaemonSet, error) {
	ds := &appsv1.DaemonSet{}
	if err := yaml.Unmarshal(runtimeExtractorDaemonSetYAML, ds); err != nil {
		return nil, fmt.Errorf("failed to unmarshal runtime extractor daemonset: %w", err)
	}
	return ds, nil
}

// applyDaemonSet creates or updates the runtime extractor DaemonSet
// Retries on conflict errors using exponential backoff
func (rm *ResourceManager) applyDaemonSet(ctx context.Context, tlsProfile *configv1.TLSSecurityProfile) (*appsv1.DaemonSet, error) {
	daemonSet, err := loadRuntimeExtractorDaemonSet()
	if err != nil {
		return nil, err
	}

	rm.updateContainerImages(daemonSet)
	updateKubeRBACProxyTLSArgs(daemonSet, tlsProfile)

	// Retry with exponential backoff on conflict errors
	var appliedDaemonSet *appsv1.DaemonSet
	var modified bool

	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		var retryErr error
		appliedDaemonSet, modified, retryErr = resourceapply.ApplyDaemonSet(ctx, rm.daemonSetGetterClient, rm.recorder, daemonSet, -1)
		return retryErr
	})
	if err != nil {
		return nil, fmt.Errorf("failed to apply runtime extractor daemonset: %w", err)
	}

	if modified {
		rm.recorder.Event(
			"DaemonSet Updated",
			fmt.Sprintf(
				"Runtime extractor DaemonSet %s/%s was created or updated",
				appliedDaemonSet.Namespace, appliedDaemonSet.Name,
			))
		klog.Infof("Runtime extractor DaemonSet %s/%s was created or updated", appliedDaemonSet.Namespace, appliedDaemonSet.Name)
	}

	return appliedDaemonSet, nil
}

// updateContainerImages updates the container images to the version specified by the CVO operator
func (rm *ResourceManager) updateContainerImages(ds *appsv1.DaemonSet) {
	extractorReleaseVersion, exporterReleaseVersion, proxyReleaseVersion := loadImagesFromEnvs()

	for i := range ds.Spec.Template.Spec.Containers {
		container := &ds.Spec.Template.Spec.Containers[i]
		switch container.Name {
		case "extractor":
			container.Image = extractorReleaseVersion
			klog.Infof("Updated runtime extractor container image to %s", container.Image)
		case "exporter":
			container.Image = exporterReleaseVersion
			klog.Infof("Updated runtime exporter container image to %s", container.Image)
		case kubeRbacProxyContainerName:
			// kube-rbac-proxy uses its own versioning, keep as-is
			// Could be updated separately if needed
			container.Image = proxyReleaseVersion
			klog.Infof("Updated kube-rbac-proxy container image to %s", container.Image)
		}
	}
}

// kubeRBACProxyTLSArgs generates --tls-cipher-suites and --tls-min-version
// arguments for kube-rbac-proxy based on the cluster's TLS security profile.
func kubeRBACProxyTLSArgs(profile *configv1.TLSSecurityProfile) []string {
	profileSpec, err := utiltls.GetTLSProfileSpec(profile)
	if err != nil {
		klog.Warningf("Failed to get TLS profile spec, using Intermediate: %v", err)
		profileSpec = *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
	}

	cipherNames := crypto.OpenSSLToIANACipherSuites(profileSpec.Ciphers)

	var supportedCiphers []string
	for _, name := range cipherNames {
		if _, err := crypto.CipherSuite(name); err != nil {
			klog.Warningf("Dropping unsupported TLS cipher %q", name)
			continue
		}
		supportedCiphers = append(supportedCiphers, name)
	}

	if len(supportedCiphers) == 0 {
		klog.Warning("All TLS ciphers unsupported, falling back to Intermediate profile")
		profileSpec = *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
		cipherNames = crypto.OpenSSLToIANACipherSuites(profileSpec.Ciphers)
		supportedCiphers = make([]string, 0, len(cipherNames))
		for _, name := range cipherNames {
			if _, err := crypto.CipherSuite(name); err == nil {
				supportedCiphers = append(supportedCiphers, name)
			}
		}
	}

	return []string{
		"--tls-cipher-suites=" + strings.Join(supportedCiphers, ","),
		"--tls-min-version=" + string(profileSpec.MinTLSVersion),
	}
}

// updateKubeRBACProxyTLSArgs appends TLS cipher and version args to the
// kube-rbac-proxy container based on the cluster's TLS security profile.
func updateKubeRBACProxyTLSArgs(ds *appsv1.DaemonSet, profile *configv1.TLSSecurityProfile) {
	tlsArgs := kubeRBACProxyTLSArgs(profile)
	for i := range ds.Spec.Template.Spec.Containers {
		if ds.Spec.Template.Spec.Containers[i].Name == kubeRbacProxyContainerName {
			ds.Spec.Template.Spec.Containers[i].Args = append(
				ds.Spec.Template.Spec.Containers[i].Args,
				tlsArgs...,
			)
			return
		}
	}
}

// loadImagesFromEnvs loads container image references from environment variables.
// Default values are returned for any missing environment variables, with errors logged.
func loadImagesFromEnvs() (extractorReleaseVersion, exporterReleaseVersion, proxyReleaseVersion string) {
	extractorReleaseVersion = os.Getenv(extractorImageEnv)
	if len(extractorReleaseVersion) == 0 {
		klog.Errorf(envImageErrMsg, extractorImageEnv, extractorDefaultImage)
		extractorReleaseVersion = extractorDefaultImage
	}

	exporterReleaseVersion = os.Getenv(exporterImageEnv)
	if len(exporterReleaseVersion) == 0 {
		klog.Errorf(envImageErrMsg, exporterImageEnv, exporterDefaultImage)
		exporterReleaseVersion = exporterDefaultImage
	}

	proxyReleaseVersion = os.Getenv(proxyImageEnv)
	if len(proxyReleaseVersion) == 0 {
		klog.Errorf(envImageErrMsg, proxyImageEnv, proxyDefaultImage)
		proxyReleaseVersion = proxyDefaultImage
	}

	return extractorReleaseVersion, exporterReleaseVersion, proxyReleaseVersion
}

// deleteDaemonSet removes the runtime extractor DaemonSet
func (rm *ResourceManager) deleteDaemonSet(ctx context.Context) error {
	err := rm.daemonSetGetterClient.DaemonSets(daemonSetNamespace).Delete(ctx, daemonSetName, metav1.DeleteOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			klog.Infof("Runtime extractor DaemonSet %s/%s already deleted", daemonSetNamespace, daemonSetName)
			return nil
		}
		return fmt.Errorf("failed to delete runtime extractor daemonset: %w", err)
	}

	rm.recorder.Event("DaemonSet Deleted", fmt.Sprintf("Runtime extractor DaemonSet %s/%s deleted", daemonSetNamespace, daemonSetName))
	klog.Infof("Runtime extractor DaemonSet %s/%s deleted", daemonSetNamespace, daemonSetName)
	return nil
}

// getDaemonSet retrieves the runtime extractor DaemonSet
func (rm *ResourceManager) getDaemonSet(ctx context.Context) (*appsv1.DaemonSet, error) {
	return rm.daemonSetGetterClient.DaemonSets(daemonSetNamespace).Get(ctx, daemonSetName, metav1.GetOptions{})
}

// daemonSetExists checks if the runtime extractor DaemonSet exists
func (rm *ResourceManager) daemonSetExists(ctx context.Context) bool {
	_, err := rm.getDaemonSet(ctx)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false
		}
		klog.Errorf("Failed to get runtime extractor DaemonSet %s/%s: %v", daemonSetNamespace, daemonSetName, err)
		return false
	}
	return true
}
