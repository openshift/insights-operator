package status

import (
	"reflect"
	"testing"

	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_conditions_entries(t *testing.T) {
	time := metav1.Now()

	type fields struct {
		entryMap conditionsMap
	}
	tests := []struct {
		name   string
		fields fields
		want   []configv1.ClusterOperatorStatusCondition
	}{
		{
			name: "Can get the condition status from entry map",
			fields: fields{entryMap: map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{
				configv1.OperatorAvailable: {
					Type:               configv1.OperatorAvailable,
					Status:             configv1.ConditionUnknown,
					LastTransitionTime: time,
					Reason:             "",
				},
				configv1.OperatorProgressing: {
					Type:               configv1.OperatorProgressing,
					Status:             configv1.ConditionUnknown,
					LastTransitionTime: time,
					Reason:             "",
				},
			}},
			want: []configv1.ClusterOperatorStatusCondition{
				{
					Type:               configv1.OperatorAvailable,
					Status:             configv1.ConditionUnknown,
					LastTransitionTime: time,
					Reason:             "",
				},
				{
					Type:               configv1.OperatorProgressing,
					Status:             configv1.ConditionUnknown,
					LastTransitionTime: time,
					Reason:             "",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &conditions{
				entryMap: tt.fields.entryMap,
			}
			if got := c.entries(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("entries() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_conditions_findCondition(t *testing.T) {
	time := metav1.Now()

	type fields struct {
		entryMap conditionsMap
	}
	type args struct {
		condition configv1.ClusterStatusConditionType
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *configv1.ClusterOperatorStatusCondition
	}{
		{
			name: "Can find the condition status",
			fields: fields{entryMap: map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{
				configv1.OperatorAvailable: {
					Type:               configv1.OperatorAvailable,
					Status:             configv1.ConditionUnknown,
					LastTransitionTime: time,
					Reason:             "",
				},
			}},
			args: args{
				condition: configv1.OperatorAvailable,
			},
			want: &configv1.ClusterOperatorStatusCondition{
				Type:               configv1.OperatorAvailable,
				Status:             configv1.ConditionUnknown,
				LastTransitionTime: time,
				Reason:             "",
			},
		},
		{
			name: "Can't find the condition status",
			fields: fields{entryMap: map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{
				configv1.OperatorAvailable: {
					Type:               configv1.OperatorAvailable,
					Status:             configv1.ConditionUnknown,
					LastTransitionTime: time,
					Reason:             "",
				},
			}},
			args: args{
				condition: configv1.OperatorDegraded,
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &conditions{
				entryMap: tt.fields.entryMap,
			}
			if got := c.findCondition(tt.args.condition); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("findCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_conditions_hasCondition(t *testing.T) {
	time := metav1.Now()

	type fields struct {
		entryMap conditionsMap
	}
	type args struct {
		condition configv1.ClusterStatusConditionType
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   bool
	}{
		{
			name: "Condition exists",
			fields: fields{entryMap: map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{
				configv1.OperatorAvailable: {
					Type:               configv1.OperatorAvailable,
					Status:             configv1.ConditionUnknown,
					LastTransitionTime: time,
					Reason:             "",
				},
			}},
			args: args{
				condition: configv1.OperatorAvailable,
			},
			want: true,
		},
		{
			name: "Condition doesn't exists",
			fields: fields{entryMap: map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{
				configv1.OperatorAvailable: {
					Type:               configv1.OperatorAvailable,
					Status:             configv1.ConditionUnknown,
					LastTransitionTime: time,
					Reason:             "",
				},
			}},
			args: args{
				condition: configv1.OperatorDegraded,
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &conditions{
				entryMap: tt.fields.entryMap,
			}
			if got := c.hasCondition(tt.args.condition); got != tt.want {
				t.Errorf("hasCondition() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_conditions_removeCondition(t *testing.T) {
	time := metav1.Now()

	type fields struct {
		entryMap conditionsMap
	}
	type args struct {
		condition configv1.ClusterStatusConditionType
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *conditions
	}{
		{
			name: "Removing non existing condition",
			fields: fields{entryMap: map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{
				configv1.OperatorAvailable: {
					Type:               configv1.OperatorAvailable,
					Status:             configv1.ConditionUnknown,
					LastTransitionTime: time,
					Reason:             "",
				},
			}},
			args: args{
				condition: configv1.OperatorDegraded,
			},
			want: &conditions{
				entryMap: map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{
					configv1.OperatorAvailable: {
						Type:               configv1.OperatorAvailable,
						Status:             configv1.ConditionUnknown,
						LastTransitionTime: time,
						Reason:             "",
					},
				},
			},
		},
		{
			name: "Remove existing condition",
			fields: fields{entryMap: map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{
				configv1.OperatorAvailable: {
					Type:               configv1.OperatorAvailable,
					Status:             configv1.ConditionUnknown,
					LastTransitionTime: time,
					Reason:             "",
				},
				configv1.OperatorDegraded: {
					Type:               configv1.OperatorDegraded,
					Status:             configv1.ConditionUnknown,
					LastTransitionTime: time,
					Reason:             "",
				},
			}},
			args: args{
				condition: configv1.OperatorAvailable,
			},
			want: &conditions{
				entryMap: map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{
					configv1.OperatorDegraded: {
						Type:               configv1.OperatorDegraded,
						Status:             configv1.ConditionUnknown,
						LastTransitionTime: time,
						Reason:             "",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &conditions{
				entryMap: tt.fields.entryMap,
			}
			c.removeCondition(tt.args.condition)
			if !reflect.DeepEqual(c, tt.want) {
				t.Errorf("removeCondition() = %v, want %v", c, tt.want)
			}
		})
	}
}

func Test_conditions_setCondition(t *testing.T) {
	time := metav1.Now()

	type fields struct {
		entryMap conditionsMap
	}
	type args struct {
		condition configv1.ClusterStatusConditionType
		status    configv1.ConditionStatus
		reason    string
		message   string
		lastTime  metav1.Time
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   *conditions
	}{
		{
			name: "Set not existing condition",
			fields: fields{entryMap: map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{
				configv1.OperatorAvailable: {
					Type:               configv1.OperatorAvailable,
					Status:             configv1.ConditionUnknown,
					LastTransitionTime: time,
					Reason:             "",
				},
			}},
			args: args{
				condition: configv1.OperatorDegraded,
				status:    configv1.ConditionUnknown,
				reason:    "degraded reason",
				message:   "degraded message",
				lastTime:  time,
			},
			want: &conditions{
				entryMap: map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{
					configv1.OperatorAvailable: {
						Type:               configv1.OperatorAvailable,
						Status:             configv1.ConditionUnknown,
						LastTransitionTime: time,
						Reason:             "",
					},
					configv1.OperatorDegraded: {
						Type:               configv1.OperatorDegraded,
						Status:             configv1.ConditionUnknown,
						LastTransitionTime: time,
						Reason:             "degraded reason",
						Message:            "degraded message",
					},
				},
			},
		},
		{
			name: "Set existing condition",
			fields: fields{entryMap: map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{
				configv1.OperatorAvailable: {
					Type:               configv1.OperatorAvailable,
					Status:             configv1.ConditionUnknown,
					LastTransitionTime: time,
					Reason:             "",
				},
				configv1.OperatorDegraded: {
					Type:               configv1.OperatorDegraded,
					Status:             configv1.ConditionUnknown,
					LastTransitionTime: time,
					Reason:             "",
				},
			}},
			args: args{
				condition: configv1.OperatorAvailable,
				status:    configv1.ConditionTrue,
				reason:    "available reason",
				message:   "",
				lastTime:  time,
			},
			want: &conditions{
				entryMap: map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{
					configv1.OperatorAvailable: {
						Type:               configv1.OperatorAvailable,
						Status:             configv1.ConditionTrue,
						LastTransitionTime: time,
						Reason:             "available reason",
					},
					configv1.OperatorDegraded: {
						Type:               configv1.OperatorDegraded,
						Status:             configv1.ConditionUnknown,
						LastTransitionTime: time,
						Reason:             "",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &conditions{
				entryMap: tt.fields.entryMap,
			}
			c.setCondition(tt.args.condition, tt.args.status, tt.args.reason, tt.args.message, time)
			if !reflect.DeepEqual(c, tt.want) {
				t.Errorf("setConditions() = %v, want %v", c, tt.want)
			}
		})
	}
}

func Test_newConditions(t *testing.T) {
	time := metav1.Now()

	type args struct {
		cos  *configv1.ClusterOperatorStatus
		time metav1.Time
	}
	tests := []struct {
		name string
		args args
		want *conditions
	}{
		{
			name: "Test newConditions constructor (empty)",
			args: args{
				cos:  &configv1.ClusterOperatorStatus{Conditions: nil},
				time: time,
			},
			want: &conditions{
				entryMap: map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{
					configv1.OperatorAvailable: {
						Type:               configv1.OperatorAvailable,
						Status:             configv1.ConditionUnknown,
						LastTransitionTime: time,
						Reason:             "",
					},
					configv1.OperatorProgressing: {
						Type:               configv1.OperatorProgressing,
						Status:             configv1.ConditionUnknown,
						LastTransitionTime: time,
						Reason:             "",
					},
					configv1.OperatorDegraded: {
						Type:               configv1.OperatorDegraded,
						Status:             configv1.ConditionUnknown,
						LastTransitionTime: time,
						Reason:             "",
					},
					configv1.OperatorUpgradeable: {
						Type:               configv1.OperatorUpgradeable,
						Status:             configv1.ConditionUnknown,
						LastTransitionTime: time,
						Reason:             "",
					},
				},
			},
		},
		{
			name: "Test newConditions constructor",
			args: args{
				cos: &configv1.ClusterOperatorStatus{
					Conditions: []configv1.ClusterOperatorStatusCondition{
						{
							Type:               configv1.OperatorDegraded,
							Status:             configv1.ConditionUnknown,
							LastTransitionTime: time,
							Reason:             "degraded reason",
							Message:            "degraded message",
						},
					},
				},
				time: time,
			},
			want: &conditions{
				entryMap: map[configv1.ClusterStatusConditionType]configv1.ClusterOperatorStatusCondition{
					configv1.OperatorAvailable: {
						Type:               configv1.OperatorAvailable,
						Status:             configv1.ConditionUnknown,
						LastTransitionTime: time,
						Reason:             "",
					},
					configv1.OperatorProgressing: {
						Type:               configv1.OperatorProgressing,
						Status:             configv1.ConditionUnknown,
						LastTransitionTime: time,
						Reason:             "",
					},
					configv1.OperatorDegraded: {
						Type:               configv1.OperatorDegraded,
						Status:             configv1.ConditionUnknown,
						LastTransitionTime: time,
						Reason:             "degraded reason",
						Message:            "degraded message",
					},
					configv1.OperatorUpgradeable: {
						Type:               configv1.OperatorUpgradeable,
						Status:             configv1.ConditionUnknown,
						LastTransitionTime: time,
						Reason:             "",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newConditions(tt.args.cos, tt.args.time); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newConditions() = %v, want %v", got, tt.want)
			}
		})
	}
}
