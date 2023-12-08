package conditional

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGatherer_areAllConditionsSatisfied(t *testing.T) {
	type fields struct {
		firingAlerts   map[string][]AlertLabels
		clusterVersion string
	}
	type args struct {
		conditions []ConditionWithParams
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    bool
		wantErr error
	}{
		{
			name: "invalid cluster version",
			fields: fields{
				firingAlerts:   map[string][]AlertLabels{},
				clusterVersion: "",
			},
			args: args{
				conditions: []ConditionWithParams{
					{
						Type:  AlertIsFiring,
						Alert: &AlertConditionParams{Name: "APIRemovedInNextEUSReleaseInUse"},
					},
					{
						Type: ClusterVersionMatches,
						ClusterVersionMatches: &ClusterVersionMatchesConditionParams{
							Version: "4.11.0",
						},
					},
				},
			},
			want:    false,
			wantErr: nil,
		},
		{
			name: "no conditions satisfied",
			fields: fields{
				firingAlerts:   map[string][]AlertLabels{},
				clusterVersion: "4.11.0",
			},
			args: args{
				conditions: []ConditionWithParams{
					{
						Type:  AlertIsFiring,
						Alert: &AlertConditionParams{Name: "APIRemovedInNextEUSReleaseInUse"},
					},
					{
						Type: ClusterVersionMatches,
						ClusterVersionMatches: &ClusterVersionMatchesConditionParams{
							Version: "<= 4.9.0",
						},
					},
				},
			},
			want:    false,
			wantErr: nil,
		},
		{
			name: "only cluster version satisfied",
			fields: fields{
				firingAlerts:   map[string][]AlertLabels{},
				clusterVersion: "4.9.0-0.nightly-2022-05-25-193227",
			},
			args: args{
				conditions: []ConditionWithParams{
					{
						Type: ClusterVersionMatches,
						ClusterVersionMatches: &ClusterVersionMatchesConditionParams{
							Version: "<= 4.11.0",
						},
					},
					{
						Type:  AlertIsFiring,
						Alert: &AlertConditionParams{Name: "APIRemovedInNextEUSReleaseInUse"},
					},
				},
			},
			want:    false,
			wantErr: nil,
		},
		{
			name: "only fire alert satisfied",
			fields: fields{
				firingAlerts:   map[string][]AlertLabels{"APIRemovedInNextEUSReleaseInUse": {}},
				clusterVersion: "4.11.0-0.nightly-2022-05-25-193227",
			},
			args: args{
				conditions: []ConditionWithParams{
					{
						Type:  AlertIsFiring,
						Alert: &AlertConditionParams{Name: "APIRemovedInNextEUSReleaseInUse"},
					},
					{
						Type: ClusterVersionMatches,
						ClusterVersionMatches: &ClusterVersionMatchesConditionParams{
							Version: "<= 4.9.0",
						},
					},
				},
			},
			want:    false,
			wantErr: nil,
		},
		{
			name: "both conditions satisfied",
			fields: fields{
				firingAlerts:   map[string][]AlertLabels{"APIRemovedInNextEUSReleaseInUse": {}},
				clusterVersion: "4.11.0-0.nightly-2022-05-25-193227",
			},
			args: args{
				conditions: []ConditionWithParams{
					{
						Type:  AlertIsFiring,
						Alert: &AlertConditionParams{Name: "APIRemovedInNextEUSReleaseInUse"},
					},
					{
						Type: ClusterVersionMatches,
						ClusterVersionMatches: &ClusterVersionMatchesConditionParams{
							Version: ">= 4.11.0",
						},
					},
				},
			},
			want:    true,
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := &Gatherer{
				firingAlerts:   tt.fields.firingAlerts,
				clusterVersion: tt.fields.clusterVersion,
			}
			got, err := g.areAllConditionsSatisfied(tt.args.conditions)

			assert.Equalf(t, tt.wantErr, err, fmt.Sprintf("want '%v', got '%v'", tt.wantErr, err))
			assert.Equalf(t, tt.want, got, "want '%v', got '%v'", tt.want, got)
		})
	}
}
