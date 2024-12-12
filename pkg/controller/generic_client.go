package controller

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/cache"

	operatorapiv1 "github.com/openshift/api/operator/v1"
	applyoperatorv1 "github.com/openshift/client-go/operator/applyconfigurations/operator/v1"
	operatorclientv1 "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1"
	operatorinformers "github.com/openshift/client-go/operator/informers/externalversions"
	"github.com/openshift/library-go/pkg/apiserver/jsonpatch"
)

type genericClient struct {
	informers operatorinformers.SharedInformerFactory
	client    operatorclientv1.OperatorV1Interface
}

func (p *genericClient) Informer() cache.SharedIndexInformer {
	return p.informers.Operator().V1().InsightsOperators().Informer()
}

func (p *genericClient) CurrentStatus() (operatorapiv1.OperatorStatus, error) {
	instance, err := p.informers.Operator().V1().InsightsOperators().Lister().Get("cluster")
	if err != nil {
		return operatorapiv1.OperatorStatus{}, err
	}

	return instance.Status.OperatorStatus, nil
}

func (p *genericClient) GetOperatorState() (*operatorapiv1.OperatorSpec, *operatorapiv1.OperatorStatus, string, error) {
	instance, err := p.informers.Operator().V1().InsightsOperators().Lister().Get("cluster")
	if err != nil {
		return nil, nil, "", err
	}

	return &instance.Spec.OperatorSpec, &instance.Status.OperatorStatus, instance.ResourceVersion, nil
}

func (p *genericClient) GetObjectMeta() (*metav1.ObjectMeta, error) {
	resource, err := p.informers.Operator().V1().InsightsOperators().Lister().Get("cluster")
	if err != nil {
		return nil, err
	}
	return &resource.ObjectMeta, nil
}

func (p *genericClient) UpdateOperatorSpec(ctx context.Context, resourceVersion string,
	spec *operatorapiv1.OperatorSpec) (*operatorapiv1.OperatorSpec, string, error) {
	resource, err := p.informers.Operator().V1().InsightsOperators().Lister().Get("cluster")
	if err != nil {
		return nil, "", err
	}
	resourceCopy := resource.DeepCopy()
	resourceCopy.ResourceVersion = resourceVersion
	resourceCopy.Spec.OperatorSpec = *spec

	ret, err := p.client.InsightsOperators().Update(ctx, resourceCopy, metav1.UpdateOptions{})
	if err != nil {
		return nil, "", err
	}

	return &ret.Spec.OperatorSpec, ret.ResourceVersion, nil
}
func (p *genericClient) UpdateOperatorStatus(ctx context.Context, resourceVersion string,
	status *operatorapiv1.OperatorStatus) (*operatorapiv1.OperatorStatus, error) {
	resource, err := p.informers.Operator().V1().InsightsOperators().Lister().Get("cluster")
	if err != nil {
		return nil, err
	}
	resourceCopy := resource.DeepCopy()
	resourceCopy.ResourceVersion = resourceVersion
	resourceCopy.Status.OperatorStatus = *status

	ret, err := p.client.InsightsOperators().UpdateStatus(ctx, resourceCopy, metav1.UpdateOptions{})
	if err != nil {
		return nil, err
	}

	return &ret.Status.OperatorStatus, nil
}

func (p *genericClient) GetOperatorStateWithQuorum(_ context.Context) (spec *operatorapiv1.OperatorSpec,
	status *operatorapiv1.OperatorStatus, resourceVersion string, err error) {
	resource, err := p.informers.Operator().V1().InsightsOperators().Lister().Get("cluster")
	if err != nil {
		return nil, nil, "", err
	}

	return &resource.Spec.OperatorSpec, &resource.Status.OperatorStatus, resource.ResourceVersion, nil
}

func (p *genericClient) ApplyOperatorSpec(ctx context.Context,
	fieldManager string,
	applyConfiguration *applyoperatorv1.OperatorSpecApplyConfiguration) (err error) {
	desiredSpecApplyConf := &applyoperatorv1.InsightsOperatorApplyConfiguration{
		Spec: &applyoperatorv1.InsightsOperatorSpecApplyConfiguration{
			OperatorSpecApplyConfiguration: *applyConfiguration,
		},
	}
	_, err = p.client.InsightsOperators().Apply(ctx, desiredSpecApplyConf, metav1.ApplyOptions{
		FieldManager: fieldManager,
	})
	return err
}

func (p *genericClient) ApplyOperatorStatus(ctx context.Context,
	fieldManager string,
	applyConfiguration *applyoperatorv1.OperatorStatusApplyConfiguration) (err error) {
	desiredStattusApplyConf := &applyoperatorv1.InsightsOperatorApplyConfiguration{
		Status: &applyoperatorv1.InsightsOperatorStatusApplyConfiguration{
			OperatorStatusApplyConfiguration: *applyConfiguration,
		},
	}

	_, err = p.client.InsightsOperators().ApplyStatus(ctx, desiredStattusApplyConf, metav1.ApplyOptions{
		FieldManager: fieldManager,
	})
	return err
}

func (p *genericClient) PatchOperatorStatus(ctx context.Context,
	jsonPatch *jsonpatch.PatchSet) (err error) {
	jpData, err := jsonPatch.Marshal()
	if err != nil {
		return err
	}
	_, err = p.client.InsightsOperators().Patch(ctx, "cluster", types.JSONPatchType, jpData, metav1.PatchOptions{}, "/status")
	return err
}
