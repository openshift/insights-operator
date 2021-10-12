package clusterconfig

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/openshift/insights-operator/pkg/recorder"

	"k8s.io/klog/v2"

	"k8s.io/client-go/rest"

	"github.com/openshift/insights-operator/pkg/utils/marshal"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

func (g *Gatherer) GatherNodeLogs(ctx context.Context) ([]record.Record, []error) {
	clientSet, err := kubernetes.NewForConfig(g.gatherProtoKubeConfig)
	if err != nil {
		return nil, []error{err}
	}
	return gatherNodeLogs(ctx, clientSet.CoreV1())
}

func gatherNodeLogs(ctx context.Context, client corev1client.CoreV1Interface) ([]record.Record, []error) {
	nodes, err := client.Nodes().List(ctx, metav1.ListOptions{LabelSelector: "node-role.kubernetes.io/master"})
	if err != nil {
		return nil, []error{err}
	}

	var errs []error
	records := make([]record.Record, 0)

	bufferSize := int64(recorder.MaxArchiveSize * logCompressionRatio / len(nodes.Items) / 2)
	klog.V(2).Infof("Maximum buffer size: %v bytes", bufferSize)
	buf := bytes.NewBuffer(make([]byte, 0, bufferSize))

	restClient := client.RESTClient()

	for i := range nodes.Items {
		uri := getNodeResourceURI(restClient, nodes.Items[i].Name)
		logString, err := getNodeLogString(restClient, uri, logMaxTailLines, buf)
		if err != nil {
			klog.V(2).Infof("Error: %q", err)
			errs = append(errs, err)
		}

		records = append(records, record.Record{
			Name: fmt.Sprintf("config/node/logs/%s.log", nodes.Items[i].Name),
			Item: marshal.Raw{Str: logString},
		})
	}

	return records, errs
}

func getNodeResourceURI(client rest.Interface, name string) string {
	uri := client.Get().
		Name(name).
		Resource("nodes").SubResource("proxy", "logs").
		Suffix("journal").URL().Path

	if strings.HasSuffix("journal", "/") {
		uri += "/"
	}
	return uri
}

func getNodeLogString(client rest.Interface, uri string, tail int64, buf *bytes.Buffer) (string, error) {
	buf.Reset()

	req := client.Get().RequestURI(uri).
		SetHeader("Accept", "text/plain, */*").
		SetHeader("Accept-Encoding", "gzip").
		Param("tail", strconv.Itoa(int(tail))) // It will always be 2 times the given value

	in, err := req.Stream(context.TODO())
	if err != nil {
		return "", err
	}
	defer in.Close()

	gz, err := gzip.NewReader(in)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	_, err = io.Copy(buf, gz)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}
