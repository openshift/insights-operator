package operatorconfig

import (
	"context"
	"fmt"
	"strings"

	operatorv1 "github.com/openshift/api/operator/v1"
	operatorv1client "github.com/openshift/client-go/operator/clientset/versioned/typed/operator/v1"
	"github.com/openshift/insights-operator/pkg/record"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

var (
	operatorSerializer serializer.CodecFactory
	operatorScheme     = runtime.NewScheme()
)

func init() {
	utilruntime.Must(operatorv1.AddToScheme(operatorScheme))
	operatorSerializer = serializer.NewCodecFactory(operatorScheme)
}

// Gatherer collects operators config data
type Gatherer struct {
	ctx                context.Context
	consoleClient      operatorv1client.ConsoleInterface
	openshiftAPIClient operatorv1client.OpenShiftAPIServerInterface
}

// New creates new Gatherer
func New(client *operatorv1client.OperatorV1Client) *Gatherer {

	return &Gatherer{
		consoleClient:      client.Consoles(),
		openshiftAPIClient: client.OpenShiftAPIServers(),
	}

}

// Gather is hosting and calling all the recording functions
func (i *Gatherer) Gather(ctx context.Context, recorder record.Interface) error {
	i.ctx = ctx
	return record.Collect(ctx, recorder, GatherConsoles(i), GatherOpenshiftAPIServers(i))
}

// GatherConsoles - TODO
func GatherConsoles(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		cList, err := i.consoleClient.List(i.ctx, metav1.ListOptions{})
		if err != nil {
			return nil, []error{err}
		}
		records := []record.Record{}
		for _, c := range cList.Items {
			records = append(records, record.Record{
				Name: fmt.Sprintf("operatorconfig/%s-%s", strings.ToLower(c.Kind), c.Name),
				Item: ConsolesAnonymizer{&c},
			})
		}
		return records, nil
	}
}

// GatherOpenshiftAPIServers - TODO
func GatherOpenshiftAPIServers(i *Gatherer) func() ([]record.Record, []error) {
	return func() ([]record.Record, []error) {
		oa, err := i.openshiftAPIClient.List(i.ctx, metav1.ListOptions{})
		if err != nil {
			return nil, []error{err}
		}
		records := []record.Record{}
		for _, oa := range oa.Items {
			records = append(records, record.Record{
				Name: fmt.Sprintf("operatorconfig/%s-%s", strings.ToLower(oa.Kind), oa.Name),
				Item: OpenShiftAPIServerAnonymizer{&oa},
			})
		}
		return records, nil
	}
}

// ConsolesAnonymizer implements Console serialization wiht anonymization
type ConsolesAnonymizer struct{ *operatorv1.Console }

// Marshal implements Console serialization
func (a ConsolesAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(operatorSerializer.LegacyCodec(operatorv1.SchemeGroupVersion), a.Console)
}

// GetExtension returns extension for Console object
func (a ConsolesAnonymizer) GetExtension() string {
	return "json"
}

// OpenShiftAPIServerAnonymizer implements OpenShiftAPIServer serialization without anonymization
type OpenShiftAPIServerAnonymizer struct{ *operatorv1.OpenShiftAPIServer }

// Marshal implements OpenShiftAPIServer serialization
func (a OpenShiftAPIServerAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(operatorSerializer.LegacyCodec(operatorv1.SchemeGroupVersion), a.OpenShiftAPIServer)
}

// GetExtension returns extension for OpenShiftAPIServer object
func (a OpenShiftAPIServerAnonymizer) GetExtension() string {
	return "json"
}
