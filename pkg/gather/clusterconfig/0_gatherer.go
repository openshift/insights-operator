package clusterconfig

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"k8s.io/client-go/rest"
	"k8s.io/klog"

	_ "k8s.io/apimachinery/pkg/runtime/serializer/yaml"

	"github.com/openshift/insights-operator/pkg/record"
)

type gatherStatusReport struct {
	Name    string        `json:"name"`
	Elapsed time.Duration `json:"elapsed"`
	Report  int           `json:"report"`
	Errors  []error       `json:"errors"`
}

// Gatherer is a driving instance invoking collection of data
type Gatherer struct {
	ctx                     context.Context
	gatherKubeConfig        *rest.Config
	gatherProtoKubeConfig   *rest.Config
	metricsGatherKubeConfig *rest.Config
}

// New creates new Gatherer
func New(gatherKubeConfig *rest.Config, gatherProtoKubeConfig *rest.Config, metricsGatherKubeConfig *rest.Config) *Gatherer {
	return &Gatherer{
		gatherKubeConfig:        gatherKubeConfig,
		gatherProtoKubeConfig:   gatherProtoKubeConfig,
		metricsGatherKubeConfig: metricsGatherKubeConfig,
	}
}

// Gather is hosting and calling all the recording functions
func (g *Gatherer) Gather(ctx context.Context, recorder record.Interface) error {
	g.ctx = ctx
	bulkFns := []func() ([]record.Record, []error){
		GatherPodDisruptionBudgets(g),
		GatherMostRecentMetrics(g),
		GatherClusterOperators(g),
		GatherContainerImages(g),
		GatherNodes(g),
		GatherConfigMaps(g),
		GatherClusterVersion(g),
		GatherClusterID(g),
		GatherClusterInfrastructure(g),
		GatherClusterNetwork(g),
		GatherClusterAuthentication(g),
		GatherClusterImageRegistry(g),
		GatherClusterImagePruner(g),
		GatherClusterFeatureGates(g),
		GatherClusterOAuth(g),
		GatherClusterIngress(g),
		GatherClusterProxy(g),
		GatherCertificateSigningRequests(g),
		GatherCRD(g),
		GatherHostSubnet(g),
		GatherMachineSet(g),
		GatherInstallPlans(g),
		GatherServiceAccounts(g),
		GatherMachineConfigPool(g),
		GatherContainerRuntimeConfig(g),
		GatherStatefulSets(g),
		GatherNetNamespace(g),
	}

	var errors []string
	var gatherReport []interface{}
	for _, bulkFn := range bulkFns {
		gatherName := runtime.FuncForPC(reflect.ValueOf(bulkFn).Pointer()).Name()
		klog.V(5).Infof("Gathering %s", gatherName)

		start := time.Now()
		records, errs := bulkFn()
		elapsed := time.Now().Sub(start).Truncate(time.Millisecond)

		klog.V(4).Infof("Gather %s took %s to process %d records", gatherName, elapsed, len(records))
		gatherReport = append(gatherReport, gatherStatusReport{gatherName, elapsed, len(records), errs})

		for _, err := range errs {
			errors = append(errors, err.Error())
		}
		for _, record := range records {
			if err := recorder.Record(record); err != nil {
				errors = append(errors, fmt.Sprintf("unable to record %s: %v", record.Name, err))
				continue
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}

	// Creates the gathering performance report
	if err := recordGatherReport(recorder, gatherReport); err != nil {
		errors = append(errors, fmt.Sprintf("unable to record io status reports: %v", err))
	}

	if len(errors) > 0 {
		sort.Strings(errors)
		errors = uniqueStrings(errors)
		return fmt.Errorf("%s", strings.Join(errors, ", "))
	}
	return nil
}

func recordGatherReport(recorder record.Interface, report []interface{}) error {
	r := record.Record{Name: "insights-operator/gathers", Item: record.JSONMarshaller{Object: report}}
	return recorder.Record(r)
}

func uniqueStrings(arr []string) []string {
	var last int
	for i := 1; i < len(arr); i++ {
		if arr[i] == arr[last] {
			continue
		}
		last++
		if last != i {
			arr[last] = arr[i]
		}
	}
	if last < len(arr) {
		last++
	}
	return arr[:last]
}
