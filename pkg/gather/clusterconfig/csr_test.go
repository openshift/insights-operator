package clusterconfig

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"testing"

	certificatesv1b1api "k8s.io/api/certificates/v1beta1"
)

func TestCSRs(t *testing.T) {
	var files = []struct {
		dataFile string
		expFile  string
	}{
		{"testdata/csr_appr.json", "testdata/csr_appr_anon.json"},
		{"testdata/csr_unappr.json", "testdata/csr_unappr_anon.json"},
	}

	for _, tt := range files {
		t.Run(tt.dataFile, func(t *testing.T) {

			r := &certificatesv1b1api.CertificateSigningRequest{}

			f, err := os.Open(tt.dataFile)
			if err != nil {
				t.Fatal("test failed to unmarshal csr data", err)
			}
			defer f.Close()
			bts, err := ioutil.ReadAll(f)
			if err != nil {
				t.Fatal("error reading test data file", err)
			}
			err = json.Unmarshal([]byte(bts), r)
			if err != nil {
				t.Fatal("test failed to unmarshal csr data", err)
			}
			exp := &CSRAnonymizedFeatures{}

			f, err = os.Open(tt.expFile)
			if err != nil {
				t.Fatal("test failed to unmarshal csr anonymized data", err)
			}
			defer f.Close()
			bts, err = ioutil.ReadAll(f)
			if err != nil {
				t.Fatal("error reading test data file", err)
			}
			err = json.Unmarshal([]byte(bts), exp)
			if err != nil {
				t.Fatal("test failed to unmarshal anonymized csr data", err)
			}

			a, err := anonymizeCsr(r)
			ss, _ := json.Marshal(a)
			_ = ss
			if err != nil {
				t.Fatal("should not fail", err)
			}
			if !reflect.DeepEqual(exp, a) {
				t.Fatal("Expected", exp, "but got", a)
			}
		})
	}
}
