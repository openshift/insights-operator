// nolint: lll
package conditional

import (
	"context"
	"fmt"

	imagev1 "github.com/openshift/api/image/v1"
	imageclient "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift/insights-operator/pkg/gatherers"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils/anonymize"
)

// BuildGatherImageStreamsOfNamespace Closure which collects image streams from the provided namespace
//
// Params is of type GatherImageStreamsOfNamespaceParams:
// - namespace string - namespace from which to collect image streams
//
// ### API Reference
// - https://docs.openshift.com/container-platform/4.7/rest_api/image_apis/imagestream-image-openshift-io-v1.html#apisimage-openshift-iov1namespacesnamespaceimagestreams
//
// ### Sample data
// - docs/insights-archive-sample/conditional/namespaces/openshift-cluster-samples-operator/imagestreams/example.json
//
// ### Location in archive
// | Version   | Path														|
// | --------- | --------------------------------------------------------	|
// | >= 4.9.0  | conditional/namespaces/{namespace}/imagestreams/{name}.json |
//
// ### Config ID
// `conditional/image_streams_of_namespace`
//
// ### Released version
// - 4.9.0
//
// ### Backported versions
// None
//
// ### Changes
// None
func (g *Gatherer) BuildGatherImageStreamsOfNamespace(paramsInterface interface{}) (gatherers.GatheringClosure, error) {
	params, ok := paramsInterface.(GatherImageStreamsOfNamespaceParams)
	if !ok {
		return gatherers.GatheringClosure{}, fmt.Errorf(
			"unexpected type in paramsInterface, expected %T, got %T",
			GatherImageStreamsOfNamespaceParams{}, paramsInterface,
		)
	}

	return gatherers.GatheringClosure{
		Run: func(ctx context.Context) ([]record.Record, []error) {
			records, err := g.gatherImageStreamsOfNamespace(ctx, params.Namespace)
			if err != nil {
				return records, []error{err}
			}
			return records, nil
		},
	}, nil
}

func (g *Gatherer) gatherImageStreamsOfNamespace(ctx context.Context, namespace string) ([]record.Record, error) {
	imageClient, err := imageclient.NewForConfig(g.imageKubeConfig)
	if err != nil {
		return nil, err
	}

	imageStreams, err := imageClient.ImageStreams(namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var records []record.Record

	for i, imageStream := range imageStreams.Items { //nolint:gocritic
		imageStream = anonymizeImageStream(imageStream)

		records = append(records, record.Record{
			Name: fmt.Sprintf(
				"%v/namespaces/%v/imagestreams/%v",
				g.GetName(), imageStream.GetNamespace(), imageStream.GetName(),
			),
			Item: record.ResourceMarshaller{Resource: &imageStreams.Items[i]},
		})
	}

	return records, nil
}

func anonymizeImageStream(imageStream imagev1.ImageStream) imagev1.ImageStream { //nolint:gocritic
	imageStream.Spec.DockerImageRepository = anonymize.String(imageStream.Spec.DockerImageRepository)

	specTags := imageStream.Spec.Tags
	for i := range specTags {
		tag := &specTags[i]
		tag.From.Name = anonymize.String(tag.From.Name)
	}

	imageStream.Status.DockerImageRepository = anonymize.String(imageStream.Status.DockerImageRepository)
	imageStream.Status.PublicDockerImageRepository = anonymize.String(imageStream.Status.PublicDockerImageRepository)

	statusTags := imageStream.Status.Tags
	for tagIndex := range statusTags {
		tag := &statusTags[tagIndex]
		for itemIndex := range tag.Items {
			item := &tag.Items[itemIndex]
			item.DockerImageReference = anonymize.String(item.DockerImageReference)
		}
	}

	return imageStream
}
