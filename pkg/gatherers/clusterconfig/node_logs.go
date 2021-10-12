package clusterconfig

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/openshift/insights-operator/pkg/recorder"

	"k8s.io/klog/v2"

	"k8s.io/client-go/rest"

	"github.com/openshift/insights-operator/pkg/utils/marshal"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/kubernetes"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/openshift/insights-operator/pkg/record"
)

// GatherNodeLogs fetches the node logs from journal unit
//
// Response see https://docs.openshift.com/container-platform/4.8/rest_api/node_apis/node-core-v1.html#apiv1nodesnameproxypath
//
// * Location in archive: config/nodes/logs/
// * See: docs/insights-archive-sample/config/nodes/logs
// * Id in config: node_logs
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
		uri := nodeLogResourceURI(restClient, nodes.Items[i].Name)
		req := requestNodeLog(restClient, uri, logNodeMaxTailLines, logNodeUnit)
		logString, err := nodeLogString(req, buf)
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

// nodeLogResourceURI creates the resource path URI to be fetched
func nodeLogResourceURI(client rest.Interface, name string) string {
	return client.Get().
		Name(name).
		Resource("nodes").SubResource("proxy", "logs").
		Suffix("journal").URL().Path
}

// requestNodeLog creates the request to the API to retrieve the resource stream
func requestNodeLog(client rest.Interface, uri string, tail int, unit string) *rest.Request {
	return client.Get().RequestURI(uri).
		SetHeader("Accept", "text/plain, */*").
		SetHeader("Accept-Encoding", "gzip").
		Param("tail", strconv.Itoa(tail)).
		Param("unit", unit)
}

// nodeLogString retrieve the data from the stream, decompress it (if necessary) and return the string
func nodeLogString(req *rest.Request, buf *bytes.Buffer) (string, error) {
	buf.Reset()

	in, err := req.Stream(context.TODO())
	if err != nil {
		return "", err
	}
	defer in.Close()

	gz, err := gzip.NewReader(in)
	if err != nil {
		// not gzipped stream
		_, err = io.Copy(buf, in)
		if err != nil && err != io.EOF {
			return "", err
		}

		return buf.String(), nil
	}

	// nolint: gosec
	_, err = io.Copy(buf, gz)
	if err != nil && err != io.EOF {
		return "", err
	}
	return buf.String(), nil
}
