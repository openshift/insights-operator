package anonymization

import (
	"fmt"
	"net"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/stretchr/testify/assert"
	kubefake "k8s.io/client-go/kubernetes/fake"
	corefake "k8s.io/client-go/kubernetes/typed/core/v1/fake"
	clienttesting "k8s.io/client-go/testing"

	"github.com/openshift/insights-operator/pkg/record"
)

func Test_GetNextIP(t *testing.T) {
	type testCase struct {
		originalIP net.IP
		nextIP     net.IP
		mask       net.IPMask
		overflow   bool
	}
	testCases := []testCase{
		{
			originalIP: net.IPv4(127, 0, 0, 0),
			nextIP:     net.IPv4(127, 0, 0, 1),
			mask:       net.IPv4Mask(255, 255, 255, 0),
			overflow:   false,
		},
		{
			originalIP: net.IPv4(192, 168, 0, 1),
			nextIP:     net.IPv4(192, 168, 0, 2),
			mask:       net.IPv4Mask(255, 255, 0, 0),
			overflow:   false,
		},
		{
			originalIP: net.IPv4(192, 168, 0, 254),
			nextIP:     net.IPv4(192, 168, 0, 255),
			mask:       net.IPv4Mask(255, 255, 0, 0),
			overflow:   false,
		},
		{
			originalIP: net.IPv4(192, 168, 0, 255),
			nextIP:     net.IPv4(192, 168, 1, 0),
			mask:       net.IPv4Mask(255, 255, 0, 0),
			overflow:   false,
		},
		{
			originalIP: net.IPv4(192, 168, 255, 255),
			nextIP:     net.IPv4(192, 168, 0, 0),
			mask:       net.IPv4Mask(255, 255, 0, 0),
			overflow:   true,
		},
		{
			originalIP: net.IPv4(10, 0, 0, 54),
			nextIP:     net.IPv4(10, 0, 0, 55),
			mask:       net.IPv4Mask(255, 255, 255, 254),
			overflow:   false,
		},
		{
			originalIP: net.IPv4(10, 0, 0, 55),
			nextIP:     net.IPv4(10, 0, 0, 54),
			mask:       net.IPv4Mask(255, 255, 255, 254),
			overflow:   true,
		},
		{
			originalIP: net.IPv4(255, 255, 255, 255),
			nextIP:     net.IPv4(255, 255, 255, 255),
			mask:       net.IPv4Mask(255, 255, 255, 255),
			overflow:   true,
		},
		{
			originalIP: net.IPv4(255, 255, 255, 255),
			nextIP:     net.IPv4(0, 0, 0, 0),
			mask:       net.IPv4Mask(0, 0, 0, 0),
			overflow:   false,
		},
		// IPv6
		{
			originalIP: net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			nextIP:     net.IP{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
			mask:       net.IPMask{255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			overflow:   false,
		},
		// IPv6
		{
			originalIP: net.IP{16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 255, 255, 255},
			nextIP:     net.IP{16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0},
			mask:       net.IPMask{255, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
			overflow:   false,
		},
	}

	for _, testCase := range testCases {
		nextIP, overflow := getNextIP(testCase.originalIP, testCase.mask)
		assert.True(
			t,
			nextIP.Equal(testCase.nextIP),
			"IP %v and %v are not equal",
			nextIP.String(),
			testCase.nextIP,
		)
		assert.Equal(t, overflow, testCase.overflow)
	}
}

func getAnonymizer(t *testing.T) *Anonymizer {
	clusterBaseDomain := "example.com"
	networks := []string{
		"127.0.0.0/8",
		"192.168.0.0/16",
	}
	anonymizer, err := NewAnonymizer(clusterBaseDomain, networks, kubefake.NewSimpleClientset().CoreV1().Secrets(secretNamespace))
	assert.NoError(t, err)

	return anonymizer
}

func Test_Anonymizer(t *testing.T) {
	anonymizer := getAnonymizer(t)

	type testCase struct {
		before string
		after  string
	}

	nameTestCases := []testCase{
		{"node1.example.com", "node1.<CLUSTER_BASE_DOMAIN>"},
		{"api.example.com/test", "api.<CLUSTER_BASE_DOMAIN>/test"},
	}
	dataTestCases := []testCase{
		{"api.example.com\n127.0.0.1  ", "api.<CLUSTER_BASE_DOMAIN>\n127.0.0.1  "},
		{"api.example.com\n127.0.0.128  ", "api.<CLUSTER_BASE_DOMAIN>\n127.0.0.2  "},
		{"127.0.0.1  ", "127.0.0.1  "},
		{"127.0.0.128  ", "127.0.0.2  "},
		{"192.168.1.15  ", "192.168.0.1  "},
		{"192.168.1.5  ", "192.168.0.2  "},
		{"192.168.1.255  ", "192.168.0.3  "},
		{"192.169.1.255  ", "0.0.0.0  "},
		{`{"key1": "val1", "key2": "127.0.0.128"'}`, `{"key1": "val1", "key2": "127.0.0.2"'}`},
	}

	for _, testCase := range nameTestCases {
		obfuscatedName := anonymizer.AnonymizeMemoryRecord(&record.MemoryRecord{
			Name: testCase.before,
		}).Name

		assert.Equal(t, testCase.after, obfuscatedName)
	}

	for _, testCase := range dataTestCases {
		obfuscatedData := string(anonymizer.AnonymizeMemoryRecord(&record.MemoryRecord{
			Data: []byte(testCase.before),
		}).Data)

		assert.Equal(t, testCase.after, obfuscatedData)
	}
}

func Test_Anonymizer_TranslationTableTest(t *testing.T) {
	anonymizer := getAnonymizer(t)

	for i := 0; i < 254; i++ {
		obfuscatedData := string(anonymizer.AnonymizeMemoryRecord(&record.MemoryRecord{
			Data: []byte(fmt.Sprintf("192.168.0.%v", 255-i)),
		}).Data)

		assert.Equal(t, fmt.Sprintf("192.168.0.%v", i+1), obfuscatedData)
	}

	// 192.168.0.0 is the network address, we don't want to change it
	obfuscatedData := string(anonymizer.AnonymizeMemoryRecord(&record.MemoryRecord{
		Data: []byte("192.168.0.0"),
	}).Data)

	assert.Equal(t, "192.168.0.0", obfuscatedData)

	obfuscatedData = string(anonymizer.AnonymizeMemoryRecord(&record.MemoryRecord{
		Data: []byte("192.168.1.255"),
	}).Data)

	assert.Equal(t, "192.168.0.255", obfuscatedData)

	obfuscatedData = string(anonymizer.AnonymizeMemoryRecord(&record.MemoryRecord{
		Data: []byte("192.168.1.55"),
	}).Data)

	assert.Equal(t, "192.168.1.0", obfuscatedData)

	obfuscatedData = string(anonymizer.AnonymizeMemoryRecord(&record.MemoryRecord{
		Data: []byte("192.168.1.56"),
	}).Data)

	assert.Equal(t, "192.168.1.1", obfuscatedData)

	assert.Equal(t, 257, len(anonymizer.translationTable))
	anonymizer.ResetTranslationTable()
	assert.Equal(t, 0, len(anonymizer.translationTable))
}

func Test_Anonymizer_StoreTranslationTable(t *testing.T) {
	anonymizer := getAnonymizer(t)

	// Empty translation table == No call made to
	assert.Nil(t, anonymizer.StoreTranslationTable())

	// Mock the client to react/check Apply calls
	kube := kubefake.Clientset{}
	client := kube.CoreV1().Secrets(secretNamespace)
	client.(*corefake.FakeSecrets).Fake.AddReactor("create", "secrets",
		func(action clienttesting.Action) (handled bool, ret runtime.Object, err error) {
			if createAction, ok := action.(clienttesting.CreateAction); ok {
				assert.Equal(t, secretNamespace, createAction.GetNamespace())
				assert.Equal(t, secretAPIVersion, createAction.GetResource().Version)
				var secret *corev1.Secret
				secret, ok = createAction.GetObject().(*corev1.Secret)
				if !ok {
					t.Errorf("Failed to convert sent Secret.")
				}
				return true, secret, nil
			}
			t.Errorf("Incorrect action, expected patch got %s", action)
			return false, nil, nil
		})
	anonymizer.secretsClient = client

	// Fill translation table
	for i := 0; i < 10; i++ {
		obfuscatedData := string(anonymizer.AnonymizeMemoryRecord(&record.MemoryRecord{
			Data: []byte(fmt.Sprintf("192.168.0.%v", 255-i)),
		}).Data)

		assert.Equal(t, fmt.Sprintf("192.168.0.%v", i+1), obfuscatedData)
	}
	// Store translation table, then check
	secret := anonymizer.StoreTranslationTable()
	for i := 0; i < 10; i++ {
		assert.Equal(t, secret.StringData[fmt.Sprintf("192.168.0.%v", 255-i)], fmt.Sprintf("192.168.0.%v", i+1))
	}
}
