package gathering

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"
	configv1 "github.com/openshift/api/config/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilrand "k8s.io/apimachinery/pkg/util/rand"

	testutil "github.com/openshift/insights-operator/tests-extension/test/util"
)

// versionFilePath is where the "config/version" record lands on disk inside
// the archive (see pkg/gatherers/clusterconfig/gather_cluster_version.go and
// docs/insights-archive-sample/config/version.json).
const versionFilePath = "config/version.json"

var _ = g.Describe("[sig-insights][Feature:Gathering] on-demand DataGather with PersistentVolume storage", func() {
	g.It("gathers cluster data and writes a readable archive to the configured PVC", func() {
		ctx := context.Background()

		cfg, err := testutil.NewConfig()
		o.Expect(err).NotTo(o.HaveOccurred())

		clients, err := testutil.NewClients(cfg)
		o.Expect(err).NotTo(o.HaveOccurred())

		// Both names are suffixed with a random string so a leftover PVC/pod
		// from a previous (e.g. interrupted) run never collides with this one.
		runSuffix := utilrand.String(5)

		pvcName := "test-datagather-pvc-" + runSuffix
		_, err = testutil.CreatePVC(ctx, clients.Kube, pvcName, "1Gi")
		o.Expect(err).NotTo(o.HaveOccurred())
		g.DeferCleanup(func(ctx context.Context) {
			o.Expect(testutil.DeletePVC(ctx, clients.Kube, pvcName)).To(o.Succeed())
		})

		dg, err := testutil.CreatePVCDataGather(ctx, clients.Insights, pvcName)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.DeferCleanup(func(ctx context.Context) {
			o.Expect(testutil.DeleteDataGather(ctx, clients.Insights, dg.Name)).To(o.Succeed())
		})
		// Delete the Job explicitly so the pod releases the PVC
		// before the PVC cleanup below runs.
		g.DeferCleanup(func(ctx context.Context) {
			o.Expect(testutil.DeleteGatheringJob(ctx, clients.Kube, dg.Name)).To(o.Succeed())
		})

		// Wait for the gathering job itself to finish. Only once the pod
		// terminates is it safe for another pod to mount the same
		// ReadWriteOnce PVC to read the archive back; CreateArchiveReaderPod
		// requires this Job to enforce that ordering.
		gatheringJob, err := testutil.WaitForGatheringJobComplete(ctx, clients.Kube, dg.Name, 10*time.Minute)
		o.Expect(err).NotTo(o.HaveOccurred())

		dg, err = testutil.WaitForDataGatherRecorded(ctx, clients.Insights, dg.Name, 10*time.Minute)
		o.Expect(err).NotTo(o.HaveOccurred())

		o.Expect(dg.Status.StartTime).NotTo(o.BeNil(), "StartTime should be set")

		_, err = testutil.WaitForPVCBound(ctx, clients.Kube, pvcName, 2*time.Minute)
		o.Expect(err).NotTo(o.HaveOccurred())

		readerPodName := "test-archive-reader-" + runSuffix
		_, err = testutil.CreateArchiveReaderPod(
			ctx, clients.Kube, cfg, gatheringJob, readerPodName, pvcName, testutil.DefaultPVCMountPath, 2*time.Minute,
		)
		o.Expect(err).NotTo(o.HaveOccurred())
		g.DeferCleanup(func(ctx context.Context) {
			o.Expect(testutil.DeleteArchiveReaderPod(ctx, clients.Kube, readerPodName)).To(o.Succeed())
		})

		archiveName, err := testutil.FindNewestArchive(ctx, clients.Kube, cfg, readerPodName, testutil.DefaultPVCMountPath)
		o.Expect(err).NotTo(o.HaveOccurred())

		archivePath := fmt.Sprintf("%s/%s", testutil.DefaultPVCMountPath, archiveName)
		archiveBytes, err := testutil.ReadFileFromPod(ctx, clients.Kube, cfg, readerPodName, archivePath, 2*time.Minute)
		o.Expect(err).NotTo(o.HaveOccurred())

		archive, err := testutil.OpenArchive(archiveBytes)
		o.Expect(err).NotTo(o.HaveOccurred())

		o.Expect(archive.Files()).NotTo(o.BeEmpty(), "expected the archive to contain files")

		versionContent, found := archive.FindFile(versionFilePath)
		o.Expect(found).To(o.BeTrue(), "expected %s to be present in the archive, files: %v", versionFilePath, archive.Files())

		var archivedClusterVersion configv1.ClusterVersion
		o.Expect(json.Unmarshal(versionContent, &archivedClusterVersion)).To(o.Succeed())
		o.Expect(archivedClusterVersion.Spec.ClusterID).NotTo(o.BeEmpty())

		liveClusterVersion, err := clients.Config.ConfigV1().ClusterVersions().Get(ctx, "version", metav1.GetOptions{})
		o.Expect(err).NotTo(o.HaveOccurred())

		o.Expect(string(archivedClusterVersion.Spec.ClusterID)).To(o.Equal(string(liveClusterVersion.Spec.ClusterID)),
			"cluster ID in the archive should match the live ClusterVersion, proving the archive holds real data")
	})
})
