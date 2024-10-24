package clusterconfig

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

func TestGatherClusterRoles(t *testing.T) {
	tests := []struct {
		name                    string
		clusterRoleNames        []string
		testClusterRoles        []v1.ClusterRole
		expectedErrors          []error
		expectedLenghtOfRecords int
	}{
		{
			name:             "no existing clusterroles",
			testClusterRoles: []v1.ClusterRole{},
		},
		{
			name:             "no existing clusterroles but some are requested",
			clusterRoleNames: []string{"role1", "role2"},
			testClusterRoles: []v1.ClusterRole{},
			expectedErrors: []error{
				&errors.StatusError{
					ErrStatus: metav1.Status{
						Status: "Failure",
						Reason: metav1.StatusReasonNotFound,
						Details: &metav1.StatusDetails{
							Name:  "role1",
							Group: "rbac.authorization.k8s.io",
							Kind:  "clusterroles",
						},
						Code:    404,
						Message: "clusterroles.rbac.authorization.k8s.io \"role1\" not found",
					},
				},
				&errors.StatusError{
					ErrStatus: metav1.Status{
						Reason: metav1.StatusReasonNotFound,
						Status: "Failure",
						Details: &metav1.StatusDetails{
							Name:  "role2",
							Group: "rbac.authorization.k8s.io",
							Kind:  "clusterroles",
						},
						Code:    404,
						Message: "clusterroles.rbac.authorization.k8s.io \"role2\" not found",
					},
				},
			},
		},
		{
			name:             "one existing clusterrole gathered",
			clusterRoleNames: []string{"role1", "role2"},
			testClusterRoles: []v1.ClusterRole{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "role1",
					},
					Rules: []v1.PolicyRule{
						{Verbs: []string{"get", "list"}},
					},
				},
			},
			expectedLenghtOfRecords: 1,
			expectedErrors: []error{
				&errors.StatusError{
					ErrStatus: metav1.Status{
						Status: "Failure",
						Reason: metav1.StatusReasonNotFound,
						Details: &metav1.StatusDetails{
							Name:  "role2",
							Group: "rbac.authorization.k8s.io",
							Kind:  "clusterroles",
						},
						Code:    404,
						Message: "clusterroles.rbac.authorization.k8s.io \"role2\" not found",
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cli := kubefake.NewSimpleClientset()
			for _, clusterRole := range tt.testClusterRoles {
				err := cli.Tracker().Add(&clusterRole)
				assert.NoError(t, err)
			}
			records, errs := gatherClusterRoles(context.Background(), cli.RbacV1(), tt.clusterRoleNames)
			assert.Len(t, records, tt.expectedLenghtOfRecords)
			assert.Equal(t, tt.expectedErrors, errs)
		})
	}
}
