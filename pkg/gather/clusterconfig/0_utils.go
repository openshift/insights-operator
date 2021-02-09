package clusterconfig

import (
	"fmt"
	"bytes"
	"context"
	"reflect"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/json"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	kubescheme "k8s.io/client-go/kubernetes/scheme"

	configv1 "github.com/openshift/api/config/v1"
	registryv1 "github.com/openshift/api/imageregistry/v1"
	networkv1 "github.com/openshift/api/network/v1"
	openshiftscheme "github.com/openshift/client-go/config/clientset/versioned/scheme"
	appsv1 "k8s.io/api/apps/v1"
	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
)

const (
	maxNamespacesLimit = 1000
)

var (
	openshiftSerializer = openshiftscheme.Codecs.LegacyCodec(configv1.SchemeGroupVersion)
	kubeSerializer      = kubescheme.Codecs.LegacyCodec(corev1.SchemeGroupVersion)
	appsV1Serializer    = kubescheme.Codecs.LegacyCodec(appsv1.SchemeGroupVersion)
	// maxEventTimeInterval represents the "only keep events that are maximum 1h old"
	// TODO: make this dynamic like the reporting window based on configured interval
	maxEventTimeInterval = 1 * time.Hour

	registrySerializer serializer.CodecFactory
	networkSerializer  serializer.CodecFactory
	registryScheme     = runtime.NewScheme()
	networkScheme      = runtime.NewScheme()

	// logTailLines sets maximum number of lines to fetch from pod logs
	logTailLines = int64(100)

	imageHostRegex = regexp.MustCompile(`(^|\.)(openshift\.org|registry\.redhat\.io|registry\.access\.redhat\.com)$`)

	// lineSep is the line separator used by the alerts metric
	lineSep = []byte{'\n'}

	defaultNamespaces = []string{"default", "kube-system", "kube-public"}
)

func init() {
	utilruntime.Must(registryv1.AddToScheme(registryScheme))
	utilruntime.Must(networkv1.AddToScheme(networkScheme))
	networkSerializer = serializer.NewCodecFactory(networkScheme)
	registrySerializer = serializer.NewCodecFactory(registryScheme)
}

func failEarly(fns ...func() error) error {
	for _, f := range fns {
		err := f()
		if err != nil {
			return err
		}
	}
	return nil
}

func parseJSONQuery(j map[string]interface{}, jq string, o interface{}) error {
	for _, k := range strings.Split(jq, ".") {
		// optional field
		opt := false
		sz := len(k)
		if sz > 0 && k[sz-1] == '?' {
			opt = true
			k = k[:sz-1]
		}

		if uv, ok := j[k]; ok {
			if v, ok := uv.(map[string]interface{}); ok {
				j = v
				continue
			}
			// ValueOf to enter reflect-land
			dstPtrValue := reflect.ValueOf(o)
			dstValue := reflect.Indirect(dstPtrValue)
			dstValue.Set(reflect.ValueOf(uv))

			return nil
		}
		if opt {
			return nil
		}
		// otherwise key was not found
		// keys are case sensitive, because maps are
		for ki := range j {
			if strings.ToLower(k) == strings.ToLower(ki) {
				return fmt.Errorf("key %s wasn't found, but %s was ", k, ki)
			}
		}
		return fmt.Errorf("key %s wasn't found in %v ", k, j)
	}
	return fmt.Errorf("query didn't match the structure")
}

func getAllNamespaces(ctx context.Context, coreClient corev1client.CoreV1Interface) (*corev1.NamespaceList, context.Context, error) {
	ns, ok := ctx.Value(contextKeyAllNamespaces).(*corev1.NamespaceList)
	if ok {
		return ns, ctx, nil
	}
	ns, err := coreClient.Namespaces().List(ctx, metav1.ListOptions{Limit: maxNamespacesLimit})
	if err != nil {
		return nil, ctx, err
	}

	ctx = context.WithValue(ctx, contextKeyAllNamespaces, ns)

	return ns, ctx, nil
}

