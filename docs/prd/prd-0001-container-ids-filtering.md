# PRD-0001: Container IDs Filtering for Runtime Extraction

**Status**: Draft  
**Author**: Jeff Mesnil  
**Created**: 2026-02-02  
**Jira**: [CCXDEV-15960](https://issues.redhat.com/browse/CCXDEV-15960)  
**Related**: [insights-runtime-extractor prd-0001](https://github.com/jmesnil/insights-runtime-extractor/blob/e18320a7988c5a0d7238af8b7ad2d8878f3ec725/docs/prd/prd-0001-container-ids-filtering.md)

## Summary

This PRD describes changes to the insights-operator to improve the performance of runtime information gathering by leveraging the new container ID filtering capability in insights-runtime-extractor. Instead of scanning all running containers on each node, the insights-operator will provide specific container IDs to the extractor, reducing resource consumption and response times.

**Deployment Order**: The insights-runtime-extractor will be updated with POST endpoint support **before** any changes are made to the insights-operator. This guarantees the new endpoint is available when the operator starts using it, eliminating the need for fallback mechanisms.

## Background

### Current Architecture

The workloads gatherer (`pkg/gatherers/workloads/`) runs on a 12-hour interval and collects runtime information from all nodes in the cluster:

1. **Pod Discovery**: Lists all pods in the cluster via Kubernetes API (up to 8000 pods)
2. **Runtime Extraction**: For each node running `insights-runtime-extractor`, makes an HTTPS GET request to `https://{pod-ip}:8443/gather_runtime_info`
3. **Data Processing**: Merges runtime info with pod shape fingerprints
4. **Archive Creation**: Generates `config/workload_info.json` with anonymized data

### Current Flow

```
insights-operator                    insights-runtime-extractor
      |                                        |
      |  GET /gather_runtime_info              |
      |--------------------------------------->|
      |                                        |
      |                                        | (scans ALL containers on node) 
      |                                        |
      |  JSON response with all runtime info   |
      |<---------------------------------------|
```

### Problem Statement

The current implementation has performance inefficiencies:

1. **Redundant Scanning**: The insights-runtime-extractor scans ALL running containers on a node, even though the insights-operator may only need runtime info for a subset of pods
1. **Increased CPU load**: Scanning all containers increases the CPU loads on worker nodes and can affect their performance on high-density cluster nodes
1. **Increased Latency**: Scanning all containers increases response time, especially on nodes with many running containers
1. **Timeout Risk**: The 2-minute timeout per node may be exceeded on nodes with high container density

## Proposal

### Solution Overview

Leverage the new POST endpoint in insights-runtime-extractor that accepts a list of container IDs to scan. The insights-operator will:

1. Determine which container IDs are needed based on the pods it's processing
2. Send only those container IDs to the insights-runtime-extractor via POST request
3. Receive runtime info only for the requested containers

### New Flow

```
insights-operator                    insights-runtime-extractor
      |                                        |
      |  POST /gather_runtime_info             |
      |  Body: {"containerIds": ["id1","id2"]} |
      |--------------------------------------->|
      |                                        |
      |                                        |  (scans ONLY specified containers)
      |                                        |
      |  JSON response with filtered info      |
      |<---------------------------------------|
```

## Implementation Details

### Execution Order Requirement

The insights-runtime-extractor must be called **after** all `workloadContainerShape` structures have been collected (or when `limitReached` is true). This ensures:

1. Only containers that will be included in the final output are scanned
2. No wasted scanning of containers that exceed the pod limit (8000)
3. Container IDs are extracted from the actual shapes being reported

**Current flow** (inefficient):
```
1. Start gathering runtime info (all containers)
2. List pods and build shapes
3. Merge runtime info into shapes
```

**New flow** (optimized):
```
1. List pods, build shapes, and collect container IDs
2. Call insights-runtime-extractor with collected container IDs
3. Merge runtime info into shapes
```

### New Data Structure

To avoid keeping all pod objects in memory while still tracking the information needed for runtime extraction, introduce a lightweight structure that accumulates container IDs by node as pods are processed:

```go
// containerIDsByNode maps node name to list of container IDs running on that node
type containerIDsByNode map[string][]string

// addPodContainers adds container IDs from a pod to the tracking structure
// Called during pod processing, before the pod object is discarded
// Only adds containers that are not terminated (running or waiting)
func (c containerIDsByNode) addPodContainers(pod *corev1.Pod) {
    if pod.Status.Phase != corev1.PodRunning {
        return
    }
    nodeName := pod.Spec.NodeName
    for _, containerStatus := range pod.Status.ContainerStatuses {
        // Skip terminated containers - they no longer have a running process to scan
        if containerStatus.State.Terminated != nil {
            continue
        }
        if containerStatus.ContainerID != "" {
            c[nodeName] = append(c[nodeName], containerStatus.ContainerID)
        }
    }
}
```

This structure:
- Uses minimal memory (only node names and container ID strings)
- Is populated incrementally as pods are processed
- Does not require keeping full `corev1.Pod` objects in memory
- Can be passed to `gatherWorkloadRuntimeInfos` after shape collection is complete

### Changes to `pkg/gatherers/workloads/gather_workloads_runtime_infos.go`

#### Modify HTTP Request to Use POST

Change from GET to POST request with container IDs in the request body:

```go
type gatherRuntimeInfoRequest struct {
    ContainerIDs []string `json:"containerIds"`
}

func getNodeWorkloadRuntimeInfos(
	ctx context.Context,
	url string,
	token string,
	httpCli *http.Client,
    containerIDs []string
) workloadRuntimesResult {
     ...

    reqBody := gatherRuntimeInfoRequest{
        ContainerIDs: containerIDs,
    }
    body, err := json.Marshal(reqBody)
    if err != nil {
        return nil, fmt.Errorf("failed to marshal request: %w", err)
    }
    request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return workloadRuntimesResult{
			Error: err,
		}
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Authorization", "Bearer "+token)

    resp, err := client.Do(req)
    // ... handle response
}
```

#### Update `gatherWorkloadRuntimeInfos` Function

Modify the main gathering function to:
1. Accept container IDs grouped by node (extracted from collected shapes)
2. Only be called after shape collection is complete
3. Pass container IDs to each node request

```go
func gatherWorkloadRuntimeInfos(
    ctx context.Context,
    coreClient corev1client.CoreV1Interface,
    containersByNode containerIDsByNode,
) (workloadRuntimes, []error) {

    ...

  	for i := range runtimePodIPs {
		go func(podInfo podWithNodeName) {
			defer wg.Done()

            containerIDs := containersByNode[podInfo.nodeName]
            // Skip nodes with no containers to scan
            if len(containerIDs) == 0 {
                return
            }

			klog.Infof("Gathering workload runtime info for node %s...\n", podInfo.nodeName)
			hostPort := net.JoinHostPort(podInfo.podIP, "8443")
			extractorURL := fmt.Sprintf("https://%s/gather_runtime_info", hostPort)
			httpCli, err := createHTTPClient()
			if err != nil {
				klog.Errorf("Failed to initialize the HTTP client: %v", err)
				return
			}
			tokenData, err := readToken()
			if err != nil {
				klog.Errorf("Failed to read the serviceaccount token: %v", err)
				return
			}
			nodeWorkloadCh <- getNodeWorkloadRuntimeInfos(ctx, extractorURL, string(tokenData), httpCli, containerIDs)
		}(runtimePodIPs[i])
	}

    ...
}
```

### Changes to `pkg/gatherers/workloads/gather_workloads_info.go`

#### Update `workloadInfo` Function

The `workloadInfo` function is modified to track container IDs as pods are processed. It returns the `containerIDsByNode` structure along with its existing return values:

```go
func workloadInfo(
    ctx context.Context,
    coreClient corev1client.CoreV1Interface,
    imageCh chan string,
) (bool, workloadPods, containerIDsByNode, error) {
    defer close(imageCh)
    limitReached := false
    containersByNode := make(containerIDsByNode)

    // ... existing initialization ...

    for {
        pods, err := coreClient.Pods("").List(ctx, metav1.ListOptions{
            Limit:    workloadGatherPageSize,
            Continue: continueValue,
        })
        if err != nil {
            return false, workloadPods{}, nil, err
        }

        for podIdx := range pods.Items {
            pod := pods.Items[podIdx]

            // ... existing namespace handling ...

            if info.PodCount >= podsLimit || info.PodCount+namespacePods.Count >= podsLimit {
                pods.Continue = ""
                limitReached = true
                break
            }
            namespacePods.Count++

            switch {
            case isPodTerminated(&pod):
                namespacePods.TerminalCount++
                continue
            case podCanBeIgnored(&pod):
                namespacePods.IgnoredCount++
                continue
            }

            podShape, ok := calculatePodShape(h, &pod, nil) // No runtime info yet
            if !ok {
                namespacePods.InvalidCount++
                continue
            }

            // NEW: Track container IDs for pods that contribute to shapes
            containersByNode.addPodContainers(&pod)

            // ... existing shape deduplication and image channel logic ...
        }

        if pods.Continue == "" {
            break
        }
        continueValue = pods.Continue
    }

    // ... existing finalization ...

    return limitReached, info, containersByNode, nil
}
```

#### Update `gatherWorkloadInfo` Function

The orchestration function is updated to:
1. Call `workloadInfo` first (without runtime info)
2. Call `gatherWorkloadRuntimeInfos` with the collected container IDs
3. Merge runtime info into the shapes

```go
func gatherWorkloadInfo(
    ctx context.Context,
    coreClient corev1client.CoreV1Interface,
    imageClient imageclient.ImageV1Interface,
) ([]record.Record, []error) {
    var errs = []error{}

    imageCh, imagesDoneCh := gatherWorkloadImageInfo(ctx, imageClient.Images())

    start := time.Now()
    // Step 1: Build shapes and collect container IDs (no runtime info yet)
    limitReached, info, containersByNode, err := workloadInfo(ctx, coreClient, imageCh)
    if err != nil {
        errs = append(errs, err)
        return nil, errs
    }

    // Step 2: Gather runtime info for only the containers in collected shapes
    workloadInfos, runtimeInfoErrs := gatherWorkloadRuntimeInfos(ctx, coreClient, containersByNode)
    errs = append(errs, runtimeInfoErrs...)

    // Step 3: Merge runtime info into shapes
    mergeRuntimeInfoIntoShapes(&info, workloadInfos)

    // ... existing image handling and record creation ...
    workloadImageResize(info.PodCount)

    ...
}
```

This approach:
- Minimal changes to `workloadInfo` - only adds container ID tracking
- Preserves the existing streaming/paginated pod processing
- Does not require keeping full `corev1.Pod` objects in memory
- Tracks only the minimal data needed (node name + container IDs)
- Stops tracking when `limitReached` is true
- Calls the extractor only after all shape decisions are finalized

#### Add `mergeRuntimeInfoIntoShapes` Function

This new function injects runtime info into the already-built shapes after runtime data has been gathered from the extractor.

**Background**: Currently, `calculatePodShape` receives runtime info as a parameter and looks up each container's runtime info during shape creation. With the new flow, shapes are built first (without runtime info), then runtime info is gathered, then merged in.

**Approach**: Each container shape must store a key that can be used to look up its runtime info later. Reuse the existing `containerInfo` type (defined in `gather_workloads_info.go`):

```go
type containerInfo struct {
    namespace   string
    pod         string
    containerID string
}
```

**Changes to `workloadContainerShape`**:

```go
type workloadContainerShape struct {
    ImageID      string                         `json:"imageID"`
    FirstCommand string                         `json:"firstCommand,omitempty"`
    FirstArg     string                         `json:"firstArg,omitempty"`
    RuntimeInfo  *workloadRuntimeInfoContainer  `json:"runtimeInfo,omitempty"`

    // runtimeKey is used to look up runtime info after shapes are built
    // Must not be serialized to JSON - it's only used internally
    runtimeKey   containerInfo `json:"-"`
}
```

**Note**: The `runtimeKey` field uses the `json:"-"` tag to exclude it from JSON serialization. This field is only used internally during the gathering process and must not appear in the collected `workload_info.json` output.

**The merge function**:

```go
// mergeRuntimeInfoIntoShapes updates the shapes in workloadPods with runtime info
// This is called after shapes are built and runtime info is gathered
func mergeRuntimeInfoIntoShapes(info *workloadPods, runtimeInfos workloadRuntimes) {
    for nsHash, nsPods := range info.Namespaces {
        for shapeIdx := range nsPods.Shapes {
            shape := &nsPods.Shapes[shapeIdx]

            // Merge runtime info for regular containers only
            for containerIdx := range shape.Containers {
                container := &shape.Containers[containerIdx]
                if ri, ok := runtimeInfos[container.runtimeKey]; ok {
                    container.RuntimeInfo = &ri
                }
            }
        }
        info.Namespaces[nsHash] = nsPods
    }
}
```

**When to populate `runtimeKey`**: During `calculateWorkloadContainerShapes` (or the container shape calculation), set the `runtimeKey` for each container:

```go
containerShape := workloadContainerShape{
    ImageID:      imageID,
    FirstCommand: commandHash,
    FirstArg:     argHash,
    runtimeKey: containerInfo{
        namespace:   podMeta.Namespace,
        pod:         podMeta.Name,
        containerID: status[i].ContainerID,
    },
}
```

### API Contract

#### Request

```http
POST /gather_runtime_info HTTP/1.1
Content-Type: application/json
Authorization: Bearer <token>

{
    "containerIds": [
        "cri-o://abc123def456...",
        "cri-o://789xyz012..."
    ]
}
```

#### Response

Same JSON structure as the current GET endpoint, but filtered to only include requested containers.

#### Container ID Format

- Full 64-character container IDs with optional `cri-o://` prefix
- The insights-runtime-extractor automatically strips the `cri-o://` prefix if present
- IDs are obtained from `pod.Status.ContainerStatuses[].ContainerID`

## Backward Compatibility

### Deployment Order Guarantee

Since the insights-runtime-extractor will be updated **before** the insights-operator changes are deployed, backward compatibility with older extractor versions is not required. The POST endpoint will always be available when the updated operator is deployed.

This simplifies the implementation:
- No fallback to GET endpoint needed
- No version detection or feature flags required
- Direct POST requests without retry logic for method compatibility

### Response Format Compatibility

The POST endpoint returns the same JSON response format as the existing GET endpoint, ensuring the operator's response parsing logic remains unchanged.

## Performance Considerations

### Expected Improvements

| Metric | Before | After | Notes |
|--------|--------|-------|-------|
| Containers scanned per node | All running | Only needed | Depends on pod distribution |
| Response time per node | O(n) all containers | O(m) requested containers | m <= n |
| Memory usage on extractor | Higher | Lower | Less data to hold |
| Timeout risk | Higher | Lower | Faster responses |

### Typical Scenarios

1. **Mixed workload cluster**: Node runs 50 containers, operator needs 10 => 80% reduction in scanning
2. **System-heavy node**: Node runs 30 system pods, 5 user pods => Only scan 5 containers
3. **Dense node**: Node at capacity with many containers => Significant timeout risk reduction

## Testing Strategy

### Unit Tests

1. Test `containerIDsByNode.addPodContainers` method:
   - Pod in Running phase adds container IDs
   - Pod in non-Running phase is skipped
   - Terminated containers are skipped (State.Terminated != nil)
   - Running and waiting containers are included
   - Pod with missing container IDs handled gracefully
   - Multiple pods on same node accumulate IDs
   - Pods on different nodes tracked separately

2. Test POST request construction:
   - Valid container ID list
   - Empty container ID list (should skip node)
   - Container ID format handling (with/without `cri-o://` prefix)

3. Test error handling:
   - HTTP errors from extractor
   - Timeout handling
   - Malformed response handling

4. Test execution order:
   - Verify runtime info is not requested before shapes are collected
   - Verify only containers from processed pods are requested
   - Verify containers are not tracked after limitReached is true

### Integration Tests

1. Verify POST request is sent with correct container IDs
2. Verify response parsing matches existing behavior
3. Verify correct container IDs are extracted from pod statuses

### End-to-End Tests

1. **Sequential deployment validation**:
   - Deploy updated insights-runtime-extractor first
   - Verify existing operator still works with GET endpoint
   - Deploy updated insights-operator
   - Verify POST endpoint is used correctly
2. Verify workload_info.json contains expected runtime data
3. Measure performance improvement on cluster with many containers

## Security Considerations

- No new privileges required
- Container IDs are not sensitive (they are also visible in pod status)
- Same authentication mechanism (service account bearer token)
- Same TLS requirements

## Rollout Plan

The rollout follows a strict sequential order to ensure compatibility:

### Phase 1: insights-runtime-extractor Update (First)

1. Deploy insights-runtime-extractor with POST endpoint support ([openshift/insights-runtime-extractor#60](https://github.com/openshift/insights-runtime-extractor/pull/60))
2. Existing GET endpoint continues to work unchanged for current operator
3. Validate POST endpoint works correctly in staging/test environments
4. **Must complete before Phase 2 begins**

### Phase 2: insights-operator Update (Second)

1. Update insights-operator to use POST endpoint with container IDs
2. No fallback logic needed since extractor is already updated
3. Deploy to clusters where extractor update has been confirmed

### Phase 3: Cleanup (Optional, Future)

1. Deprecate GET endpoint in extractor after all operators are updated
2. Remove GET endpoint in future major version of extractor

## Files Modified

| File | Changes |
|------|---------|
| `pkg/gatherers/workloads/gather_workloads_runtime_infos.go` | Add `POST` request support, accept container IDs parameter |
| `pkg/gatherers/workloads/gather_workloads_info.go` | Track container IDs in `workloadInfo`, add `mergeRuntimeInfoIntoShapes`, reorder calls in `gatherWorkloadInfo` |
| `pkg/gatherers/workloads/gather_workloads_info_test.go` | Add tests for container ID tracking and merging |
| `pkg/gatherers/workloads/gather_workloads_runtime_infos_test.go` | Add tests for `POST` request functionality |

## References

- [openshift/insights-runtime-extractor#60](https://github.com/openshift/insights-runtime-extractor/pull/60)
- [CCXDEV-15960](https://issues.redhat.com/browse/CCXDEV-15960)
- [Workloads Gatherer Implementation](../gathered-data.md#workloadinfo)
