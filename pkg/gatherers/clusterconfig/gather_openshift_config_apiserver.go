package clusterconfig

import (
	"context"

	configv1client "github.com/openshift/client-go/config/clientset/versioned/typed/config/v1"
	"github.com/openshift/insights-operator/pkg/record"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TODO - Doc
func (g *Gatherer) GatherOpenshiftConfigAPIServer(ctx context.Context) ([]record.Record, []error) {
	configClient, err := configv1client.NewForConfig(g.gatherKubeConfig)
	if err != nil {
		return nil, []error{err}
	}

	return configAPIServer{}.gather(ctx, configClient)
}

type configAPIServer struct{}

func (cas configAPIServer) gather(ctx context.Context, client *configv1client.ConfigV1Client) ([]record.Record, []error) {
	const APIServerName = "cluster"
	const Filename = "config/apiserver"

	server, err := client.APIServers().Get(ctx, APIServerName, v1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, []error{err}
	}

	return []record.Record{{Name: Filename, Item: record.ResourceMarshaller{Resource: server}}}, nil
}
