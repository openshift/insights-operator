# Integration Test Suite Implementation Plan

## Context

The insights-operator currently has comprehensive unit tests using standard Go testing with testify/assert, but lacks integration tests that validate end-to-end workflows with real Kubernetes API interactions. This plan establishes a complete integration test infrastructure using the **OpenShift Tests Extension (OTE)** framework with Ginkgo, following the official OpenShift integration testing standards.

**Why this change is needed:**
- Unit tests with fake clients cannot catch real Kubernetes API interaction issues
- Need to validate complete DataGather CR lifecycle with actual condition transitions
- Controller reconciliation loops require testing with real API server watches and informers
- Gatherer content validation requires end-to-end gathering execution against real cluster resources
- OpenShift operator standards require specific condition management patterns that need integration testing
- Follow OpenShift's standardized OTE framework for consistency with other OpenShift components

**Current state:**
- Working on `integration-tests-poc` branch
- Empty `test/` directory (no existing integration tests)
- 3 main gatherers: ClusterConfig (69 functions), Workloads (2 functions), Conditional (5 types)
- 5 controllers: Periodic, Status, SCA, Cluster Transfer, Runtime-Extractor
- All tests use fake clients with testify/assert (no Ginkgo usage)
- Have "Integration Guide for OpenShift Tests Extension.pdf" in repo root

**Test environment (OTE framework):**
- Tests built as a standalone OTE binary that integrates with OpenShift CI
- Tests run against real OpenShift clusters (provisioned on-demand via OpenShift CI)
- Binary runs locally or in CI, connects via kubeconfig to cluster under test
- Uses `github.com/openshift-eng/openshift-tests-extension` framework
- Tests use Ginkgo v2 with environment selectors (CEL expressions) for conditional execution
- Organized into test suites (e.g., "openshift/conformance/parallel")

## Implementation Approach

### Phase 1: Foundation Setup (OTE Framework)

**1.1 Add OTE Dependencies**

Add to `go.mod`:
```go
require (
    github.com/openshift-eng/openshift-tests-extension v0.0.0-<latest>
    github.com/onsi/ginkgo/v2 v2.15.0
    k8s.io/kubernetes/test/e2e/framework v0.0.0-<version>
    // gomega already present at v1.39.1
)
```

Reference: Vendor `github.com/openshift-eng/openshift-tests-extension` as shown in PDF

**1.2 Create OTE Binary Structure**

Following the OTE guide, create:
```
cmd/
└── insights-operator-tests/        # OTE binary entry point
    └── main.go                     # Cobra CLI with OTE subcommands

test/
└── extended/                       # OTE test directory
    ├── util/                       # Test helpers  
    │   ├── cluster.go             # Resource creation helpers
    │   ├── assertions.go          # Custom Gomega matchers
    │   └── conditions.go          # Condition checking utilities
    ├── gatherers/
    │   └── nodes_test.go          # Start with one gatherer test
    └── controllers/
        └── periodic_test.go        # Start with one controller test
```

**1.3 Implement OTE Binary Entry Point**

Create `cmd/insights-operator-tests/main.go`:
```go
package main

import (
    "github.com/openshift-eng/openshift-tests-extension/pkg/cmd"
    "github.com/openshift/insights-operator/test/extended/util"
)

func main() {
    // Register OTE subcommands (info, list, run-test, run-suite)
    rootCmd := cmd.DefaultExtensionCommands(util.Registry)
    rootCmd.Execute()
}
```

Create `test/extended/util/init.go`:
```go
package util

import (
    "github.com/openshift-eng/openshift-tests-extension/pkg/extension"
    "github.com/openshift/insights-operator/test/extended/gatherers"
    "github.com/openshift/insights-operator/test/extended/controllers"
)

var Registry = extension.NewExtension("insights-operator", "openshift")

func init() {
    // Use BuildExtensionTestSpecsFromOpenShiftGinkoSuite() to auto-register
    // Ginkgo tests from imported packages
    extension.BuildExtensionTestSpecsFromOpenShiftGinkoSuite()(Registry)
}
```

