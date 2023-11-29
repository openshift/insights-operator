package common

import (
	"bufio"
	"context"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/openshift/insights-operator/pkg/utils/marshal"

	"github.com/openshift/insights-operator/pkg/record"

	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/stretchr/testify/assert"
)

// nolint: lll, misspell
var testText = `Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.
Ullamcorper eget nulla facilisi etiam dignissim diam quis enim.
Rhoncus mattis rhoncus urna neque viverra.
Tempus urna et pharetra pharetra massa. Enim tortor at auctor urna nunc.
Id volutpat lacus laoreet non curabitur.
Feugiat pretium nibh ipsum consequat nisl vel.
Morbi tristique senectus et netus.
Tellus mauris a diam maecenas sed enim ut sem viverra.
Nunc scelerisque viverra mauris in aliquam.
Facilisis volutpat est velit egestas. Et netus et malesuada fames ac turpis egestas.
Sapien eget mi proin sed libero enim sed. Urna id volutpat lacus laoreet non.
Scelerisque eu ultrices vitae auctor.
Volutpat maecenas volutpat blandit aliquam etiam.
Sit amet nisl purus in mollis nunc sed id.
Tortor at auctor urna nunc id.
Purus in mollis nunc sed.
Enim ut tellus elementum sagittis vitae et leo.Quis viverra nibh cras pulvinar mattis nunc sed blandit libero.
Morbi tempus iaculis urna id volutpat lacus laoreet.
Pellentesque elit ullamcorper dignissim cras tincidunt lobortis.
Vitae proin sagittis nisl rhoncus.
Tortor condimentum lacinia quis vel eros donec ac odio tempor.`

func TestCollectLogsFromContainers(t *testing.T) {
	coreClient := kubefake.NewSimpleClientset().CoreV1()
	ctx := context.Background()

	if err := createPods(coreClient, ctx, "test-namespace", "test"); err != nil {
		t.Fatal(err)
	}
	if err := createPods(coreClient, ctx, "second-namespace", "new-pod"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name              string
		logResourceFilter *LogResourceFilter
		logMessageFilter  *LogMessagesFilter
		wantRecords       []record.Record
		wantErr           error
	}{
		{
			name: "substring search should exist",
			logResourceFilter: &LogResourceFilter{
				Namespaces: []string{"test-namespace"},
			},
			logMessageFilter: &LogMessagesFilter{
				MessagesToSearch: []string{"fake logs"},
				IsRegexSearch:    false,
			},
			wantRecords: []record.Record{
				{
					Name: "config/pod/test-namespace/logs/test/errors.log",
					Item: marshal.Raw{Str: "fake logs"},
				},
			},
			wantErr: nil,
		},
		{
			name: "substring search should not exist",
			logResourceFilter: &LogResourceFilter{
				Namespaces: []string{"test-namespace"},
			},
			logMessageFilter: &LogMessagesFilter{
				MessagesToSearch: []string{"The quick brown fox jumps over the lazy dog"},
				IsRegexSearch:    false,
			},
			wantRecords: nil,
			wantErr:     nil,
		},
		{
			name: "regex search should exist",
			logResourceFilter: &LogResourceFilter{
				Namespaces: []string{"test-namespace"},
			},
			logMessageFilter: &LogMessagesFilter{
				MessagesToSearch: []string{"f.*l"},
				IsRegexSearch:    true,
			},
			wantRecords: []record.Record{
				{
					Name: "config/pod/test-namespace/logs/test/errors.log",
					Item: marshal.Raw{Str: "fake logs"},
				},
			},
			wantErr: nil,
		},
		{
			name: "regex search should not exist",
			logResourceFilter: &LogResourceFilter{
				Namespaces: []string{"test-namespace"},
			},
			logMessageFilter: &LogMessagesFilter{
				MessagesToSearch: []string{"f.*l"},
				IsRegexSearch:    false,
			},
			wantRecords: nil,
			wantErr:     nil,
		},
		{
			name: "regex search should not exist",
			logResourceFilter: &LogResourceFilter{
				Namespaces: []string{"test-namespace"},
			},
			logMessageFilter: &LogMessagesFilter{
				MessagesToSearch: []string{"[0-9]99"},
				IsRegexSearch:    true,
			},
			wantRecords: nil,
			wantErr:     nil,
		},
		{
			name: "deprecated namespace still supported",
			logResourceFilter: &LogResourceFilter{
				Namespace: "test-namespace",
			},
			logMessageFilter: &LogMessagesFilter{
				MessagesToSearch: []string{"fake logs"},
				IsRegexSearch:    false,
			},
			wantRecords: []record.Record{
				{
					Name: "config/pod/test-namespace/logs/test/errors.log",
					Item: marshal.Raw{Str: "fake logs"},
				},
			},
			wantErr: nil,
		},
		{
			name: "support multiple namespaces",
			logResourceFilter: &LogResourceFilter{
				Namespaces: []string{"test-namespace", "second-namespace"},
			},
			logMessageFilter: &LogMessagesFilter{
				MessagesToSearch: []string{"fake logs"},
				IsRegexSearch:    false,
			},
			wantRecords: []record.Record{
				{
					Name: "config/pod/test-namespace/logs/test/errors.log",
					Item: marshal.Raw{Str: "fake logs"},
				},
				{
					Name: "config/pod/second-namespace/logs/new-pod/errors.log",
					Item: marshal.Raw{Str: "fake logs"},
				},
			},
			wantErr: nil,
		},
	}

	for _, testCase := range tests {
		tt := testCase
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			records, err := CollectLogsFromContainers(
				ctx,
				coreClient,
				tt.logResourceFilter,
				tt.logMessageFilter,
				nil,
			)

			assert.Equal(t, err, tt.wantErr)
			assert.Equal(t, records, tt.wantRecords)
		})
	}
}