type contextKey string

const contextKeyAllNamespaces contextKey = "allnamespaces"

// RawByte is skipping Marshalling from byte slice
type RawByte []byte

// Marshal just returns bytes
func (r RawByte) Marshal(_ context.Context) ([]byte, error) {
	return r, nil
}

// GetExtension returns extension for "id" file - none
func (r RawByte) GetExtension() string {
	return ""
}

// Raw is another simplification of marshalling from string
type Raw struct{ string }

// Marshal returns raw bytes
func (r Raw) Marshal(_ context.Context) ([]byte, error) {
	return []byte(r.string), nil
}

// GetExtension returns extension for raw marshaller
func (r Raw) GetExtension() string {
	return ""
}

// Anonymizer returns serialized runtime.Object without change
type Anonymizer struct{ runtime.Object }

// Marshal serializes with OpenShift client-go openshiftSerializer
func (a Anonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(openshiftSerializer, a.Object)
}

// GetExtension returns extension for anonymized openshift objects
func (a Anonymizer) GetExtension() string {
	return "json"
}

// CompactedEvent holds one Namespace Event
type CompactedEvent struct {
	Namespace     string    `json:"namespace"`
	LastTimestamp time.Time `json:"lastTimestamp"`
	Reason        string    `json:"reason"`
	Message       string    `json:"message"`
}

// CompactedEventList is collection of events
type CompactedEventList struct {
	Items []CompactedEvent `json:"items"`
}

// EventAnonymizer implements serializaion of Events with anonymization
type EventAnonymizer struct{ *CompactedEventList }

// Marshal serializes Events with anonymization
func (a EventAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return json.Marshal(a.CompactedEventList)
}

// GetExtension returns extension for anonymized event objects
func (a EventAnonymizer) GetExtension() string {
	return "json"
}

func anonymizeURLCSV(s string) string {
	strs := strings.Split(s, ",")
	outSlice := anonymizeURLSlice(strs)
	return strings.Join(outSlice, ",")
}

func anonymizeURLSlice(in []string) []string {
	outSlice := []string{}
	for _, str := range in {
		outSlice = append(outSlice, anonymizeURL(str))
	}
	return outSlice
}

var reURL = regexp.MustCompile(`[^\.\-/\:]`)

func anonymizeURL(s string) string { return reURL.ReplaceAllString(s, "x") }

func anonymizeString(s string) string {
	return strings.Repeat("x", len(s))
}

func anonymizeSliceOfStrings(slice []string) []string {
	anonymizedSlice := make([]string, len(slice), len(slice))
	for i, s := range slice {
		anonymizedSlice[i] = anonymizeString(s)
	}
	return anonymizedSlice
}

// PodAnonymizer implements serialization with anonymization for a Pod
type PodAnonymizer struct{ *corev1.Pod }

// Marshal implements serialization of a Pod with anonymization
func (a PodAnonymizer) Marshal(_ context.Context) ([]byte, error) {
	return runtime.Encode(kubeSerializer, anonymizePod(a.Pod))
}

// GetExtension returns extension for anonymized pod objects
func (a PodAnonymizer) GetExtension() string {
	return "json"
}

func anonymizePod(pod *corev1.Pod) *corev1.Pod {
	// pods gathered from openshift namespaces and cluster operators are expected to be under our control and contain
	// no sensitive information
	return pod
}

