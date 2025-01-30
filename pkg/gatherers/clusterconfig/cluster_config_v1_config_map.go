package clusterconfig

import (
	"context"
	"fmt"

	installertypes "github.com/openshift/installer/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"sigs.k8s.io/yaml"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

// gatherClusterConfigV1 gathers "cluster-config-v1" from "kube-system" namespace leaving only "install-config" from data.
// "install-config" is anonymized.
func gatherClusterConfigV1(ctx context.Context, coreClient corev1client.CoreV1Interface) ([]record.Record, []error) {
	configMap, err := coreClient.ConfigMaps("kube-system").Get(ctx, "cluster-config-v1", metav1.GetOptions{})
	if err != nil {
		return nil, []error{err}
	}

	var installConfigBytes []byte

	if installConfigStr, found := configMap.Data["install-config"]; found {
		installConfig := &installertypes.InstallConfig{}
		err := yaml.Unmarshal([]byte(installConfigStr), installConfig)
		if err != nil {
			return nil, []error{err}
		}

		installConfig = anonymizeInstallConfig(installConfig)

		installConfigBytes, err = yaml.Marshal(installConfig)
		if err != nil {
			return nil, []error{err}
		}
	}

	return []record.Record{{Name: fmt.Sprintf("config/configmaps/%s/%s/install-config", configMap.Namespace, configMap.Name),
		Item: ConfigMapAnonymizer{v: installConfigBytes, encodeBase64: false}}}, nil
}

func anonymizeInstallConfig(installConfig *installertypes.InstallConfig) *installertypes.InstallConfig {
	installConfig.SSHKey = anonymize.String(installConfig.SSHKey)
	installConfig.PullSecret = anonymize.String(installConfig.PullSecret)
	// we don't use it
	installConfig.BaseDomain = anonymize.String(installConfig.BaseDomain)

	if installConfig.Platform.AWS != nil {
		installConfig.Platform.AWS.Region = anonymize.String(installConfig.Platform.AWS.Region)
	}
	if installConfig.Platform.Azure != nil {
		installConfig.Platform.Azure.Region = anonymize.String(installConfig.Platform.Azure.Region)
	}
	if installConfig.Platform.GCP != nil {
		installConfig.Platform.GCP.Region = anonymize.String(installConfig.Platform.GCP.Region)
		installConfig.Platform.GCP.ProjectID = anonymize.String(installConfig.Platform.GCP.ProjectID)
	}
	if installConfig.Platform.VSphere != nil {
		installConfig.Platform.VSphere.Datacenter = anonymize.String(installConfig.Platform.VSphere.Datacenter)
		installConfig.Platform.VSphere.Username = anonymize.String(installConfig.Platform.VSphere.Username)
		installConfig.Platform.VSphere.Password = anonymize.String(installConfig.Platform.VSphere.Password)
	}
	if installConfig.Platform.OpenStack != nil {
		installConfig.Platform.OpenStack.Cloud = anonymize.String(installConfig.Platform.OpenStack.Cloud)
	}

	return installConfig
}
