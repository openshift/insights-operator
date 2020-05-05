package clusterconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	imageregistryv1 "github.com/openshift/api/imageregistry/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog"

	imageregistryfake "github.com/openshift/client-go/imageregistry/clientset/versioned/fake"
	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
)

func TestConfigMapAnonymizer(t *testing.T) {
	klog.SetOutput(utils.NewTestLog(t).Writer())

	var cases = []struct {
		testName               string
		configMapName          string
		expectedAnonymizedJSON string
	}{
		{
			"ConfigMap Non PEM data",
			"openshift-install",
			`{
				"invoker":"codeReadyContainers",
				"version":"unreleased-master-2205-g2055609f95b19322ee6cfdd0bea73399297c4a3e"
			}`,
		},
		{
			"ConfigMap PEM is anonymized",
			"initial-kube-apiserver-server-ca",
			`{
				"ca-bundle.crt": "-----BEGIN CERTIFICATE-----\nANONYMIZED\n-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\nANONYMIZED\n-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\nANONYMIZED\n-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\nANONYMIZED\n-----END CERTIFICATE-----\n-----BEGIN CERTIFICATE-----\nANONYMIZED\n-----END CERTIFICATE-----\n"
			}`,
		},
		{
			"ConfigMap BinaryData non anonymized",
			"test-binary",
			`{
				"ls": "z/rt/gcAAAEDAA=="
			}`,
		},
	}

	for _, tt := range cases {
		t.Run(tt.testName, func(t *testing.T) {
			f, err := os.Open("testdata/configmaps.json")
			mustNotFail(t, err, "error opening test data file. %+v")
			defer f.Close()
			bts, err := ioutil.ReadAll(f)
			mustNotFail(t, err, "error reading test data file. %+v")
			var cml *v1.ConfigMapList
			mustNotFail(t, json.Unmarshal([]byte(bts), &cml), "error unmarshalling json %+v")
			cm := findMap(cml, tt.configMapName)
			mustNotFail(t, cm != nil, "haven't found a ConfigMap %+v")
			var res []byte
			cmdata := map[string]string{}
			addAnonymized := func(cmdata map[string]string, dn string, encodebase64 bool, d []byte) {
				m := record.Marshalable(ConfigMapAnonymizer{v: d, encodeBase64: encodebase64})

				res, err = m.Marshal(context.TODO())
				cmdata[dn] = string(res)
				mustNotFail(t, err, "serialization failed %+v")
			}
			for dn, dv := range cm.Data {
				addAnonymized(cmdata, dn, false, []byte(dv))
			}
			for dn, dv := range cm.BinaryData {
				addAnonymized(cmdata, dn, true, dv)
			}
			var md []byte
			md, err = json.Marshal(cmdata)
			mustNotFail(t, err, "marshaling failed %+v")
			d := map[string]string{}
			err = json.Unmarshal([]byte(tt.expectedAnonymizedJSON), &d)
			mustNotFail(t, err, "unmarshaling of expected failed %+v")
			exp, err := json.Marshal(d)
			mustNotFail(t, err, "marshaling of expected failed %+v")
			if string(exp) != string(md) {
				t.Fatalf("The test %s result is unexpected. Result: \n%s \nExpected \n%s", tt.testName, string(md), string(exp))
			}
		})
	}

}

