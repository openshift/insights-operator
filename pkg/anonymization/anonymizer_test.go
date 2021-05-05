package anonymization

import (
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	kubefake "k8s.io/client-go/kubernetes/fake"

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

	anonymizer, err := NewAnonymizer(clusterBaseDomain, networks, kubefake.NewSimpleClientset().CoreV1().Secrets("test"))
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
}
