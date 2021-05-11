package clusterconfig

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
	"time"

	certificatesv1api "k8s.io/api/certificates/v1"
)

func Test_CSR(t *testing.T) {
	var files = []struct {
		dataFile string
		expFile  string
	}{
		{"testdata/csr_appr.json", "testdata/csr_appr_anon.json"},
		{"testdata/csr_unappr.json", "testdata/csr_unappr_anon.json"},
	}

	for _, tt := range files {
		tt := tt
		t.Run(tt.dataFile, func(t *testing.T) {
			t.Parallel()

			r := &certificatesv1api.CertificateSigningRequest{}

			f, err := os.Open(tt.dataFile)
			if err != nil {
				t.Fatal("test failed to unmarshal csr data", err)
			}
			defer f.Close()
			bts, err := ioutil.ReadAll(f)
			if err != nil {
				t.Fatal("error reading test data file", err)
			}
			err = json.Unmarshal(bts, r)
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
			err = json.Unmarshal(bts, exp)
			if err != nil {
				t.Fatal("test failed to unmarshal anonymized csr data", err)
			}

			a := anonymizeCSR(r)
			if !reflect.DeepEqual(exp, a) {
				t.Fatal("Expected", exp, "but got", a)
			}
		})
	}
}

// Verifies if CSR features will be ignored in package
func Test_CSR_Filters(t *testing.T) {
	var files = []struct {
		name             string
		csr              *CSRAnonymizedFeatures
		shouldBeIncluded bool
	}{
		{"Verified shoudln't be included", &CSRAnonymizedFeatures{Status: &StatusFeatures{Cert: &CertFeatures{Verified: true}}}, false},
		{"Non verified (empty) will be included", &CSRAnonymizedFeatures{}, true},
		{"Non verified will be included", &CSRAnonymizedFeatures{Status: &StatusFeatures{Cert: &CertFeatures{Verified: false}}}, true},
		// NotAfter/NotBefore should be in time.RFC3339
		{"Verified, but not valid yet will be included", &CSRAnonymizedFeatures{Status: &StatusFeatures{Cert: &CertFeatures{Verified: true,
			NotBefore: "2020-02-20T09:38:42+01:00"}}}, true},
		{"Verified, but already not valid will be included", &CSRAnonymizedFeatures{Status: &StatusFeatures{Cert: &CertFeatures{Verified: true,
			NotAfter: "2020-02-16T09:38:42+01:00"}}}, true},
		{"Verified and valid shouldn't be included", &CSRAnonymizedFeatures{Status: &StatusFeatures{Cert: &CertFeatures{Verified: true,
			NotBefore: "2020-02-16T09:38:42+01:00", NotAfter: "2020-02-20T09:38:42+01:00"}}}, false},
	}

	for i, tt := range files {
		n := csrName(tt.csr, fmt.Sprintf("[n/a:%d]", i))
		tt := tt
		t.Run(n, func(t *testing.T) {
			t.Parallel()

			now, err := time.Parse("Mon Jan 02 2006 15:04:05 GMT-0700 (MST)", "Tue Feb 18 2020 09:38:42 GMT+0100 (CEST)")
			if err != nil {
				t.Fatalf("parse couldnt parse date %v", err)
			}
			isIncl := IncludeCSR(tt.csr, WithTime(now))
			if isIncl != tt.shouldBeIncluded {
				t.Errorf("%s CSR %s Should %s included but it %s", t.Name(), tt.name, tobePres[tt.shouldBeIncluded], tobePast[isIncl])
			}
		})
	}
}

func csrName(csr *CSRAnonymizedFeatures, def string) string {
	if csr == nil {
		return def
	}
	if csr.Spec == nil {
		return def
	}
	if csr.Spec.Request != nil {
		return def
	}
	return csr.Spec.Request.Subject.String()
}

var (
	tobePast = map[bool]string{
		true:  "was",
		false: "wasn't",
	}
	tobePres = map[bool]string{
		true:  "be",
		false: "not be",
	}
)