func createPods(coreClient corev1client.CoreV1Interface, ctx context.Context, namespace, podName string) error {
	_, err := coreClient.Pods(namespace).Create(
		ctx,
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      podName,
				Namespace: namespace,
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: podName},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: podName},
				},
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		return err
	}

	return nil
}

func Test_FilterLogFromScanner(t *testing.T) {
	tests := []struct {
		name             string
		messagesToSearch []string
		messagesRegex    *regexp.Regexp
		expectedOutput   string
	}{
		{
			name:             "simple non-regex search",
			messagesToSearch: []string{"Pellentesque"},
			messagesRegex:    nil,
			expectedOutput:   "Pellentesque elit ullamcorper dignissim cras tincidunt lobortis.",
		},
		{
			name:             "non-regex search with empty messages",
			messagesToSearch: []string{},
			messagesRegex:    nil,
			expectedOutput:   testText,
		},
		{
			name:             "advanced non-regex search",
			messagesToSearch: []string{"Pellentesque", "scelerisque", "this is not there"},
			messagesRegex:    nil,
			// nolint lll
			expectedOutput: "Nunc scelerisque viverra mauris in aliquam.\nScelerisque eu ultrices vitae auctor.\nPellentesque elit ullamcorper dignissim cras tincidunt lobortis.",
		},
		{
			name:             "Regex search with empty messages",
			messagesToSearch: nil,
			messagesRegex:    nil,
			expectedOutput:   testText,
		},
		{
			name:             "advanced regex search",
			messagesToSearch: nil,
			messagesRegex:    regexp.MustCompile(strings.Join([]string{"Pellentesque", "scelerisque", "this is not there"}, "|")),
			expectedOutput:   "Nunc scelerisque viverra mauris in aliquam.\nPellentesque elit ullamcorper dignissim cras tincidunt lobortis.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(testText)
			var filter FilterLogOption
			if tt.messagesRegex != nil {
				filter = WithRegexFilter(tt.messagesRegex)
			} else {
				filter = WithSubstringFilter(tt.messagesToSearch)
			}
			result, err := FilterLogFromScanner(bufio.NewScanner(reader), filter, nil)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedOutput, result)
		})
	}
}

func Benchmark_FilterLogFromScanner(b *testing.B) {
	var m runtime.MemStats
	messagesToSearch := []string{"Pellentesque", "scelerisque", "this is not there"}
	messagesRegex := regexp.MustCompile(strings.Join(messagesToSearch, "|"))
	reader := strings.NewReader(testText)
	for i := 0; i <= b.N; i++ {
		// nolint errcheck
		FilterLogFromScanner(bufio.NewScanner(reader), WithRegexFilter(messagesRegex), nil)
		runtime.ReadMemStats(&m)
		b.Logf("Size of allocated heap objects: %d MB, Size of heap in use: %d MB", m.Alloc/1024/1024, m.HeapInuse/1024/1024)
	}
}