func TestGatherClusterImageRegistry(t *testing.T) {
	tests := []struct {
		name       string
		inputObj   *imageregistryv1.Config
		evalOutput func(t *testing.T, obj *imageregistryv1.Config)
	}{
		{
			name: "httpSecret",
			inputObj: &imageregistryv1.Config{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: imageregistryv1.ImageRegistrySpec{
					HTTPSecret: "secret",
				},
			},
			evalOutput: func(t *testing.T, obj *imageregistryv1.Config) {
				if obj.Spec.HTTPSecret != "xxxxxx" {
					t.Errorf("expected HTTPSecret anonymized, got %q", obj.Spec.HTTPSecret)
				}
			},
		},
		{
			name: "s3",
			inputObj: &imageregistryv1.Config{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: imageregistryv1.ImageRegistrySpec{
					Storage: imageregistryv1.ImageRegistryConfigStorage{
						S3: &imageregistryv1.ImageRegistryConfigStorageS3{
							Bucket:         "foo",
							Region:         "bar",
							RegionEndpoint: "point",
							KeyID:          "key",
						},
					},
				},
			},
			evalOutput: func(t *testing.T, obj *imageregistryv1.Config) {
				if obj.Spec.Storage.S3.Bucket != "xxx" {
					t.Errorf("expected s3 bucket anonymized, got %q", obj.Spec.Storage.S3.Bucket)
				}
				if obj.Spec.Storage.S3.Region != "xxx" {
					t.Errorf("expected s3 region anonymized, got %q", obj.Spec.Storage.S3.Region)
				}
				if obj.Spec.Storage.S3.RegionEndpoint != "xxxxx" {
					t.Errorf("expected s3 region endpoint anonymized, got %q", obj.Spec.Storage.S3.RegionEndpoint)
				}
				if obj.Spec.Storage.S3.KeyID != "xxx" {
					t.Errorf("expected s3 keyID anonymized, got %q", obj.Spec.Storage.S3.KeyID)
				}
			},
		},
		{
			name: "azure",
			inputObj: &imageregistryv1.Config{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: imageregistryv1.ImageRegistrySpec{
					Storage: imageregistryv1.ImageRegistryConfigStorage{
						Azure: &imageregistryv1.ImageRegistryConfigStorageAzure{
							AccountName: "account",
							Container:   "container",
						},
					},
				},
			},
			evalOutput: func(t *testing.T, obj *imageregistryv1.Config) {
				if obj.Spec.Storage.Azure.AccountName != "xxxxxxx" {
					t.Errorf("expected azure account name anonymized, got %q", obj.Spec.Storage.Azure.AccountName)
				}
				if obj.Spec.Storage.Azure.Container == "xxxxxxx" {
					t.Errorf("expected azure container anonymized, got %q", obj.Spec.Storage.Azure.Container)
				}
			},
		},
		{
			name: "gcs",
			inputObj: &imageregistryv1.Config{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cluster",
				},
				Spec: imageregistryv1.ImageRegistrySpec{
					Storage: imageregistryv1.ImageRegistryConfigStorage{
						GCS: &imageregistryv1.ImageRegistryConfigStorageGCS{
							Bucket:    "bucket",
							Region:    "region",
							ProjectID: "foo",
							KeyID:     "bar",
						},
					},
				},
			},
			evalOutput: func(t *testing.T, obj *imageregistryv1.Config) {
				if obj.Spec.Storage.GCS.Bucket != "xxxxxx" {
					t.Errorf("expected gcs bucket anonymized, got %q", obj.Spec.Storage.GCS.Bucket)
				}
				if obj.Spec.Storage.GCS.ProjectID != "xxx" {
					t.Errorf("expected gcs projectID endpoint anonymized, got %q", obj.Spec.Storage.GCS.ProjectID)
				}
				if obj.Spec.Storage.GCS.KeyID != "xxx" {
					t.Errorf("expected gcs keyID anonymized, got %q", obj.Spec.Storage.GCS.KeyID)
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := imageregistryfake.NewSimpleClientset(test.inputObj)
			gatherer := &Gatherer{registryClient: client.ImageregistryV1()}
			records, errs := GatherClusterImageRegistry(gatherer)()
			if len(errs) > 0 {
				t.Errorf("unexpected errors: %#v", errs)
				return
			}
			if numRecords := len(records); numRecords != 1 {
				t.Errorf("expected one record, got %d", numRecords)
				return
			}
			if expectedRecordName := "config/imageregistry"; records[0].Name != expectedRecordName {
				t.Errorf("expected %q record name, got %q", expectedRecordName, records[0].Name)
				return
			}
			item := records[0].Item
			itemBytes, err := item.Marshal(context.TODO())
			if err != nil {
				t.Fatalf("unable to marshal config: %v", err)
			}
			var output imageregistryv1.Config
			obj, _, err := registrySerializer.LegacyCodec(imageregistryv1.SchemeGroupVersion).Decode(itemBytes, nil, &output)
			if err != nil {
				t.Fatalf("failed to decode object: %v", err)
			}
			test.evalOutput(t, obj.(*imageregistryv1.Config))
		})
	}

}

func ExampleGatherMostRecentMetrics_Test() {
	b, err := ExampleMostRecentMetrics()
	if err != nil {
		fmt.Print(err)
	}
	fmt.Print(string(b))
	// Output:
	// [{"Name":"config/metrics","Captured":"0001-01-01T00:00:00Z","Fingerprint":"","Item":"SGVsbG8sIGNsaWVudAo="}]
}

func ExampleGatherClusterOperators_Test() {
	b, err := ExampleClusterOperators()
	if err != nil {
		fmt.Print(err)
	}
	fmt.Print(string(b))
	// Output:
	// [{"Name":"config/clusteroperator/","Captured":"0001-01-01T00:00:00Z","Fingerprint":"","Item":{"metadata":{"creationTimestamp":null},"spec":{},"status":{"conditions":[{"type":"Degraded","status":"","lastTransitionTime":null}],"extension":null}}}]
}

func ExampleGatherUnhealthyNodes_Test() {
	b, err := ExampleUnhealthyNodes()
	if err != nil {
		fmt.Print(err)
	}
	fmt.Print(string(b))
	// Output:
	// [{"Name":"config/node/","Captured":"0001-01-01T00:00:00Z","Fingerprint":"","Item":{"metadata":{"creationTimestamp":null},"spec":{},"status":{"conditions":[{"type":"Ready","status":"False","lastHeartbeatTime":null,"lastTransitionTime":null}],"daemonEndpoints":{"kubeletEndpoint":{"Port":0}},"nodeInfo":{"machineID":"","systemUUID":"","bootID":"","kernelVersion":"","osImage":"","containerRuntimeVersion":"","kubeletVersion":"","kubeProxyVersion":"","operatingSystem":"","architecture":""}}}}]
}

func mustNotFail(t *testing.T, err interface{}, fmtstr string) {
	if e, ok := err.(error); ok && e != nil {
		t.Fatalf(fmtstr, e)
	}
	if e, ok := err.(bool); ok && !e {
		t.Fatalf(fmtstr, e)
	}
}

func findMap(cml *v1.ConfigMapList, name string) *v1.ConfigMap {
	for _, it := range cml.Items {
		if it.Name == name {
			return &it
		}
	}
	return nil
}
