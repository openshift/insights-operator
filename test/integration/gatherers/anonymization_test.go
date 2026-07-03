package gatherers

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	configv1 "github.com/openshift/api/config/v1"
	insightsv1 "github.com/openshift/api/insights/v1"
	"github.com/openshift/insights-operator/test/integration/util"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = g.Describe("[sig-insights] Data Anonymization", func() {
	defer g.GinkgoRecover()

	// Table-driven test for different anonymization options
	type anonymizationValidation struct {
		name          string
		checkFunc     func(archive []byte) error
		expectedMatch bool // true if pattern should match, false if shouldn't
	}

	type anonymizationTestCase struct {
		name            string
		dataPolicies    []insightsv1.DataPolicyOption
		validations     []anonymizationValidation
		description     string
	}

	testCases := []anonymizationTestCase{
		{
			name:         "ObfuscateNetworking",
			dataPolicies: []insightsv1.DataPolicyOption{insightsv1.DataPolicyOptionObfuscateNetworking},
			description:  "IP addresses and domain names should be obfuscated",
			validations: []anonymizationValidation{
				{
					name: "IP addresses should be obfuscated",
					checkFunc: func(archive []byte) error {
						// Extract infrastructure to check for obfuscated IPs
						files, err := util.ExtractFilesMatching(archive, "config/infrastructure/")
						if err != nil {
							return err
						}
						if len(files) == 0 {
							return fmt.Errorf("no infrastructure files found")
						}

						// Look for real IP addresses (should NOT be present when obfuscated)
						ipPattern := regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)
						for filename, content := range files {
							if strings.HasSuffix(filename, ".json") {
								var infra configv1.Infrastructure
								if err := json.Unmarshal(content, &infra); err == nil {
									// Check if API server URL contains real IP (should be obfuscated)
									if ipPattern.MatchString(infra.Status.APIServerURL) {
										return fmt.Errorf("found non-obfuscated IP in APIServerURL: %s", infra.Status.APIServerURL)
									}
								}
							}
						}
						return nil
					},
					expectedMatch: true,
				},
				{
					name: "domain names should be obfuscated",
					checkFunc: func(archive []byte) error {
						files, err := util.ExtractFilesMatching(archive, "config/infrastructure/")
						if err != nil {
							return err
						}

						for filename, content := range files {
							if strings.HasSuffix(filename, ".json") {
								var infra configv1.Infrastructure
								if err := json.Unmarshal(content, &infra); err == nil {
									// When obfuscated, domain should be replaced with xxxxx
									if strings.Contains(infra.Status.APIServerURL, ".") &&
									   !strings.Contains(infra.Status.APIServerURL, "xxxxx") {
										return fmt.Errorf("domain name not obfuscated in APIServerURL: %s", infra.Status.APIServerURL)
									}
								}
							}
						}
						return nil
					},
					expectedMatch: true,
				},
			},
		},
		{
			name:         "WorkloadNames",
			dataPolicies: []insightsv1.DataPolicyOption{insightsv1.DataPolicyOptionObfuscateWorkloadNames},
			description:  "Workload names should be replaced with UIDs",
			validations: []anonymizationValidation{
				{
					name: "pod names should be UIDs",
					checkFunc: func(archive []byte) error {
						files, err := util.ExtractFilesMatching(archive, "config/pod/")
						if err != nil {
							return err
						}
						if len(files) == 0 {
							// Pod data might not always be gathered, skip
							return nil
						}

						// UUID pattern: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
						uuidPattern := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

						for filename, content := range files {
							if strings.HasSuffix(filename, ".json") {
								var pod corev1.Pod
								if err := json.Unmarshal(content, &pod); err == nil {
									// When workload names are obfuscated, name should be UID format
									if !uuidPattern.MatchString(pod.Name) && pod.UID != "" {
										return fmt.Errorf("pod name is not obfuscated (should be UID): %s", pod.Name)
									}
								}
							}
						}
						return nil
					},
					expectedMatch: true,
				},
			},
		},
		{
			name:         "Both anonymization options",
			dataPolicies: []insightsv1.DataPolicyOption{insightsv1.DataPolicyOptionObfuscateNetworking, insightsv1.DataPolicyOptionObfuscateWorkloadNames},
			description:  "Both IP/domain and workload names should be obfuscated",
			validations: []anonymizationValidation{
				{
					name: "combined obfuscation check",
					checkFunc: func(archive []byte) error {
						// Just verify archive was created successfully
						// Individual validations are tested in separate cases
						files, err := util.ListArchiveContents(archive)
						if err != nil {
							return err
						}
						if len(files) == 0 {
							return fmt.Errorf("archive is empty")
						}
						return nil
					},
					expectedMatch: true,
				},
			},
		},
	}

	// Run table-driven tests
	for _, tc := range testCases {
		tc := tc // capture range variable
		g.It(fmt.Sprintf("validates anonymization with %s", tc.name), func() {
			ctx := context.TODO()

			g.By("initializing test context")
			err := util.InitTest(ctx)
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("creating PVC for archive storage")
			pvcName := "integration-test-anon-" + util.RandomSuffix()
			_, err = util.CreateTestPVC(ctx, pvcName, "openshift-insights")
			o.Expect(err).NotTo(o.HaveOccurred())

			kubeClient := util.GetKubeClient()
			defer kubeClient.CoreV1().PersistentVolumeClaims("openshift-insights").Delete(ctx, pvcName, metav1.DeleteOptions{})

			g.By(fmt.Sprintf("creating DataGather with anonymization: %v", tc.dataPolicies))
			dg := &insightsv1.DataGather{
				ObjectMeta: metav1.ObjectMeta{
					Name: "integration-test-anon-" + util.RandomSuffix(),
				},
				Spec: insightsv1.DataGatherSpec{
					DataPolicy: tc.dataPolicies,
					Gatherers: insightsv1.Gatherers{
						Mode: insightsv1.GatheringModeAll,
					},
					Storage: insightsv1.Storage{
						Type: insightsv1.StorageTypePersistentVolume,
						PersistentVolume: insightsv1.PersistentVolumeConfig{
							Claim: insightsv1.PersistentVolumeClaimReference{
								Name: pvcName,
							},
						},
					},
				},
			}

			insightsClient := util.GetInsightsClient()
			created, err := insightsClient.InsightsV1().DataGathers().Create(ctx, dg, metav1.CreateOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())
			defer insightsClient.InsightsV1().DataGathers().Delete(ctx, created.Name, metav1.DeleteOptions{})

			g.By("waiting for DataGather to complete")
			o.Eventually(func() bool {
				dg, err := insightsClient.InsightsV1().DataGathers().Get(ctx, created.Name, metav1.GetOptions{})
				if err != nil {
					return false
				}
				return util.HasCondition(dg, "DataRecorded", metav1.ConditionTrue)
			}, 5*time.Minute, 10*time.Second).Should(o.BeTrue(), "DataGather should complete")

			g.By("reading archive from PVC")
			archive, err := util.ReadArchiveFromPVC(ctx, pvcName, "openshift-insights")
			o.Expect(err).NotTo(o.HaveOccurred())

			// Run all validations for this test case
			for _, validation := range tc.validations {
				g.By(validation.name)
				err := validation.checkFunc(archive)
				if validation.expectedMatch {
					o.Expect(err).NotTo(o.HaveOccurred(), validation.name+" should pass")
				} else {
					o.Expect(err).To(o.HaveOccurred(), validation.name+" should fail")
				}
			}
		})
	}
})
