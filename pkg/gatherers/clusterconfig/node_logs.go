package clusterconfig

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/openshift/insights-operator/pkg/gatherers/common"

	corev1 "k8s.io/api/core/v1"

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
// Response see https://docs.openshift.com/container-platform/4.9/rest_api/node_apis/node-core-v1.html#apiv1nodesnameproxypath
//
// * Location in archive: config/nodes/logs/
// * See: docs/insights-archive-sample/config/nodes/logs
// * Id in config: node_logs
// * Since versions:
//   * 4.10+
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
	return nodeLogRecords(ctx, client.RESTClient(), nodes)
}

// nodeLogRecords generate the records and errors list
func nodeLogRecords(ctx context.Context, restClient rest.Interface, nodes *corev1.NodeList) ([]record.Record, []error) {
	var errs []error
	records := make([]record.Record, 0)

	bufferSize := recorder.MaxArchiveSize * logCompressionRatio / len(nodes.Items) / 2
	klog.V(2).Infof("Maximum buffer size: %v bytes", bufferSize)
	buf := bytes.NewBuffer(make([]byte, 0, bufferSize))

	for i := range nodes.Items {
		name := nodes.Items[i].Name
		uri := nodeLogResourceURI(restClient, name)
		req := requestNodeLog(restClient, uri, logNodeMaxTailLines, logNodeUnit)

		logString, err := nodeLogString(ctx, req, buf, bufferSize)
		if err != nil {
			klog.V(2).Infof("Error: %q", err)
			errs = append(errs, err)
		}

		records = append(records, record.Record{
			Name: fmt.Sprintf("config/node/logs/%s.log", name),
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
func nodeLogString(ctx context.Context, req *rest.Request, out *bytes.Buffer, size int) (string, error) {
	in, err := req.Stream(ctx)
	if err != nil {
		return "", err
	}
	defer in.Close()

	buf := bufio.NewReaderSize(in, size)
	head, err := buf.Peek(1024)
	if err != nil && err != io.EOF {
		return "", err
	}

	_, err = gzip.NewReader(bytes.NewBuffer(head))
	if err != nil {
		// not gzipped stream
		_, err = io.Copy(out, buf)
		if err != nil && err != io.EOF {
			return "", err
		}
	} else {
		r, err := gzip.NewReader(buf)
		if err != nil {
			return "", err
		}

		// nolint: gosec
		_, err = io.Copy(out, r)
		if err != nil {
			return "", err
		}
	}

	scanner := bufio.NewScanner(out)
	messagesToSearch := []string{
		"E\\d{4} [0-9]{1,2}:[0-9]{1,2}:[0-9]{1,2}", //  Errors from log
	}
	return common.FilterLogFromScanner(scanner, messagesToSearch, true, func(lines []string) []string {
		if len(lines) > logNodeMaxLines {
			return lines[len(lines)-logNodeMaxLines:]
		}
		return lines
	})
}