func isHealthyPod(pod *corev1.Pod, now time.Time) bool {
	// pending pods may be unable to schedule or start due to failures, and the info they provide in status is important
	// for identifying why scheduling has not happened
	if pod.Status.Phase == corev1.PodPending {
		if now.Sub(pod.CreationTimestamp.Time) > 2*time.Minute {
			return false
		}
	}
	// pods that have containers that have terminated with non-zero exit codes are considered failure
	for _, status := range pod.Status.InitContainerStatuses {
		if status.LastTerminationState.Terminated != nil && status.LastTerminationState.Terminated.ExitCode != 0 {
			return false
		}
		if status.State.Terminated != nil && status.State.Terminated.ExitCode != 0 {
			return false
		}
		if status.RestartCount > 0 {
			return false
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		if status.LastTerminationState.Terminated != nil && status.LastTerminationState.Terminated.ExitCode != 0 {
			return false
		}
		if status.State.Terminated != nil && status.State.Terminated.ExitCode != 0 {
			return false
		}
		if status.RestartCount > 0 {
			return false
		}
	}
	return true
}

// MinimalNodeInfo contains the most essential information about a node
type MinimalNodeInfo struct {
	ProviderID string `json:"providerID"`
	Image      string `json:"image"`
}

func forceParseURLHost(rawurl string) (string, error) {
	// If the scheme isn't specified, the URL will not be parsed nicely and everything will end up in the "path"
	if !strings.Contains(rawurl, "://") {
		return forceParseURLHost("https://" + rawurl)
	}

	parsedURL, err := url.Parse(rawurl)
	if err != nil {
		return "", err
	}
	return parsedURL.Host, nil
}

func isContainerInCrashloop(status *corev1.ContainerStatus) bool {
	return status.RestartCount > 0 && ((status.LastTerminationState.Terminated != nil && status.LastTerminationState.Terminated.ExitCode != 0) || status.LastTerminationState.Waiting != nil)
}

func hasContainerInCrashloop(pod *corev1.Pod) bool {
	for _, status := range pod.Status.InitContainerStatuses {
		if isContainerInCrashloop(&status) {
			return true
		}
	}
	for _, status := range pod.Status.ContainerStatuses {
		if isContainerInCrashloop(&status) {
			return true
		}
	}
	return false
}

// NewLineLimitReader returns a Reader that reads from `r` but stops with EOF after `n` lines.
func NewLineLimitReader(r io.Reader, n int) *LineLimitedReader { return &LineLimitedReader{r, n, 0} }

// A LineLimitedReader reads from R but limits the amount of
// data returned to just N lines. Each call to Read
// updates N to reflect the new amount remaining.
// Read returns EOF when N <= 0 or when the underlying R returns EOF.
type LineLimitedReader struct {
	reader        io.Reader // underlying reader
	maxLinesLimit int       // max lines remaining
	// totalLinesRead is the total number of line separators already read by the underlying reader.
	totalLinesRead int
}

func (l *LineLimitedReader) Read(p []byte) (int, error) {
	if l.maxLinesLimit <= 0 {
		return 0, io.EOF
	}

	rc, err := l.reader.Read(p)
	l.totalLinesRead += bytes.Count(p[:rc], lineSep)

	lc := 0
	for {
		lineSepIdx := bytes.Index(p[lc:rc], lineSep)
		if lineSepIdx == -1 {
			return rc, err
		}
		if l.maxLinesLimit <= 0 {
			return lc, io.EOF
		}
		l.maxLinesLimit--
		lc += lineSepIdx + 1 // skip past the EOF
	}
}

// GetTotalLinesRead return the total number of line separators already read by the underlying reader.
// This includes lines that have been truncated by the `Read` calls after exceeding the line limit.
func (l *LineLimitedReader) GetTotalLinesRead() int { return l.totalLinesRead }

// countLines reads the remainder of the reader and counts the number of lines.
//
// Inspired by: https://stackoverflow.com/a/24563853/
func countLines(r io.Reader) (int, error) {
	buf := make([]byte, 0x8000)
	// Original implementation started from 0, but a file with no line separator
	// still contains a single line, so I would say that was an off-by-1 error.
	lineCount := 1
	for {
		c, err := r.Read(buf)
		lineCount += bytes.Count(buf[:c], lineSep)
		if err != nil {
			return lineCount, err
		}
	}
}
