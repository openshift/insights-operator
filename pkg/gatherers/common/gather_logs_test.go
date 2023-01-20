package common

import (
	"bufio"
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"

	"github.com/openshift/insights-operator/pkg/utils/marshal"
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

func testGatherLogs(t *testing.T, regexSearch bool, stringToSearch string, shouldExist bool) {
	const testPodName = "test"
	const testLogFileName = "errors"

	coreClient := kubefake.NewSimpleClientset().CoreV1()
	ctx := context.Background()

	_, err := coreClient.Pods(testPodName).Create(
		ctx,
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testPodName,
				Namespace: testPodName,
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{
					{Name: testPodName},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{Name: testPodName},
				},
			},
		},
		metav1.CreateOptions{},
	)
	if err != nil {
		t.Fatal(err)
	}

	records, err := CollectLogsFromContainers(
		ctx,
		coreClient,
		&LogResourceFilter{
			Namespace: testPodName,
		},
		&LogMessagesFilter{
			MessagesToSearch: []string{
				stringToSearch,
			},
			IsRegexSearch: regexSearch,
			SinceSeconds:  86400,     // last day
			LimitBytes:    1024 * 64, // maximum 64 kb of logs
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !shouldExist {
		assert.Len(t, records, 0)
		return
	}

	assert.Len(t, records, 1)
	assert.Equal(
		t,
		fmt.Sprintf("config/pod/%s/logs/%s/%s.log", testPodName, testPodName, testLogFileName),
		records[0].Name,
	)
	if regexSearch {
		assert.Regexp(t, stringToSearch, records[0].Item)
	} else {
		assert.Equal(t, marshal.Raw{Str: stringToSearch}, records[0].Item)
	}
}

func Test_GatherLogs(t *testing.T) {
	t.Run("SubstringSearch_ShouldExist", func(t *testing.T) {
		testGatherLogs(t, false, "fake logs", true)
	})
	t.Run("SubstringSearch_ShouldNotExist", func(t *testing.T) {
		testGatherLogs(t, false, "The quick brown fox jumps over the lazy dog", false)
	})
	t.Run("SubstringSearch_ShouldNotExist", func(t *testing.T) {
		testGatherLogs(t, false, "f.*l", false)
	})

	t.Run("RegexSearch_ShouldExist", func(t *testing.T) {
		testGatherLogs(t, true, "f.*l", true)
	})
	t.Run("RegexSearch_ShouldNotExist", func(t *testing.T) {
		testGatherLogs(t, true, "[0-9]99", false)
	})
}

func Test_FilterLogFromScanner(t *testing.T) {
	tests := []struct {
		name             string
		messagesToSearch []string
		isRegexSearch    bool
		expectedOutput   string
	}{
		{
			name:             "simple non-regex search",
			messagesToSearch: []string{"Pellentesque"},
			isRegexSearch:    false,
			expectedOutput:   "Pellentesque elit ullamcorper dignissim cras tincidunt lobortis.",
		},
		{
			name:             "non-regex search with empty messages",
			messagesToSearch: []string{},
			isRegexSearch:    false,
			expectedOutput:   testText,
		},
		{
			name:             "advanced non-regex search",
			messagesToSearch: []string{"Pellentesque", "scelerisque", "this is not there"},
			isRegexSearch:    false,
			// nolint lll
			expectedOutput: "Nunc scelerisque viverra mauris in aliquam.\nScelerisque eu ultrices vitae auctor.\nPellentesque elit ullamcorper dignissim cras tincidunt lobortis.",
		},
		{
			name:             "Regex search with empty messages",
			messagesToSearch: []string{},
			isRegexSearch:    true,
			expectedOutput:   testText,
		},
		{
			name:             "advanced regex search",
			messagesToSearch: []string{"Pellentesque", "scelerisque", "this is not there"},
			isRegexSearch:    true,
			expectedOutput:   "Nunc scelerisque viverra mauris in aliquam.\nPellentesque elit ullamcorper dignissim cras tincidunt lobortis.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(testText)
			result, err := FilterLogFromScanner(bufio.NewScanner(reader), tt.messagesToSearch, tt.isRegexSearch, nil)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedOutput, result)
		})
	}
}

func Benchmark_FilterLogFromScanner(b *testing.B) {
	var m runtime.MemStats
	messagesToSearch := []string{"Pellentesque", "scelerisque", "this is not there"}
	reader := strings.NewReader(testText)
	for i := 0; i <= b.N; i++ {
		// nolint errcheck
		FilterLogFromScanner(bufio.NewScanner(reader), messagesToSearch, true, nil)
		runtime.ReadMemStats(&m)
		b.Logf("Size of allocated heap objects: %d MB, Size of heap in use: %d MB", m.Alloc/1024/1024, m.HeapInuse/1024/1024)
	}
}
