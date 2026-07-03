package gatherers

import (
	"context"
	"encoding/json"
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

var _ = g.Describe("[sig-insights] Basic Data Gathering", func() {
	defer g.GinkgoRecover()

	g.It("validates standard gatherer outputs in archive", func() {
		ctx := context.TODO()

		g.By("initializing test context")
		err := util.InitTest(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("creating PVC for archive storage in openshift-insights namespace")
		pvcName := "integration-test-basic-" + util.RandomSuffix()
		_, err = util.CreateTestPVC(ctx, pvcName, "openshift-insights")
		o.Expect(err).NotTo(o.HaveOccurred())

		kubeClient := util.GetKubeClient()
		defer kubeClient.CoreV1().PersistentVolumeClaims("openshift-insights").Delete(ctx, pvcName, metav1.DeleteOptions{})

		g.By("creating DataGather CR with all gatherers enabled")
		dg := &insightsv1.DataGather{
			ObjectMeta: metav1.ObjectMeta{
				Name: "integration-test-basic-" + util.RandomSuffix(),
			},
			Spec: insightsv1.DataGatherSpec{
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

		g.By("waiting for DataGather to complete (DataRecorded condition)")
		o.Eventually(func() bool {
			dg, err := insightsClient.InsightsV1().DataGathers().Get(ctx, created.Name, metav1.GetOptions{})
			if err != nil {
				return false
			}
			return util.HasCondition(dg, "DataRecorded", metav1.ConditionTrue)
		}, 5*time.Minute, 10*time.Second).Should(o.BeTrue(), "DataGather should complete gathering")

		g.By("mounting PVC to test pod and reading archive")
		archive, err := util.ReadArchiveFromPVC(ctx, pvcName, "openshift-insights")
		o.Expect(err).NotTo(o.HaveOccurred())

		// List all files for debugging if needed
		allFiles, err := util.ListArchiveContents(archive)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.GinkgoWriter.Printf("Archive contains %d files\n", len(allFiles))

		// Define expected gatherer outputs to validate
		expectedGatherers := []struct {
			name        string
			pathPattern string
			validator   func(content []byte) error
		}{
			{
				name:        "nodes",
				pathPattern: "config/node/",
				validator: func(content []byte) error {
					var node corev1.Node
					return json.Unmarshal(content, &node)
				},
			},
			{
				name:        "clusteroperators",
				pathPattern: "config/clusteroperator/",
				validator: func(content []byte) error {
					var co configv1.ClusterOperator
					return json.Unmarshal(content, &co)
				},
			},
			{
				name:        "namespaces",
				pathPattern: "config/namespace/",
				validator: func(content []byte) error {
					var ns corev1.Namespace
					return json.Unmarshal(content, &ns)
				},
			},
			{
				name:        "clusterversion",
				pathPattern: "config/clusterversion/",
				validator: func(content []byte) error {
					var cv configv1.ClusterVersion
					return json.Unmarshal(content, &cv)
				},
			},
		}

		// Validate each expected gatherer output
		for _, expected := range expectedGatherers {
			g.By("validating " + expected.name + " gatherer output")

			files, err := util.ExtractFilesMatching(archive, expected.pathPattern)
			o.Expect(err).NotTo(o.HaveOccurred())

			if len(files) == 0 {
				g.GinkgoWriter.Printf("WARNING: No files found matching pattern %s\n", expected.pathPattern)
				g.GinkgoWriter.Printf("Matching files in archive:\n")
				for _, file := range allFiles {
					if strings.Contains(file, expected.pathPattern) {
						g.GinkgoWriter.Printf("  - %s\n", file)
					}
				}
			}
			o.Expect(files).NotTo(o.BeEmpty(), "archive should contain %s data", expected.name)

			// Filter out non-JSON files (logs, etc.)
			jsonFiles := make(map[string][]byte)
			for filename, content := range files {
				if !strings.HasSuffix(filename, ".log") && !strings.Contains(filename, "/logs/") {
					jsonFiles[filename] = content
				}
			}

			o.Expect(jsonFiles).NotTo(o.BeEmpty(), "archive should contain JSON files for %s", expected.name)

			// Validate structure of at least one file
			validated := false
			for filename, content := range jsonFiles {
				err := expected.validator(content)
				if err != nil {
					g.GinkgoWriter.Printf("Failed to validate %s file %s: %v\n", expected.name, filename, err)
					g.GinkgoWriter.Printf("Content (first 500 chars): %s\n", string(content[:min(500, len(content))]))
				}
				o.Expect(err).NotTo(o.HaveOccurred(), "%s file %s should be valid JSON", expected.name, filename)
				validated = true
				break // Just validate one file per gatherer
			}
			o.Expect(validated).To(o.BeTrue(), "should have validated at least one %s file", expected.name)
		}
	})
})
