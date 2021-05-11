package clusterconfig

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"

	"github.com/openshift/insights-operator/pkg/record"
	"github.com/openshift/insights-operator/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

//nolint: lll
func Test_ConfigMap_Anonymizer(t *testing.T) {
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
		tt := tt
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel()
			cml, err := readConfigMapsTestData()
			mustNotFail(t, err, "error creating test data %+v")
			cm := findMap(cml, tt.configMapName)
			mustNotFail(t, cm != nil, "haven't found a ConfigMap %+v")
			// just to make lint happy
			if cm == nil {
				return
			}
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

func mustNotFail(t *testing.T, err interface{}, fmtstr string) {
	if e, ok := err.(error); ok && e != nil {
		t.Fatalf(fmtstr, e)
	}
	if e, ok := err.(bool); ok && !e {
		t.Fatalf(fmtstr, e)
	}
}

func findMap(cml *corev1.ConfigMapList, name string) *corev1.ConfigMap {
	for i := range cml.Items {
		if cml.Items[i].Name == name {
			return &cml.Items[i]
		}
	}
	return nil
}

func readConfigMapsTestData() (*corev1.ConfigMapList, error) {
	f, err := os.Open("testdata/configmaps.json")
	if err != nil {
		return nil, fmt.Errorf("error reading test data file %+v ", err)
	}

	defer f.Close()

	bts, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("error reading test data file %+v ", err)
	}

	var cml *corev1.ConfigMapList
	err = json.Unmarshal(bts, &cml)
	if err != nil {
		return nil, fmt.Errorf("error unmarshalling json %+v ", err)
	}

	return cml, nil
}

func Test_ConfigMap_Gather(t *testing.T) {
	cml, err := readConfigMapsTestData()
	mustNotFail(t, err, "error creating test data %+v")
	coreClient := kubefake.NewSimpleClientset()

	for _, cm := range cml.Items {
		_, err := coreClient.CoreV1().ConfigMaps(cm.Namespace).Create(context.Background(), &cm, metav1.CreateOptions{}) //nolint: gosec
		if err != nil {
			t.Fatalf("error creating configmap %s", cm.Name)
		}
	}
	records, errs := gatherConfigMaps(context.Background(), coreClient.CoreV1())
	if len(errs) > 0 {
		t.Errorf("unexpected errors: %#v", errs)
		return
	}
	if len(records) != 8 {
		t.Fatalf("unexpected number of configmaps gathered %d", len(records))
	}
	for _, r := range records {
		if !strings.HasPrefix(r.Name, "config/configmaps/openshift-config/") {
			t.Fatalf("unexpected configmap path in archive %s", r.Name)
		}
	}
}

func Test_ConfigMap_YAML_Data(t *testing.T) {
	var cases = []struct {
		testName      string
		testCM        corev1.ConfigMap
		expNumOfRec   int
		expectedError error
	}{
		{
			"Valid monitoring config map",
			corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-monitoring-config",
					Namespace: "openshift-monitoring",
				},
				Data: map[string]string{
					"config.yaml": "\"valid\"",
				},
			},
			1,
			nil,
		},
		{
			"Invalid monitoring config map",
			corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cluster-monitoring-config",
					Namespace: "openshift-monitoring",
				},
				Data: map[string]string{
					"config.yaml": "invalid: test:",
				},
			},
			0,
			fmt.Errorf("yaml: mapping values are not allowed in this context"),
		},
	}

	for _, tt := range cases {
		t.Run(tt.testName, func(t *testing.T) {
			coreClient := kubefake.NewSimpleClientset()
			_, err := coreClient.CoreV1().ConfigMaps(tt.testCM.Namespace).Create(context.Background(), &tt.testCM, metav1.CreateOptions{})
			if err != nil {
				t.Fatalf("cannot create %s config map: %v", tt.testCM.Name, err)
			}
			records, errs := gatherMonitoringCM(context.Background(), coreClient.CoreV1())
			if len(errs) > 0 {
				if errs[0].Error() != tt.expectedError.Error() {
					t.Fatalf("unexpected errors: %v", errs[0].Error())
				}
			}
			if tt.expNumOfRec > 0 {
				r := records[0].Item.(RawJSON)
				if string(r) != tt.testCM.Data["config.yaml"] {
					t.Fatalf("unexpected value %s", string(r))
				}
			}
		})
	}
}