**1.4 Initialize Test Context (compat_otp pattern)**

Create `test/extended/util/test_context.go`:
```go
package util

import (
    "context"
    compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
    e2e "k8s.io/kubernetes/test/e2e/framework"
)

// InitTest initializes test context from kubeconfig
func InitTest(ctx context.Context) error {
    if err := compat_otp.InitTest(ctx, false); err != nil {
        return err
    }
    e2e.AfterReadingAllFlags(compat_otp.TestContext)
    return nil
}
```

This uses the `compat_otp` library from OpenShift origin to read kubeconfig and create clients.

**1.5 Create Test Helpers**

`test/extended/util/cluster.go`:
- `GetExistingNodes()` - Read existing cluster nodes (for gatherer validation tests)
- `CreateTestNamespace()` - Generate unique namespace with cleanup (for controller tests that need isolation)
- `WaitForResourceDeletion()` - Poll until resource is deleted
- Helper functions for creating test resources when needed

`test/extended/util/assertions.go`:
- `HaveCondition(type, status, reason)` - Gomega matcher for conditions
- `MatchDataGatherPhase(phase)` - Match DataGather status
- `ContainGatheredRecord(name)` - Verify gatherer output

`test/extended/util/conditions.go`:
- `GetCondition(obj, type)` - Extract condition from status
- `WaitForCondition(getter, type, status, timeout)` - Poll for condition

**1.6 Update Makefile**

Add OTE binary build and test targets:
```makefile
## --------------------------------------
## Integration Tests (OTE Framework)
## --------------------------------------

OTE_BINARY := bin/insights-operator-tests

.PHONY: build-ote
build-ote: ## Build OTE test binary
	CGO_ENABLED=0 go build -o $(OTE_BINARY) ./cmd/insights-operator-tests

.PHONY: integration-test
integration-test: build-ote ## Run integration tests against cluster (requires KUBECONFIG)
	@if [ -z "$$KUBECONFIG" ]; then \
		echo "Error: KUBECONFIG environment variable must be set"; \
		exit 1; \
	fi
	$(OTE_BINARY) run-suite insights-operator/all

.PHONY: integration-test-list
integration-test-list: build-ote ## List available integration tests
	$(OTE_BINARY) list

.PHONY: integration-test-info
integration-test-info: build-ote ## Show OTE binary information
	$(OTE_BINARY) info

.PHONY: integration-test-run
integration-test-run: build-ote ## Run specific test (use TEST="test name")
	@if [ -z "$$KUBECONFIG" ]; then \
		echo "Error: KUBECONFIG environment variable must be set"; \
		exit 1; \
	fi
	$(OTE_BINARY) run-test -n "$(TEST)"
```

### Phase 2: Initial Test Implementation (One Simple Test)

**2.1 Implement First Integration Test (OTE Pattern with DataGather CR)**

Create `test/extended/gatherers/nodes_test.go`:

