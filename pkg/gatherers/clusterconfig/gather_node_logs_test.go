package clusterconfig

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/client-go/rest"
)

func Test_nodeLogRecords(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rc := testRESTClient(t, s)

	nodes, err := readNodeTestData()
	mustNotFail(t, err, "error creating test data %+v")

	records, errs := nodeLogRecords(context.TODO(), rc, nodes)
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
	}
	if len(records) != 6 {
		t.Fatalf("unexpected number of records %d", len(records))
	}
	for _, r := range records {
		if !strings.HasPrefix(r.Name, "config/node/logs/") {
			t.Fatalf("unexpected node logs path in archive %s", r.Name)
		}
	}
}

func Test_nodeLogResourceURI(t *testing.T) {
	c, _ := rest.NewRESTClient(&url.URL{Path: ""}, "", rest.ClientContentConfig{}, nil, nil)

	type args struct {
		client rest.Interface
		name   string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Must Generate URI",
			args: args{
				client: c,
				name:   "node-test-name",
			},
			want: "/nodes/node-test-name/proxy/logs/journal",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nodeLogResourceURI(tt.args.client, tt.args.name); got != tt.want {
				t.Errorf("nodeLogResourceURI() = %v, want %v", got, tt.want)
			}
		})
	}
}

// nolint: lll
func Test_nodeLogString(t *testing.T) {
	expectedBody := `Aug 26 17:00:14 ip-10-57-11-201 hyperkube[1445]: E0826 17:00:14.128025    1445 kubelet.go:1882] "Skipping pod synchronization" err="[container runtime status check may not have completed yet, PLEG is not healthy: pleg has yet to be successful]"`
	serverData := `Aug 26 17:00:14 ip-10-57-11-201 hyperkube[1445]: I0826 17:00:14.127974    1445 kubelet.go:1858] "Starting kubelet main sync loop"
Aug 21 17:00:38 ip-10-57-11-201 hyperkube[1445]: W0826 17:00:38.117634    1445 container.go:586] Failed to update stats for container "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-podad87523d_aec6_4fa1_b4f2_d4fca2d08437.slice/
Aug 26 17:00:14 ip-10-57-11-201 hyperkube[1445]: E0826 17:00:14.128025    1445 kubelet.go:1882] "Skipping pod synchronization" err="[container runtime status check may not have completed yet, PLEG is not healthy: pleg has yet to be successful]"`

	// nolint: errcheck
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		gz := testGzipped(serverData)
		out, _ := io.ReadAll(gz)
		w.WriteHeader(http.StatusOK)
		w.Write(out)
	}))
	defer s.Close()

	c := testRESTClient(t, s)

	type args struct {
		req *rest.Request
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Test content stream",
			args: args{
				req: c.Get().Prefix("/"),
			},
			want:    expectedBody,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := nodeLogString(context.TODO(), tt.args.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("nodeLogString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("nodeLogString() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_requestNodeLog(t *testing.T) {
	c, err := rest.NewRESTClient(&url.URL{}, "", rest.ClientContentConfig{}, nil, nil)
	assert.NoErrorf(t, err, "unable to create the rest client")
	type args struct {
		client rest.Interface
		uri    string
		tail   int
		unit   string
	}
	tests := []struct {
		name string
		args args
		want *rest.Request
	}{
		{
			name: "Generate correct request",
			args: args{
				client: c,
				uri:    "/path/to/something",
				tail:   10,
				unit:   "test",
			},
			want: c.Get().
				RequestURI("/path/to/something").
				Param("tail", "10").
				Param("unit", "test").
				SetHeader("Accept", "text/plain, */*").
				SetHeader("Accept-Encoding", "gzip"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := requestNodeLog(tt.args.client, tt.args.uri, tt.args.tail, tt.args.unit)

			// This is not very nice. This reads unexported parameters of the *rest.Request type.
			// Previously we simply checked reflect.DeepEqual(got, tt.want) but it started to fail
			// with Kubernetes client 1.24
			expectedParams := GetUnexportedField(reflect.ValueOf(tt.want).Elem().FieldByName("params"))
			expectedHeaders := GetUnexportedField(reflect.ValueOf(tt.want).Elem().FieldByName("headers"))
			expectedPathPrefix := GetUnexportedField(reflect.ValueOf(tt.want).Elem().FieldByName("pathPrefix"))
			actualParams := GetUnexportedField(reflect.ValueOf(got).Elem().FieldByName("params"))
			actualHeaders := GetUnexportedField(reflect.ValueOf(got).Elem().FieldByName("headers"))
			actualPathPrefix := GetUnexportedField(reflect.ValueOf(got).Elem().FieldByName("pathPrefix"))
			assert.Exactly(t, expectedParams, actualParams)
			assert.Exactly(t, expectedHeaders, actualHeaders)
			assert.Exactly(t, expectedPathPrefix, actualPathPrefix)
		})
	}
}

func testRESTClientWithConfig(t testing.TB, srv *httptest.Server, contentConfig *rest.ClientContentConfig) *rest.RESTClient {
	base, _ := url.Parse("http://localhost")
	if srv != nil {
		var err error
		base, err = url.Parse(srv.URL)
		if err != nil {
			t.Fatalf("failed to parse test URL: %v", err)
		}
	}

	client, err := rest.NewRESTClient(base, "", *contentConfig, nil, nil)
	if err != nil {
		t.Fatalf("failed to create a client: %v", err)
	}
	return client
}

func testRESTClient(t testing.TB, srv *httptest.Server) *rest.RESTClient {
	return testRESTClientWithConfig(t, srv, &rest.ClientContentConfig{})
}

func GetUnexportedField(field reflect.Value) interface{} {
	return reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Interface()
}

// nolint: errcheck
func testGzipped(s string) io.Reader {
	out := &bytes.Buffer{}
	gw := gzip.NewWriter(out)
	gw.Write([]byte(s))
	gw.Close()
	return out
}

func readNodeTestData() (*corev1.NodeList, error) {
	f, err := os.Open("testdata/nodes.json")
	if err != nil {
		return nil, fmt.Errorf("error reading test data file %+v ", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("error reading test data file %+v ", err)
	}

	var nl *corev1.NodeList
	err = json.Unmarshal(data, &nl)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling json %+v ", err)
	}

	return nl, nil
}
