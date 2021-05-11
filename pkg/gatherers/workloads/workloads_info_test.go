package workloads

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"testing"
	"time"

	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/openshift/insights-operator/pkg/record"
)

//nolint: funlen, gocyclo, gosec
func Test_gatherWorkloadInfo(t *testing.T) {
	if len(os.Getenv("TEST_INTEGRATION")) == 0 {
		t.Skip("will not run unless TEST_INTEGRATION is set, and requires KUBECONFIG to point to a real cluster")
	}

	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	).ClientConfig()
	if err != nil {
		t.Fatal(err)
	}
	config.AcceptContentTypes = "application/vnd.kubernetes.protobuf,application/json"
	config.ContentType = "application/vnd.kubernetes.protobuf"

	g := New(config)
	ctx := context.TODO()
	start := time.Now()
	records, errs := g.GatherWorkloadInfo(ctx)
	if len(errs) > 0 {
		t.Fatal(errs)
	}

	t.Logf("Gathered in %s", time.Now().Sub(start).Round(time.Second).String())

	if len(records) != 1 {
		t.Fatalf("unexpected: %v", records)
	}
	for _, r := range records {
		out, err := json.MarshalIndent(r.Item.(record.JSONMarshaller).Object, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = ioutil.WriteFile("../../../docs/insights-archive-sample/config/workload_info.json", out, 0750); err != nil {
			t.Fatal(err)
		}

		out, err = json.Marshal(r.Item)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		if _, err := gw.Write(out); err != nil {
			t.Fatal(err)
		}
		if err := gw.Close(); err != nil {
			t.Fatal(err)
		}

		images := make(map[string]struct{})

		var total, totalTerminal, totalIgnored, totalInvalid int
		pods := r.Item.(record.JSONMarshaller).Object.(*workloadPods)
		for ns, pods := range pods.Namespaces {
			var count int
			for i, pod := range pods.Shapes {
				count += pod.Duplicates + 1
				if len(pod.Containers) == 0 {
					t.Errorf("%s.Shapes[%d] should not have a shape with empty containers: %#v", ns, i, pod)
				}
				for j, container := range pod.InitContainers {
					if len(container.ImageID) == 0 {
						t.Errorf("%s.Shapes[%d].InitContainers[%d] should have an imageID: %#v", ns, i, j, pod)
					}
					images[container.ImageID] = struct{}{}
				}
				for j, container := range pod.Containers {
					if len(container.ImageID) == 0 {
						t.Errorf("%s.Shapes[%d].Containers[%d] should have an imageID: %#v", ns, i, j, pod)
					}
					images[container.ImageID] = struct{}{}
				}
			}
			if (count + pods.TerminalCount + pods.InvalidCount + pods.IgnoredCount) != pods.Count {
				t.Errorf("%s had mismatched count of pods", ns)
			}
			total += pods.Count
			totalTerminal += pods.TerminalCount
			totalIgnored += pods.IgnoredCount
			totalInvalid += pods.InvalidCount
		}
		if pods.PodCount != total {
			t.Errorf("mismatched pod count %d vs %d", pods.PodCount, total)
		}

		var totalImagesWithData int
		for imageID, image := range pods.Images {
			totalImagesWithData++
			if len(image.LayerIDs) == 0 {
				t.Errorf("found empty layer IDs in image %s", imageID)
			}
		}
		if pods.ImageCount != len(images) {
			t.Errorf("total image count did not match counted images %d vs %d", pods.ImageCount, len(images))
		}
		if totalImagesWithData > pods.ImageCount {
			t.Errorf("found more images than exist %d vs %d", totalImagesWithData, pods.ImageCount)
		}

		t.Logf(`
  uncompressed: %10d bytes
    compressed: %10d bytes (%.1f%%)

    namespaces: %5d

          pods: %5d
      terminal: %5d (%.1f%%)
       ignored: %5d (%.1f%%)
       invalid: %5d (%.1f%%)

        images: %5d
        w/data: %5d (%.1f%%)
        cached: %5d
`,
			len(out),
			buf.Len(),
			float64(buf.Len())/float64(len(out))*100,
			len(pods.Namespaces),
			total,
			totalTerminal,
			float64(totalTerminal)/float64(total)*100,
			totalIgnored,
			float64(totalIgnored)/float64(total)*100,
			totalInvalid,
			float64(totalInvalid)/float64(total)*100,
			pods.ImageCount,
			totalImagesWithData,
			float64(totalImagesWithData)/float64(pods.ImageCount)*100,
			workloadImageLRU.Len(),
		)
	}
}