```go
package gatherers

import (
    "archive/tar"
    "compress/gzip"
    "context"
    "encoding/json"
    "io"
    "time"
    
    g "github.com/onsi/ginkgo/v2"
    o "github.com/onsi/gomega"
    ote "github.com/openshift-eng/openshift-tests-extension/pkg/ginkgo"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    
    insightsv1 "github.com/openshift/api/insights/v1"
    "github.com/openshift/insights-operator/test/extended/util"
)

var _ = g.Describe("[sig-insights] Gatherer Content Validation", func() {
    g.AddLabel("[Jira:Insights]")
    g.AddLabel("[TOTP]")
    
    defer g.GinkgoRecover()
    
    ote.Select(g.NameContains("validates node gathering")).
        Exclude(ote.ExternalConnectivityEquals("Disconnected"))
    
    g.It("validates node gathering in archive", func() {
        ctx := context.TODO()
        
        g.By("initializing test context")
        err := util.InitTest(ctx)
        o.Expect(err).NotTo(o.HaveOccurred())
        
        g.By("creating PVC for archive storage in openshift-insights namespace")
        pvcName := "integration-test-pvc-" + util.RandomSuffix()
        pvc := &corev1.PersistentVolumeClaim{
            ObjectMeta: metav1.ObjectMeta{
                Name:      pvcName,
                Namespace: "openshift-insights",
            },
            Spec: corev1.PersistentVolumeClaimSpec{
                AccessModes: []corev1.PersistentVolumeAccessMode{
                    corev1.ReadWriteOnce,
                },
                Resources: corev1.ResourceRequirements{
                    Requests: corev1.ResourceList{
                        corev1.ResourceStorage: resource.MustParse("1Gi"),
                    },
                },
            },
        }
        
        kubeClient := util.GetKubeClient()
        _, err := kubeClient.CoreV1().PersistentVolumeClaims("openshift-insights").Create(ctx, pvc, metav1.CreateOptions{})
        o.Expect(err).NotTo(o.HaveOccurred())
        defer kubeClient.CoreV1().PersistentVolumeClaims("openshift-insights").Delete(ctx, pvcName, metav1.DeleteOptions{})
        
        g.By("creating DataGather CR with PersistentVolume storage")
        dg := &insightsv1.DataGather{
            ObjectMeta: metav1.ObjectMeta{
                Name: "integration-test-nodes-" + util.RandomSuffix(),
            },
            Spec: insightsv1.DataGatherSpec{
                Gatherers: insightsv1.DataGatherGatherersSpec{
                    Mode: insightsv1.GatherAll,
                },
                Storage: &insightsv1.DataGatherStorageSpec{
                    Mode: insightsv1.PersistentVolume,
                    PersistentVolumeName: pvcName,  // Reference the PVC we created
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
        nodeFiles := util.ExtractFilesMatching(archive, "config/node/")
        o.Expect(nodeFiles).NotTo(o.BeEmpty(), "archive should contain node data")
        
        g.By("validating node data structure")
        for filename, content := range nodeFiles {
            var node corev1.Node
            err = json.Unmarshal(content, &node)
            o.Expect(err).NotTo(o.HaveOccurred(), "node file %s should be valid JSON", filename)
            o.Expect(node.Name).NotTo(o.BeEmpty())
            o.Expect(node.Status.Capacity).NotTo(o.BeEmpty())
        }
    })
})
```

**Key implementation details (End-to-End Testing):**
- **Test the production workflow**: Create DataGather CR → Wait for completion → Validate archive
- OTE binary runs locally, connects to cluster via kubeconfig
- Use `ote.Select()` with environment selectors for conditional execution
- Add labels for ownership (`[Jira:Insights]`) and tracking (`[TOTP]`)
- Tests validate the **complete gathering pipeline**, not individual functions
- Use `o.Eventually()` with appropriate timeout (5min for full gathering)
- Archive reading requires access to insights-operator pod filesystem or PV

**2.2 Archive Reading Helpers**

Create helper in `test/extended/util/archive.go`:

