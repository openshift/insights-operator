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
func gatherClusterConfigV1(ctx context.Context, coreClient corev1client.CoreV1Interface) (record.Record, []error) {
	configMap, err := coreClient.ConfigMaps("kube-system").Get(ctx, "cluster-config-v1", metav1.GetOptions{})
	if err != nil {
		return record.Record{}, []error{err}
	}

	newData := make(map[string]string)

	if installConfigStr, found := configMap.Data["install-config"]; found {
		installConfig := &installertypes.InstallConfig{}
		err := yaml.Unmarshal([]byte(installConfigStr), installConfig)
		if err != nil {
			return record.Record{}, []error{err}
		}

		installConfig = anonymizeInstallConfig(installConfig)

		installConfigBytes, err := yaml.Marshal(installConfig)
		if err != nil {
			return record.Record{}, []error{err}
		}

		newData["install-config"] = string(installConfigBytes)
	}

	configMap.Data = newData

	return record.Record{
		Name: fmt.Sprintf("config/configmaps/%s/%s", configMap.Namespace, configMap.Name),
		Item: record.JSONMarshaller{Object: configMap},
	}, nil
}

func anonymizeInstallConfig(installConfig *installertypes.InstallConfig) *installertypes.InstallConfig {
	installConfig.SSHKey = anonymize.String(installConfig.SSHKey)
	installConfig.PullSecret = anonymize.String(installConfig.PullSecret)

	return installConfig
}
