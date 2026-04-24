package controller

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"strings"

	configv1 "github.com/openshift/api/config/v1"
	configclientset "github.com/openshift/client-go/config/clientset/versioned"
	"github.com/openshift/library-go/pkg/crypto"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/insights/insightsclient"
)

//go:embed manifests/10-insights-runtime-extractor.yaml
var runtimeExtractorManifest []byte

const kubeRBACProxyContainerName = "kube-rbac-proxy"

func reconcileRuntimeExtractorDaemonSet(
	ctx context.Context, kubeClient kubernetes.Interface, configClient configclientset.Interface,
) error {
	desired, err := buildDesiredDaemonSet(configClient)
	if err != nil {
		return fmt.Errorf("failed to build desired DaemonSet: %w", err)
	}

	existing, err := kubeClient.AppsV1().DaemonSets(desired.Namespace).Get(ctx, desired.Name, metav1.GetOptions{})
	if errors.IsNotFound(err) {
		klog.Infof("Creating DaemonSet %s/%s", desired.Namespace, desired.Name)
		_, err = kubeClient.AppsV1().DaemonSets(desired.Namespace).Create(ctx, desired, metav1.CreateOptions{})
		return err
	}
	if err != nil {
		return fmt.Errorf("failed to get DaemonSet: %w", err)
	}

	if equality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		return nil
	}

	klog.Infof("Updating DaemonSet %s/%s with new TLS configuration", desired.Namespace, desired.Name)
	existing.Spec = desired.Spec
	_, err = kubeClient.AppsV1().DaemonSets(desired.Namespace).Update(ctx, existing, metav1.UpdateOptions{})
	return err
}

func buildDesiredDaemonSet(configClient configclientset.Interface) (*appsv1.DaemonSet, error) {
	ds, err := parseDaemonSetManifest(runtimeExtractorManifest)
	if err != nil {
		return nil, err
	}

	profile, err := insightsclient.GetTLSSecurityProfile(configClient)
	if err != nil {
		return nil, fmt.Errorf("failed to get TLS security profile: %w", err)
	}

	tlsArgs, err := buildKubeRBACProxyArgs(profile)
	if err != nil {
		return nil, fmt.Errorf("failed to build TLS args: %w", err)
	}

	if err := patchKubeRBACProxyArgs(ds, tlsArgs); err != nil {
		return nil, err
	}

	return ds, nil
}

func buildKubeRBACProxyArgs(profile *configv1.TLSSecurityProfile) ([]string, error) {
	profileSpec, err := insightsclient.GetTLSProfileSpec(profile)
	if err != nil {
		return nil, fmt.Errorf("failed to get TLS profile spec: %w", err)
	}

	var args []string

	// TLS 1.3 cipher suites are fixed by the protocol and not configurable
	if profileSpec.MinTLSVersion != configv1.VersionTLS13 {
		cipherNames := crypto.OpenSSLToIANACipherSuites(profileSpec.Ciphers)
		if len(cipherNames) == 0 {
			intermediateSpec := configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
			cipherNames = crypto.OpenSSLToIANACipherSuites(intermediateSpec.Ciphers)
			profileSpec = intermediateSpec
		}
		args = append(args, fmt.Sprintf("--tls-cipher-suites=%s",
			strings.Join(cipherNames, ",")))
	}

	args = append(args, fmt.Sprintf("--tls-min-version=%s", profileSpec.MinTLSVersion))

	return args, nil
}

func parseDaemonSetManifest(data []byte) (*appsv1.DaemonSet, error) {
	ds := &appsv1.DaemonSet{}
	if err := yaml.NewYAMLOrJSONDecoder(
		bytes.NewReader(data), 4096,
	).Decode(ds); err != nil {
		return nil, fmt.Errorf("failed to decode DaemonSet manifest: %w", err)
	}
	return ds, nil
}

func patchKubeRBACProxyArgs(ds *appsv1.DaemonSet, tlsArgs []string) error {
	containers := ds.Spec.Template.Spec.Containers
	for i := range containers {
		if containers[i].Name != kubeRBACProxyContainerName {
			continue
		}

		filteredArgs := make([]string, 0, len(containers[i].Args)+len(tlsArgs))
		for _, arg := range containers[i].Args {
			if !isTLSArg(arg) {
				filteredArgs = append(filteredArgs, arg)
			}
		}
		filteredArgs = append(filteredArgs, tlsArgs...)
		containers[i].Args = filteredArgs
		return nil
	}
	return fmt.Errorf("container %q not found in DaemonSet", kubeRBACProxyContainerName)
}

func isTLSArg(arg string) bool {
	return strings.HasPrefix(arg, "--tls-cipher-suites=") || strings.HasPrefix(arg, "--tls-min-version=")
}

type tlsReconcileHandler struct {
	ctx          context.Context
	kubeClient   kubernetes.Interface
	configClient configclientset.Interface
}

func newTLSReconcileHandler(
	ctx context.Context, kubeClient kubernetes.Interface, configClient configclientset.Interface,
) *tlsReconcileHandler {
	return &tlsReconcileHandler{ctx: ctx, kubeClient: kubeClient, configClient: configClient}
}

func (h *tlsReconcileHandler) OnAdd(_ interface{}, _ bool) {
	h.reconcile()
}

func (h *tlsReconcileHandler) OnUpdate(_, _ interface{}) {
	h.reconcile()
}

func (h *tlsReconcileHandler) OnDelete(_ interface{}) {}

func (h *tlsReconcileHandler) reconcile() {
	if err := reconcileRuntimeExtractorDaemonSet(h.ctx, h.kubeClient, h.configClient); err != nil {
		klog.Errorf("Failed to reconcile runtime extractor DaemonSet on TLS config change: %v", err)
	}
}