```go
package util

import (
    "archive/tar"
    "bytes"
    "compress/gzip"
    "context"
    "fmt"
    "io"
    "strings"
    "time"
    
    insightsv1 "github.com/openshift/api/insights/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    compat_otp "github.com/openshift/origin/test/extended/util/compat_otp"
)

// Note: PVC is pre-created by the test before creating DataGather CR
// The PVC name is passed to DataGather spec, not extracted from status

// ReadArchiveFromPVC reads archive by mounting PVC to a test pod
func ReadArchiveFromPVC(ctx context.Context, pvcName, namespace string) ([]byte, error) {
    kubeClient := compat_otp.TestContext.AdminKubeClient
    
    // Create test pod with PVC mounted
    podName := "test-archive-reader-" + RandomSuffix()
    pod := &corev1.Pod{
        ObjectMeta: metav1.ObjectMeta{
            Name:      podName,
            Namespace: namespace,
        },
        Spec: corev1.PodSpec{
            Containers: []corev1.Container{
                {
                    Name:    "reader",
                    Image:   "registry.redhat.io/ubi8/ubi-minimal:latest",
                    Command: []string{"sleep", "3600"},
                    VolumeMounts: []corev1.VolumeMount{
                        {
                            Name:      "archive-volume",
                            MountPath: "/archive",
                        },
                    },
                },
            },
            Volumes: []corev1.Volume{
                {
                    Name: "archive-volume",
                    VolumeSource: corev1.VolumeSource{
                        PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
                            ClaimName: pvcName,
                        },
                    },
                },
            },
            RestartPolicy: corev1.RestartPolicyNever,
        },
    }
    
    created, err := kubeClient.CoreV1().Pods(namespace).Create(ctx, pod, metav1.CreateOptions{})
    if err != nil {
        return nil, err
    }
    defer kubeClient.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{})
    
    // Wait for pod to be running
    err = waitForPodRunning(ctx, kubeClient, namespace, podName, 2*time.Minute)
    if err != nil {
        return nil, err
    }
    
    // Read archive from standardized path: /archive/insights-<timestamp>.tar.gz
    // Use kubectl exec equivalent to cat the file
    archiveData, err := execInPod(ctx, kubeClient, namespace, podName, "reader",
        []string{"sh", "-c", "cat /archive/*.tar.gz"})
    if err != nil {
        return nil, err
    }
    
    return archiveData, nil
}

// ExtractFilesMatching extracts files from tar.gz matching pattern
func ExtractFilesMatching(archiveData []byte, pattern string) (map[string][]byte, error) {
    gzipReader, err := gzip.NewReader(bytes.NewReader(archiveData))
    if err != nil {
        return nil, err
    }
    defer gzipReader.Close()
    
    tarReader := tar.NewReader(gzipReader)
    files := make(map[string][]byte)
    
    for {
        header, err := tarReader.Next()
        if err == io.EOF {
            break
        }
        if err != nil {
            return nil, err
        }
        
        if strings.Contains(header.Name, pattern) {
            content, _ := io.ReadAll(tarReader)
            files[header.Name] = content
        }
    }
    
    return files, nil
}
```

### Phase 3: Define Test Suites and Register Tests

**3.1 Create Test Suite Definition**

Following OTE framework, tests must be organized into suites. Create suite definitions in util:

`test/extended/util/suites.go`:
```go
package util

import (
    "github.com/openshift-eng/openshift-tests-extension/pkg/extension"
)

func init() {
    // Define custom test suite for all insights-operator tests
    Registry.AddSuite(extension.Suite{
        Name: "insights-operator/all",
        Description: "All insights-operator integration tests",
    })
    
    // Define gatherer-specific suite
    Registry.AddSuite(extension.Suite{
        Name: "insights-operator/gatherers",
        Description: "Gatherer content validation tests",
    })
}
```

**3.2 Expand Test Coverage (Future Work)**

After the initial test works, add more archive validation tests following the same pattern:

**Gatherer Content Tests** (create DataGather → validate archive contains expected data):
1. **Cluster Operators** - Validate `config/clusteroperator/` files
2. **Config Maps** - Validate `config/configmaps/` files
3. **Machines** - Validate `config/machine/` files
4. **CRDs** - Validate `config/crd/` files
5. **Workloads** - Validate `config/workloads/` files
6. **Conditional Gatherers** - Create alert, trigger conditional gathering, validate logs

**Controller Feature Tests** (test specific controller behaviors):
1. **SCA Certificates** - Verify entitlement secrets are created
2. **Cluster Transfer** - Test pull-secret update workflow
3. **Runtime Extractor** - Verify DaemonSet lifecycle
4. **Status Conditions** - Validate ClusterOperator status updates

Each test follows the pattern:
- Create necessary CRs/resources
- Wait for expected outcome (Eventually with timeout)
- Validate results (archive content, CR status, cluster state)
- Clean up created resources

