package gatherers

import (
	"context"
	"strings"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	insightsv1 "github.com/openshift/api/insights/v1"
	"github.com/openshift/insights-operator/test/integration/util"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = g.Describe("[sig-insights] Gatherer Configuration", func() {
	defer g.GinkgoRecover()

	g.It("validates custom gatherer configuration with disabled gatherers", func() {
		ctx := context.TODO()

		g.By("initializing test context")
		err := util.InitTest(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("creating PVC for archive storage")
		pvcName := "integration-test-config-" + util.RandomSuffix()
		_, err = util.CreateTestPVC(ctx, pvcName, "openshift-insights")
		o.Expect(err).NotTo(o.HaveOccurred())

		kubeClient := util.GetKubeClient()
		defer kubeClient.CoreV1().PersistentVolumeClaims("openshift-insights").Delete(ctx, pvcName, metav1.DeleteOptions{})

		g.By("creating DataGather with custom gatherer configuration (disable clusterconfig/nodes)")
		dg := &insightsv1.DataGather{
			ObjectMeta: metav1.ObjectMeta{
				Name: "integration-test-config-" + util.RandomSuffix(),
			},
			Spec: insightsv1.DataGatherSpec{
				Gatherers: insightsv1.Gatherers{
					Mode: insightsv1.GatheringModeCustom,
					Custom: insightsv1.Custom{
						Configs: []insightsv1.GathererConfig{
							{
								Name:  "clusterconfig/nodes",
								State: insightsv1.GathererStateDisabled,
							},
						},
					},
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
		finalDG, err := util.WaitForDataGatherCompletion(ctx, insightsClient, created.Name, 5*time.Minute)
		o.Expect(err).NotTo(o.HaveOccurred(), "DataGather should complete")

		g.By("verifying gathering succeeded")
		err = util.ValidateDataGatherSuccess(finalDG)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("reading archive from PVC")
		archive, err := util.ReadArchiveFromPVC(ctx, pvcName, "openshift-insights")
		o.Expect(err).NotTo(o.HaveOccurred())

		allFiles, err := util.ListArchiveContents(archive)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("verifying nodes gatherer was disabled (no config/node/ files)")
		nodeFiles := 0
		for _, file := range allFiles {
			if strings.HasPrefix(file, "config/node/") || strings.Contains(file, "/config/node/") {
				nodeFiles++
			}
		}
		o.Expect(nodeFiles).To(o.Equal(0), "should have no node files when gatherer is disabled")

		g.By("verifying other gatherers still ran (archive should not be empty)")
		o.Expect(len(allFiles)).To(o.BeNumerically(">", 0), "archive should contain data from other gatherers")

		// Verify some other gatherer ran successfully
		hasOtherData := false
		for _, file := range allFiles {
			if strings.HasPrefix(file, "config/clusteroperator/") ||
			   strings.HasPrefix(file, "config/clusterversion/") {
				hasOtherData = true
				break
			}
		}
		o.Expect(hasOtherData).To(o.BeTrue(), "archive should contain data from enabled gatherers")
	})

	g.It("validates disabling entire clusterconfig gatherer", func() {
		ctx := context.TODO()

		g.By("initializing test context")
		err := util.InitTest(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("creating PVC for archive storage")
		pvcName := "integration-test-disabled-" + util.RandomSuffix()
		_, err = util.CreateTestPVC(ctx, pvcName, "openshift-insights")
		o.Expect(err).NotTo(o.HaveOccurred())

		kubeClient := util.GetKubeClient()
		defer kubeClient.CoreV1().PersistentVolumeClaims("openshift-insights").Delete(ctx, pvcName, metav1.DeleteOptions{})

		g.By("creating DataGather with clusterconfig gatherer disabled")
		dg := &insightsv1.DataGather{
			ObjectMeta: metav1.ObjectMeta{
				Name: "integration-test-disabled-" + util.RandomSuffix(),
			},
			Spec: insightsv1.DataGatherSpec{
				Gatherers: insightsv1.Gatherers{
					Mode: insightsv1.GatheringModeCustom,
					Custom: insightsv1.Custom{
						Configs: []insightsv1.GathererConfig{
							{
								Name:  "clusterconfig",
								State: insightsv1.GathererStateDisabled,
							},
						},
					},
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
		finalDG, err := util.WaitForDataGatherCompletion(ctx, insightsClient, created.Name, 5*time.Minute)
		o.Expect(err).NotTo(o.HaveOccurred(), "DataGather should complete")

		g.By("verifying gathering succeeded")
		err = util.ValidateDataGatherSuccess(finalDG)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("reading archive from PVC")
		archive, err := util.ReadArchiveFromPVC(ctx, pvcName, "openshift-insights")
		o.Expect(err).NotTo(o.HaveOccurred())

		allFiles, err := util.ListArchiveContents(archive)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("verifying no clusterconfig data was gathered")
		configFiles := 0
		for _, file := range allFiles {
			if strings.HasPrefix(file, "config/") {
				configFiles++
				g.GinkgoWriter.Printf("Found unexpected config file: %s\n", file)
			}
		}
		o.Expect(configFiles).To(o.Equal(0), "should have no config/ files when clusterconfig gatherer is disabled")
	})

	g.It("validates enabling specific gatherer within disabled parent", func() {
		ctx := context.TODO()

		g.By("initializing test context")
		err := util.InitTest(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("creating PVC for archive storage")
		pvcName := "integration-test-selective-" + util.RandomSuffix()
		_, err = util.CreateTestPVC(ctx, pvcName, "openshift-insights")
		o.Expect(err).NotTo(o.HaveOccurred())

		kubeClient := util.GetKubeClient()
		defer kubeClient.CoreV1().PersistentVolumeClaims("openshift-insights").Delete(ctx, pvcName, metav1.DeleteOptions{})

		g.By("creating DataGather: disable clusterconfig but enable clusterconfig/nodes specifically")
		dg := &insightsv1.DataGather{
			ObjectMeta: metav1.ObjectMeta{
				Name: "integration-test-selective-" + util.RandomSuffix(),
			},
			Spec: insightsv1.DataGatherSpec{
				Gatherers: insightsv1.Gatherers{
					Mode: insightsv1.GatheringModeCustom,
					Custom: insightsv1.Custom{
						Configs: []insightsv1.GathererConfig{
							{
								Name:  "clusterconfig",
								State: insightsv1.GathererStateDisabled,
							},
							{
								Name:  "clusterconfig/nodes",
								State: insightsv1.GathererStateEnabled,
							},
						},
					},
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
		finalDG, err := util.WaitForDataGatherCompletion(ctx, insightsClient, created.Name, 5*time.Minute)
		o.Expect(err).NotTo(o.HaveOccurred(), "DataGather should complete")

		g.By("verifying gathering succeeded")
		err = util.ValidateDataGatherSuccess(finalDG)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("reading archive from PVC")
		archive, err := util.ReadArchiveFromPVC(ctx, pvcName, "openshift-insights")
		o.Expect(err).NotTo(o.HaveOccurred())

		allFiles, err := util.ListArchiveContents(archive)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("verifying nodes gatherer data is present")
		hasNodeData := false
		for _, file := range allFiles {
			if strings.Contains(file, "config/node/") {
				hasNodeData = true
				break
			}
		}
		o.Expect(hasNodeData).To(o.BeTrue(), "should have node data when specifically enabled")

		g.By("verifying other clusterconfig data is NOT present")
		hasOtherConfig := false
		for _, file := range allFiles {
			if strings.HasPrefix(file, "config/clusteroperator/") ||
			   strings.HasPrefix(file, "config/namespace/") {
				hasOtherConfig = true
				g.GinkgoWriter.Printf("Found unexpected config file: %s\n", file)
			}
		}
		o.Expect(hasOtherConfig).To(o.BeFalse(), "should not have other clusterconfig data when parent is disabled")
	})
})
