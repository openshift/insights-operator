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

// BuildGatherImageStreamsOfNamespace creates a gathering closure which collects image streams from the provided namespace
// Params:
//   - namespace string - namespace from which to collect image streams
//
// API reference:
//   https://docs.openshift.com/container-platform/4.7/rest_api/image_apis/imagestream-image-openshift-io-v1.html#apisimage-openshift-iov1namespacesnamespaceimagestreams
//
// * Location in archive: conditional/namespaces/{namespace}/imagestreams/{name}
func (g *Gatherer) BuildGatherImageStreamsOfNamespace(gatherParams GatheringFunctionParams) (gatherers.GatheringClosure, error) {
	namespace, err := getStringFromMap(gatherParams, "namespace")
	if err != nil {
		return gatherers.GatheringClosure{}, err
	}

	return gatherers.GatheringClosure{
		Run: func(ctx context.Context) ([]record.Record, []error) {
			records, err := g.gatherImageStreamsOfNamespace(ctx, namespace)
			if err != nil {
				return records, []error{err}
			}
			return records, nil
		},
		CanFail: canConditionalGathererFail,
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

	for _, imageStream := range imageStreams.Items {
		imageStream = anonymizeImageStream(imageStream)

		records = append(records, record.Record{
			Name: fmt.Sprintf(
				"%v/namespaces/%v/imagestreams/%v",
				g.GetName(), imageStream.GetNamespace(), imageStream.GetName(),
			),
			Item: record.JSONMarshaller{Object: imageStream},
		})
	}

	return records, nil
}

func anonymizeImageStream(imageStream imagev1.ImageStream) imagev1.ImageStream {
	imageStream.Spec.DockerImageRepository = anonymize.AnonymizeString(imageStream.Spec.DockerImageRepository)

	specTags := imageStream.Spec.Tags
	for i, _ := range specTags {
		tag := &specTags[i]
		tag.From.Name = anonymize.AnonymizeString(tag.From.Name)
	}

	imageStream.Status.DockerImageRepository = anonymize.AnonymizeString(imageStream.Status.DockerImageRepository)
	imageStream.Status.PublicDockerImageRepository = anonymize.AnonymizeString(imageStream.Status.PublicDockerImageRepository)

	statusTags := imageStream.Status.Tags
	for tagIndex, _ := range statusTags {
		tag := &statusTags[tagIndex]
		for itemIndex, _ := range tag.Items {
			item := &tag.Items[itemIndex]
			item.DockerImageReference = anonymize.AnonymizeString(item.DockerImageReference)
		}
	}

	return imageStream
}