## Critical Files to Create/Modify

1. **Makefile** - Add OTE binary build and integration-test targets
2. **go.mod** - Add OTE framework and Ginkgo v2 dependencies
3. **cmd/insights-operator-tests/main.go** - NEW: OTE binary entry point with Cobra CLI
4. **test/extended/util/init.go** - NEW: OTE extension registry and initialization
5. **test/extended/util/test_context.go** - NEW: Test context initialization using compat_otp
6. **test/extended/util/gatherers.go** - NEW: Gatherer construction helpers
7. **test/extended/util/suites.go** - NEW: Test suite definitions
8. **test/extended/util/archive.go** - NEW: Archive reading and extraction helpers
9. **test/extended/gatherers/nodes_test.go** - NEW: First integration test (DataGather workflow + archive validation)

## Verification Plan

After implementation:

1. **Build OTE binary:**
   ```bash
   make build-ote
   # Creates bin/insights-operator-tests
   ```

2. **Verify binary works:**
   ```bash
   bin/insights-operator-tests info
   # Should show: product=insights-operator, type=openshift
   
   bin/insights-operator-tests list
   # Should list available tests and suites
   ```

3. **Set up cluster access:**
   ```bash
   # For OpenShift CI - KUBECONFIG is provided automatically
   # For local testing with CRC:
   export KUBECONFIG=~/.crc/machines/crc/kubeconfig
   ```

4. **Run integration tests:**
   ```bash
   make integration-test
   # Runs: bin/insights-operator-tests run-suite insights-operator/all
   ```

5. **Run specific test:**
   ```bash
   make integration-test-run TEST="gathers node information"
   ```

6. **Verify test output:**
   - OTE shows test execution with Ginkgo verbose output
   - Test should pass with green checkmark
   - Output shows cluster connection and gatherer execution
   - JSON output available for CI integration

7. **Verify cleanup:**
   - No leftover test resources in cluster
   - Test runs are idempotent (can run multiple times)

## Future Expansion (Not in Initial Scope)

After the foundation is complete, expand coverage to:

- **All 69 ClusterConfig gatherers** - Full content validation
- **Workloads and Conditional gatherers** - Complete gatherer coverage
- **All 5 controllers** - SCA, Cluster Transfer, Runtime-Extractor, Status
- **End-to-end workflows** - Complete DataGather lifecycle
- **Condition management** - All OpenShift operator conditions
- **Error handling** - Failure scenarios and retries
- **Concurrent operations** - Parallel controller execution

## Notes

- **OTE Framework**: Following official OpenShift Tests Extension framework (see PDF in repo root)
- **Test binary**: Built as standalone binary `insights-operator-tests` with Cobra CLI subcommands
- **Real clusters**: Tests run against actual OpenShift clusters provisioned on-demand
- **Test execution model**: OTE binary runs **locally** or in CI, connects via kubeconfig
- **KUBECONFIG required**: Binary reads kubeconfig automatically via compat_otp library
- **Cluster compatibility**: Works with OpenShift CI clusters, CRC, or any OpenShift cluster
- **Environment selectors**: Use CEL expressions to conditionally run tests (platform, topology, network stack, etc.)
- **Test organization**:
  - Use `[sig-insights]` descriptor for all tests
  - Add ownership labels: `[Jira:Insights]`
  - Add tracking annotation: `[TOTP]`
- **Test naming**: Must be stable across runs (no dynamic content like pod UIDs, timestamps)
- **Suites**: Tests grouped into suites (`insights-operator/all`, `insights-operator/gatherers`)
- **CI integration**: OTE binaries integrate with OpenShift CI via standard test infrastructure
- **No custom CI jobs needed**: Tests automatically run in existing CI jobs (e.g., e2e-aws, e2e-gcp)
- **Gather tests**: Read existing cluster state, no resource creation needed for most tests
- **Following OpenShift standards**: Based on official OTE integration guide and compat_otp patterns from openshift/origin
