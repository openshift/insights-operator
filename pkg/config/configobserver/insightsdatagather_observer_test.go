package configobserver

import (
	"context"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	fakeConfigCli "github.com/openshift/client-go/config/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInsightsDataGatherSync(t *testing.T) {
	tests := []struct {
		name                        string
		insightsDatagatherToUpdated *configv1.InsightsDataGather
		expectedGatherConfig        *configv1.GatherConfig
		expectedDisable             bool
	}{
		{
			name: "Obfuscation configured and some disabled gatherers",
			insightsDatagatherToUpdated: &configv1.InsightsDataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: configv1.InsightsDataGatherSpec{
					GatherConfig: configv1.GatherConfig{
						DataPolicy: []configv1.DataPolicyOption{
							configv1.DataPolicyOptionObfuscateNetworking,
						},
						Gatherers: configv1.Gatherers{
							Mode: configv1.GatheringModeCustom,
							Custom: configv1.Custom{
								Configs: []configv1.GathererConfig{
									{
										Name:  "fooBar",
										State: configv1.GathererStateDisabled,
									},
									{
										Name:  "barrGather",
										State: configv1.GathererStateDisabled,
									},
								},
							},
						},
					},
				},
			},
			expectedGatherConfig: &configv1.GatherConfig{
				DataPolicy: []configv1.DataPolicyOption{
					configv1.DataPolicyOptionObfuscateNetworking,
				},
				Gatherers: configv1.Gatherers{
					Mode: configv1.GatheringModeCustom,
					Custom: configv1.Custom{
						Configs: []configv1.GathererConfig{
							{
								Name:  "fooBar",
								State: configv1.GathererStateDisabled,
							},
							{
								Name:  "barrGather",
								State: configv1.GathererStateDisabled,
							},
						},
					},
				},
			},
			expectedDisable: false,
		},
		{
			name: "Gathering disabled and no obfuscation",
			insightsDatagatherToUpdated: &configv1.InsightsDataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: configv1.InsightsDataGatherSpec{
					GatherConfig: configv1.GatherConfig{
						DataPolicy: []configv1.DataPolicyOption{
							configv1.DataPolicyOptionObfuscateNetworking,
						},
						Gatherers: configv1.Gatherers{
							Mode: configv1.GatheringModeNone,
						},
					},
				},
			},
			expectedGatherConfig: &configv1.GatherConfig{
				DataPolicy: []configv1.DataPolicyOption{
					configv1.DataPolicyOptionObfuscateNetworking,
				},
				Gatherers: configv1.Gatherers{
					Mode: configv1.GatheringModeNone,
				},
			},
			expectedDisable: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			insightDefaultConfig := &configv1.InsightsDataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
			}

			client := fakeConfigCli.NewSimpleClientset(insightDefaultConfig)
			idgObserver := insightsDataGatherController{
				gatherConfig: &insightDefaultConfig.Spec.GatherConfig,
				cli:          client.ConfigV1(),
			}
			err := idgObserver.sync(context.Background(), nil)
			assert.NoError(t, err)

			assert.Equal(t, &insightDefaultConfig.Spec.GatherConfig, idgObserver.GatherConfig())
			_, err = idgObserver.cli.InsightsDataGathers().Update(context.Background(), tt.insightsDatagatherToUpdated, metav1.UpdateOptions{})
			assert.NoError(t, err)
			err = idgObserver.sync(context.Background(), nil)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedGatherConfig, idgObserver.GatherConfig())
			assert.Equal(t, tt.expectedDisable, idgObserver.GatherDisabled())
		})
	}
}
