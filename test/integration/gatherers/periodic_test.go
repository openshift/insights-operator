package gatherers

import (
	"context"
	"fmt"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	insightsv1 "github.com/openshift/api/insights/v1"
	"github.com/openshift/insights-operator/test/integration/util"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = g.Describe("[sig-insights] Periodic Data Gathering", func() {
	defer g.GinkgoRecover()

	g.It("validates periodic gathering spawns DataGather CRs at configured intervals", func() {
		ctx := context.TODO()

		g.By("initializing test context")
		err := util.InitTest(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())

		kubeClient := util.GetKubeClient()
		insightsClient := util.GetInsightsClient()

		// Store original config to restore later
		g.By("backing up original insights-config ConfigMap")
		originalConfig, err := kubeClient.CoreV1().ConfigMaps("openshift-insights").Get(ctx, "insights-config", metav1.GetOptions{})
		configExists := err == nil
		var originalData map[string]string
		if configExists {
			originalData = make(map[string]string)
			for k, v := range originalConfig.Data {
				originalData[k] = v
			}
		}

		// Restore original config at the end
		defer func() {
			g.By("restoring original insights-config ConfigMap")
			if configExists && originalData != nil {
				originalConfig.Data = originalData
				_, err := kubeClient.CoreV1().ConfigMaps("openshift-insights").Update(ctx, originalConfig, metav1.UpdateOptions{})
				if err != nil {
					g.GinkgoWriter.Printf("Warning: failed to restore original config: %v\n", err)
				}
			}
		}()

		g.By("configuring periodic gathering interval to 2 minutes (minimum is 10m, we'll use 10m)")
		configData := map[string]string{
			"config.yaml": `dataReporting:
  interval: 10m
`,
		}

		if configExists {
			// Update existing ConfigMap
			originalConfig.Data = configData
			_, err = kubeClient.CoreV1().ConfigMaps("openshift-insights").Update(ctx, originalConfig, metav1.UpdateOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			// Create new ConfigMap
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "insights-config",
					Namespace: "openshift-insights",
				},
				Data: configData,
			}
			_, err = kubeClient.CoreV1().ConfigMaps("openshift-insights").Create(ctx, cm, metav1.CreateOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())
			defer kubeClient.CoreV1().ConfigMaps("openshift-insights").Delete(ctx, "insights-config", metav1.DeleteOptions{})
		}

		g.By("waiting for insights-operator to reload config (30 seconds)")
		time.Sleep(30 * time.Second)

		g.By("counting existing DataGathers before waiting for periodic gather")
		initialDGs, err := insightsClient.InsightsV1().DataGathers().List(ctx, metav1.ListOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())
		initialCount := len(initialDGs.Items)
		g.GinkgoWriter.Printf("Initial DataGather count: %d\n", initialCount)

		// List initial DGs for reference
		if initialCount > 0 {
			g.GinkgoWriter.Printf("Existing DataGathers:\n")
			for _, dg := range initialDGs.Items {
				g.GinkgoWriter.Printf("  - %s (created: %s)\n", dg.Name, dg.CreationTimestamp)
			}
		}

		g.By("waiting up to 12 minutes for a new periodic DataGather to be created")
		var newDG *insightsv1.DataGather
		o.Eventually(func() bool {
			dgs, err := insightsClient.InsightsV1().DataGathers().List(ctx, metav1.ListOptions{})
			if err != nil {
				g.GinkgoWriter.Printf("Error listing DataGathers: %v\n", err)
				return false
			}

			// Look for new DataGathers created after we started the test
			for i := range dgs.Items {
				dg := &dgs.Items[i]
				// Check if this is a new DG (not in initial list)
				isNew := true
				for _, initialDG := range initialDGs.Items {
					if dg.Name == initialDG.Name {
						isNew = false
						break
					}
				}

				if isNew {
					// Found a new DataGather!
					newDG = dg
					g.GinkgoWriter.Printf("Found new periodic DataGather: %s (created: %s)\n", dg.Name, dg.CreationTimestamp)
					return true
				}
			}

			currentCount := len(dgs.Items)
			if currentCount != initialCount {
				g.GinkgoWriter.Printf("DataGather count changed: %d -> %d (checking if new...)\n", initialCount, currentCount)
			}
			return false
		}, 12*time.Minute, 30*time.Second).Should(o.BeTrue(), "A new periodic DataGather should be created within 12 minutes")

		o.Expect(newDG).NotTo(o.BeNil(), "Should have found a new DataGather")

		g.By("verifying the periodic DataGather has correct configuration")
		// Periodic DataGathers are created automatically by the operator
		// They should have the default spec (all gatherers enabled)
		o.Expect(newDG.Spec.Gatherers.Mode).To(o.Equal(insightsv1.GatheringModeAll), "Periodic DataGather should have mode=All")

		g.By("waiting for the periodic DataGather to complete")
		finalDG, err := util.WaitForDataGatherCompletion(ctx, insightsClient, newDG.Name, 5*time.Minute)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("verifying the periodic gathering succeeded")
		err = util.ValidateDataGatherSuccess(finalDG)
		o.Expect(err).NotTo(o.HaveOccurred())

		g.By("verifying gatherer status was populated")
		o.Expect(finalDG.Status.Gatherers).NotTo(o.BeEmpty(), "Periodic DataGather should have gatherer status")
		g.GinkgoWriter.Printf("Periodic DataGather completed with %d gatherers\n", len(finalDG.Status.Gatherers))

		// Print some gatherer info for debugging
		for _, gatherer := range finalDG.Status.Gatherers {
			g.GinkgoWriter.Printf("  - %s: %v\n", gatherer.Name, gatherer.Conditions)
		}
	})

	g.It("validates multiple periodic gathers are created over time", func() {
		ctx := context.TODO()

		g.By("initializing test context")
		err := util.InitTest(ctx)
		o.Expect(err).NotTo(o.HaveOccurred())

		kubeClient := util.GetKubeClient()
		insightsClient := util.GetInsightsClient()

		g.By("backing up original insights-config ConfigMap")
		originalConfig, err := kubeClient.CoreV1().ConfigMaps("openshift-insights").Get(ctx, "insights-config", metav1.GetOptions{})
		configExists := err == nil
		var originalData map[string]string
		if configExists {
			originalData = make(map[string]string)
			for k, v := range originalConfig.Data {
				originalData[k] = v
			}
		}

		defer func() {
			g.By("restoring original insights-config ConfigMap")
			if configExists && originalData != nil {
				originalConfig.Data = originalData
				_, err := kubeClient.CoreV1().ConfigMaps("openshift-insights").Update(ctx, originalConfig, metav1.UpdateOptions{})
				if err != nil {
					g.GinkgoWriter.Printf("Warning: failed to restore original config: %v\n", err)
				}
			}
		}()

		g.By("configuring periodic gathering interval to 10 minutes (minimum allowed)")
		configData := map[string]string{
			"config.yaml": `dataReporting:
  interval: 10m
`,
		}

		if configExists {
			originalConfig.Data = configData
			_, err = kubeClient.CoreV1().ConfigMaps("openshift-insights").Update(ctx, originalConfig, metav1.UpdateOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())
		} else {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "insights-config",
					Namespace: "openshift-insights",
				},
				Data: configData,
			}
			_, err = kubeClient.CoreV1().ConfigMaps("openshift-insights").Create(ctx, cm, metav1.CreateOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())
			defer kubeClient.CoreV1().ConfigMaps("openshift-insights").Delete(ctx, "insights-config", metav1.DeleteOptions{})
		}

		g.By("waiting for insights-operator to reload config")
		time.Sleep(30 * time.Second)

		g.By("recording timestamp")
		startTime := time.Now()

		g.By(fmt.Sprintf("waiting up to 25 minutes for at least 2 new periodic DataGathers to be created (interval is 10m)"))
		var newDGCount int
		o.Eventually(func() bool {
			dgs, err := insightsClient.InsightsV1().DataGathers().List(ctx, metav1.ListOptions{})
			if err != nil {
				return false
			}

			// Count new DataGathers created after start time
			newDGCount = 0
			for i := range dgs.Items {
				dg := &dgs.Items[i]
				if dg.CreationTimestamp.After(startTime) {
					newDGCount++
				}
			}

			g.GinkgoWriter.Printf("New periodic DataGathers created: %d/2 (elapsed: %s)\n", newDGCount, time.Since(startTime).Round(time.Second))
			return newDGCount >= 2
		}, 25*time.Minute, 1*time.Minute).Should(o.BeTrue(), "At least 2 periodic DataGathers should be created in 25 minutes with 10m interval")

		g.GinkgoWriter.Printf("Successfully detected %d periodic DataGathers created over %s\n", newDGCount, time.Since(startTime).Round(time.Second))
	})
})
