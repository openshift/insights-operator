package clusterconfig

import (
	"context"
	"fmt"

	installertypes "github.com/openshift/installer/pkg/types"
	"github.com/openshift/installer/pkg/types/gcp"
	"github.com/openshift/installer/pkg/types/vsphere"
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

	return []record.Record{{
		Name: fmt.Sprintf("config/configmaps/%s/%s/install-config", configMap.Namespace, configMap.Name),
		Item: ConfigMapAnonymizer{v: installConfigBytes, encodeBase64: false},
	}}, nil
}

func anonymizeInstallConfig(installConfig *installertypes.InstallConfig) *installertypes.InstallConfig {
	installConfig.SSHKey = anonymize.String(installConfig.SSHKey)
	installConfig.PullSecret = anonymize.String(installConfig.PullSecret)
	// we don't use it
	installConfig.BaseDomain = anonymize.String(installConfig.BaseDomain)

	if installConfig.ControlPlane != nil {
		anonymizeFencing(installConfig.ControlPlane.Fencing)
	}

	if installConfig.Arbiter != nil {
		anonymizeFencing(installConfig.Arbiter.Fencing)
	}

	for i := range installConfig.Compute {
		anonymizeFencing(installConfig.Compute[i].Fencing)
	}

	if installConfig.AWS != nil {
		installConfig.AWS.Region = anonymize.String(installConfig.AWS.Region)
	}

	if installConfig.Azure != nil {
		installConfig.Azure.Region = anonymize.String(installConfig.Azure.Region)
	}

	if installConfig.BareMetal != nil {
		for i := range installConfig.BareMetal.Hosts {
			installConfig.BareMetal.Hosts[i].BMC.Username = anonymize.String(installConfig.BareMetal.Hosts[i].BMC.Username)
			installConfig.BareMetal.Hosts[i].BMC.Password = anonymize.String(installConfig.BareMetal.Hosts[i].BMC.Password)
			installConfig.BareMetal.Hosts[i].BMC.Address = anonymize.String(installConfig.BareMetal.Hosts[i].BMC.Address)
		}
	}

	if installConfig.GCP != nil {
		anonymizeGCPConfig(installConfig.GCP)
	}

	if installConfig.VSphere != nil {
		anonymizeVSphere(installConfig.VSphere)
	}

	if installConfig.OpenStack != nil {
		installConfig.OpenStack.Cloud = anonymize.String(installConfig.OpenStack.Cloud)
	}

	return installConfig
}

func anonymizeFencing(fencing *installertypes.Fencing) {
	if fencing == nil {
		return
	}

	for i := range fencing.Credentials {
		cred := fencing.Credentials[i]
		if cred == nil {
			continue
		}
		cred.HostName = anonymize.String(cred.HostName)
		cred.Username = anonymize.String(cred.Username)
		cred.Password = anonymize.String(cred.Password)
		cred.Address = anonymize.String(cred.Address)
	}
}

func anonymizeVSphere(vspherePlatform *vsphere.Platform) {
	for i := range vspherePlatform.FailureDomains {
		vspherePlatform.FailureDomains[i].Topology.Datacenter = anonymize.String(
			vspherePlatform.FailureDomains[i].Topology.Datacenter,
		)
	}
	for i := range vspherePlatform.VCenters {
		vspherePlatform.VCenters[i].Username = anonymize.String(vspherePlatform.VCenters[i].Username)
		vspherePlatform.VCenters[i].Password = anonymize.String(vspherePlatform.VCenters[i].Password)
	}
}

func anonymizeGCPConfig(gcpPlatform *gcp.Platform) {
	gcpPlatform.Region = anonymize.String(gcpPlatform.Region)
	gcpPlatform.ProjectID = anonymize.String(gcpPlatform.ProjectID)

	if gcpPlatform.DNS != nil && gcpPlatform.DNS.PrivateZone != nil {
		gcpPlatform.DNS.PrivateZone.ProjectID = anonymize.String(gcpPlatform.DNS.PrivateZone.ProjectID)
	}
}
