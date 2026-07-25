# Integration Tests - Implementation Notes

## Simplified Testing Approach (Per User Feedback)

### Key Optimization: Shared Archive Strategy

Instead of running a new `DataGather` for each individual gatherer test, we use a **shared archive approach**:

1. **BeforeSuite**: Run ONE `DataGather` with `GatherAll` mode
2. **Read Archive Once**: Download and extract the archive to memory
3. **Reuse**: All gatherer validation tests use the same shared archive

**Benefits:**
- Much faster test execution (5+ minutes saved per test)
- More efficient use of cluster resources
- Tests still validate the actual gathered data

### Test Suite Organization

**1. Standard Gatherers Suite** (`test/extended/gatherers/suite_test.go`)
- **BeforeSuite**: Create PVC → Run DataGather → Read archive
- **Tests**: Validate each gatherer's content from shared archive
  - `nodes_test.go` - Validate node data
  - `operators_test.go` - Validate ClusterOperator data
  - `crds_test.go` - Validate CRD data
  - `configmaps_test.go` - Validate ConfigMap gathering
  - `machines_test.go` - Validate Machine API data
  - etc. (69 ClusterConfig gatherers total)
- **AfterSuite**: Cleanup PVC and DataGather CR

**2. Anonymization Suite** (`test/extended/gatherers/anonymization_test.go`)
- **BeforeSuite**: Run DataGather with `dataPolicy.obfuscateNetworking: true`
- **Tests**: Validate that sensitive data is anonymized
  - IP addresses are obfuscated
  - MAC addresses are masked
  - Hostnames are anonymized
  - URLs are sanitized

**3. Custom Resource Suite** (`test/extended/gatherers/custom_resources_test.go`)
- **BeforeSuite**: 
  - Create specific CRs not present by default (e.g., test ConfigMaps, Secrets)
  - Run DataGather
- **Tests**: Validate those specific resources appear in archive

**4. Conditional Gatherers Suite** (`test/extended/gatherers/conditional_test.go`)
- **BeforeSuite**:
  - Set up conditions (e.g., create/mock specific alerts)
  - Run DataGather with conditional gathering enabled
- **Tests**: Validate conditional data appears when conditions are met

**5. Controller Suites** (`test/extended/controllers/*.go`)
- Test controller behaviors (not archive content)
- SCA certificates, cluster transfer, runtime-extractor, etc.

### Implementation Details

**Shared Archive Pattern:**

```go
// test/extended/gatherers/suite_test.go
package gatherers

var (
    sharedArchive []byte  // Shared across all tests in this suite
    pvcName      string
    dgName       string
)

var _ = g.BeforeSuite(func() {
    // Create PVC
    // Create DataGather CR
    // Wait for completion
    // Read archive once
    sharedArchive, err = util.ReadArchiveFromPVC(...)
})

var _ = g.AfterSuite(func() {
    // Cleanup PVC and DataGather
})
```

**Individual Test Pattern:**

```go
// test/extended/gatherers/nodes_test.go
var _ = g.It("validates node gathering", func() {
    // Use sharedArchive variable from suite
    nodeFiles := util.ExtractFilesMatching(sharedArchive, "config/node/")
    
    // Validate structure and content
    for filename, content := range nodeFiles {
        var node corev1.Node
        json.Unmarshal(content, &node)
        o.Expect(node.Name).NotTo(o.BeEmpty())
        // ... more validations
    }
})
```

## Current Implementation Status

### ✅ Completed
- OTE framework integration
- Test directory structure
- Archive reading helpers
- Makefile targets for integration tests
- Client initialization (simplified, no compat_otp dependency)

### 🔄 Next Steps
1. Finish dependency vendoring
2. Build OTE binary successfully
3. Create suite_test.go with shared archive approach
4. Add multiple gatherer validation tests
5. Add specialized suites (anonymization, custom resources, conditional)
6. Test against real cluster

## Technical Notes

- **No compat_otp dependency**: Simplified to direct client initialization from KUBECONFIG
- **PVC approach**: DataGather uses PersistentVolume storage, test pod mounts PVC to read archive
- **Archive path**: Standardized at `/archive/insights-*.tar.gz` in the PVC
- **Test isolation**: Each suite has its own BeforeSuite/AfterSuite with unique PVC
