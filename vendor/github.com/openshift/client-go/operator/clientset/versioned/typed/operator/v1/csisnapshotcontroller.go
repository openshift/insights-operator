// Code generated by client-gen. DO NOT EDIT.

package v1

import (
	context "context"

	operatorv1 "github.com/openshift/api/operator/v1"
	applyconfigurationsoperatorv1 "github.com/openshift/client-go/operator/applyconfigurations/operator/v1"
	scheme "github.com/openshift/client-go/operator/clientset/versioned/scheme"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	gentype "k8s.io/client-go/gentype"
)

// CSISnapshotControllersGetter has a method to return a CSISnapshotControllerInterface.
// A group's client should implement this interface.
type CSISnapshotControllersGetter interface {
	CSISnapshotControllers() CSISnapshotControllerInterface
}

// CSISnapshotControllerInterface has methods to work with CSISnapshotController resources.
type CSISnapshotControllerInterface interface {
	Create(ctx context.Context, cSISnapshotController *operatorv1.CSISnapshotController, opts metav1.CreateOptions) (*operatorv1.CSISnapshotController, error)
	Update(ctx context.Context, cSISnapshotController *operatorv1.CSISnapshotController, opts metav1.UpdateOptions) (*operatorv1.CSISnapshotController, error)
	// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
	UpdateStatus(ctx context.Context, cSISnapshotController *operatorv1.CSISnapshotController, opts metav1.UpdateOptions) (*operatorv1.CSISnapshotController, error)
	Delete(ctx context.Context, name string, opts metav1.DeleteOptions) error
	DeleteCollection(ctx context.Context, opts metav1.DeleteOptions, listOpts metav1.ListOptions) error
	Get(ctx context.Context, name string, opts metav1.GetOptions) (*operatorv1.CSISnapshotController, error)
	List(ctx context.Context, opts metav1.ListOptions) (*operatorv1.CSISnapshotControllerList, error)
	Watch(ctx context.Context, opts metav1.ListOptions) (watch.Interface, error)
	Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts metav1.PatchOptions, subresources ...string) (result *operatorv1.CSISnapshotController, err error)
	Apply(ctx context.Context, cSISnapshotController *applyconfigurationsoperatorv1.CSISnapshotControllerApplyConfiguration, opts metav1.ApplyOptions) (result *operatorv1.CSISnapshotController, err error)
	// Add a +genclient:noStatus comment above the type to avoid generating ApplyStatus().
	ApplyStatus(ctx context.Context, cSISnapshotController *applyconfigurationsoperatorv1.CSISnapshotControllerApplyConfiguration, opts metav1.ApplyOptions) (result *operatorv1.CSISnapshotController, err error)
	CSISnapshotControllerExpansion
}

// cSISnapshotControllers implements CSISnapshotControllerInterface
type cSISnapshotControllers struct {
	*gentype.ClientWithListAndApply[*operatorv1.CSISnapshotController, *operatorv1.CSISnapshotControllerList, *applyconfigurationsoperatorv1.CSISnapshotControllerApplyConfiguration]
}

// newCSISnapshotControllers returns a CSISnapshotControllers
func newCSISnapshotControllers(c *OperatorV1Client) *cSISnapshotControllers {
	return &cSISnapshotControllers{
		gentype.NewClientWithListAndApply[*operatorv1.CSISnapshotController, *operatorv1.CSISnapshotControllerList, *applyconfigurationsoperatorv1.CSISnapshotControllerApplyConfiguration](
			"csisnapshotcontrollers",
			c.RESTClient(),
			scheme.ParameterCodec,
			"",
			func() *operatorv1.CSISnapshotController { return &operatorv1.CSISnapshotController{} },
			func() *operatorv1.CSISnapshotControllerList { return &operatorv1.CSISnapshotControllerList{} },
		),
	}
}
