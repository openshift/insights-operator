package gatherers

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	insightsv1 "github.com/openshift/api/insights/v1"
	"github.com/openshift/insights-operator/test/integration/util"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = g.Describe("[sig-insights] Gatherer Content Validation", func() {
	defer g.GinkgoRecover()

	g.It("validates node gathering in archive", func() {
		ctx := context.TODO()

		g.By("initializing test context")
		err := util.InitTest(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("creating PVC for archive storage in openshift-insights namespace")
		pvcName := "integration-test-pvc-" + util.RandomSuffix()
		_, err = util.CreateTestPVC(ctx, pvcName, "openshift-insights")
		o.Expect(err).NotTo(o.HaveOccurred())

		kubeClient := util.GetKubeClient()
		defer kubeClient.CoreV1().PersistentVolumeClaims("openshift-insights").Delete(ctx, pvcName, metav1.DeleteOptions{})

		g.By("creating DataGather CR with PersistentVolume storage")
		dg := &insightsv1.DataGather{
			ObjectMeta: metav1.ObjectMeta{
				Name: "integration-test-nodes-" + util.RandomSuffix(),
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

		g.By("validating archive contains node data")
		allNodeFiles, err := util.ExtractFilesMatching(archive, "config/node/")
		o.Expect(err).NotTo(o.HaveOccurred())

		if len(allNodeFiles) == 0 {
			// Print archive contents for debugging
			allFiles, listErr := util.ListArchiveContents(archive)
			if listErr == nil {
				g.GinkgoWriter.Printf("Archive contents (%d files):\n", len(allFiles))
				for _, file := range allFiles {
					g.GinkgoWriter.Printf("  - %s\n", file)
				}
			}
		}
		o.Expect(allNodeFiles).NotTo(o.BeEmpty(), "archive should contain node data")

		// Filter to only JSON files (node definitions), excluding .log files
		nodeFiles := make(map[string][]byte)
		for filename, content := range allNodeFiles {
			if !strings.HasSuffix(filename, ".log") && !strings.Contains(filename, "/logs/") {
				nodeFiles[filename] = content
			}
		}

		if len(nodeFiles) == 0 {
			g.GinkgoWriter.Printf("Found %d files in config/node/ but none are JSON node definitions (all are logs or other files)\n", len(allNodeFiles))
			g.GinkgoWriter.Printf("Files found:\n")
			for filename := range allNodeFiles {
				g.GinkgoWriter.Printf("  - %s\n", filename)
			}
		}
		o.Expect(nodeFiles).NotTo(o.BeEmpty(), "archive should contain JSON node definitions (not just logs)")

		g.By("validating node data structure")
		for filename, content := range nodeFiles {
			var node corev1.Node
			err = json.Unmarshal(content, &node)
			if err != nil {
				g.GinkgoWriter.Printf("Failed to parse %s, content (first 500 chars):\n%s\n", filename, string(content[:min(500, len(content))]))
			}
			o.Expect(err).NotTo(o.HaveOccurred(), "node file %s should be valid JSON", filename)
			o.Expect(node.Name).NotTo(o.BeEmpty(), "node in file %s should have a name", filename)
			o.Expect(node.Status.Capacity).NotTo(o.BeEmpty(), "node in file %s should have capacity info", filename)
		}
	})
})
