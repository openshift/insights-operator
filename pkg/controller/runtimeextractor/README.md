# Runtime Extractor Controller

## Overview

The Runtime Extractor Controller manages the lifecycle of the `insights-runtime-extractor` DaemonSet in the `openshift-insights` namespace. It ensures that the DaemonSet is deployed, configured correctly, and protected from external modifications.

## Architecture

### Components

1. **runtimeExtractorController** (`controller.go`)
   - Main controller that orchestrates resource lifecycle
   - Responds to configuration changes, version updates, and resource drift

2. **ResourceManager** (`resources/manager.go`)
   - Handles actual Kubernetes resource operations (create, update, delete)
   - Uses `resourceapply` from `library-go` for server-side apply semantics

3. **ResourceInformer** (`informer.go`)
   - Watches for external modifications to runtime-extractor resources
   - Provides event-driven drift detection

### Managed Resources

- **DaemonSet**: `insights-runtime-extractor` - Runs on all Linux worker nodes
  - Container images: extractor, exporter, kube-rbac-proxy
  - Updated automatically when cluster version changes

## Reconciliation Strategy

The controller uses an **event-driven reconciliation** approach rather than periodic polling:

### 1. Configuration Changes
- **Trigger**: `insights-config` ConfigMap changes
- **Action**: Enable or disable runtime-extractor based on `DisableRuntimeExtractor` flag
- **Implementation**: Watches config changes via `ConfigNotifier` interface

### 2. Version Updates
- **Trigger**: Cluster version upgrade notification
- **Action**: Update container images to match new cluster version
- **Implementation**: Receives notifications on `updateCh` channel

### 3. Resource Drift Detection (Informer-based)
- **Trigger**: External modification or deletion of runtime-extractor DaemonSet
- **Action**: Reapply desired state to correct drift
- **Implementation**: Kubernetes informers watch DaemonSet

## Drift Detection Details

### How It Works

The `ResourceInformer` uses Kubernetes informers to watch for changes:

```go
// Watch patterns
DaemonSet:  openshift-insights/insights-runtime-extractor
```

### Detection Criteria

The informer uses **generation-based filtering** to minimize unnecessary reconciliations:

**DaemonSet**:
- Checks if `Generation` changed (only increments when spec changes, not status)
- Filters out ~90% of update events (status updates, controller metadata changes)
- Deletion events always trigger reconciliation

**Why generation-based?**
- **Simple**: Single field check instead of deep comparison
- **Efficient**: Filters out 90% of noise (status updates, reconciliation loops)
- **Reliable**: Kubernetes guarantees generation increments on spec changes
- **Performant**: `resourceapply` handles detailed comparison and actual updates

### Reconciliation Flow

```
External Change → Informer Detects → Notification Sent → Controller Reconciles → State Restored
```

## Resource Protection

### Server-Side Apply
The controller uses `resourceapply` functions from `library-go`, which provide:
- **Three-way merge**: Preserves fields managed by other controllers
- **Generation tracking**: Detects meaningful changes
- **Conflict resolution**: Handles concurrent modifications gracefully

### Retry Logic
All resource apply operations use `retry.RetryOnConflict` with exponential backoff:
- **Automatic retry**: Conflicts are automatically retried (up to 5 attempts by default)
- **Exponential backoff**: Delays increase between retries (10ms, 20ms, 40ms, etc.)
- **Jitter**: Random delay variation to prevent thundering herd
- **Success guarantee**: Eventually consistent even under concurrent modifications

### Ownership
Resources are labeled with:
```yaml
labels:
  app.kubernetes.io/managed-by: insights-operator
  app.kubernetes.io/name: insights-runtime-extractor
```

## Event Flow

### Initial Deployment
```
Operator Starts
    ↓
Read Configuration
    ↓
DisableRuntimeExtractor == false?
    ↓ (yes)
Create/Update Resources
    ↓
Monitor for Changes
```

### Configuration Change
```
Config Updated
    ↓
ConfigNotifier Sends Event
    ↓
Controller Handles Config Change
    ↓
Enable: Apply Resources
Disable: Delete Resources
```

### Resource Drift
```
User/Process Modifies Resource
    ↓
Informer Detects Change
    ↓
Notification Sent to modifiedCh
    ↓
Controller Reapplies Desired State
    ↓
Resource Restored
```

## Usage

### Creating the Controller

```go
// Create informer factory for watching resources
informerFactory := clientInformers.NewSharedInformerFactoryWithOptions(
    kubeClient,
    informerTimeout,
    clientInformers.WithNamespace("openshift-insights"),
)

// Create resource informer
ResourceInformer, err := runtimeextractor.NewResourceInformer(
    eventRecorder,
    informerFactory,
)

// Create controller
controller := runtimeextractor.NewRuntimeExtractorController(
    configNotifier,
    updateCh,
    kubeClient,
    eventRecorder,
    ResourceInformer,
)

// Start informers and controller
go informerFactory.Start(ctx.Done())
go ResourceInformer.Run(ctx, 1)
go controller.Run(ctx)
```

### Configuration

The runtime extractor can be disabled via the `insights-config` ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: insights-config
  namespace: openshift-insights
data:
  config.yaml: |
    dataReporting:
      disableRuntimeExtractor: true  # Set to false to enable
```

## Testing

### Unit Tests
- `informer_test.go`: Tests for resource watching and drift detection
- `resources/*_test.go`: Tests for individual resource management

### Running Tests
```bash
make test
# or
go test ./pkg/controller/runtimeextractor/...
```

## Advantages of Informer-Based Approach

### vs. Periodic Polling

| Aspect | Informer-Based (Our Approach) | Periodic Polling |
|--------|------------------------------|------------------|
| **Performance** | Event-driven, instant response | Fixed interval delay |
| **API Server Load** | Minimal (watch connections) | Regular GET requests |
| **Scalability** | Excellent | Poor with many controllers |
| **Kubernetes-Native** | Yes (standard pattern) | No |
| **Resource Usage** | Low (cached data) | Higher (repeated fetches) |

### Benefits
1. **Immediate drift correction**: Changes detected and corrected within seconds
2. **Low overhead**: Watch connections use less resources than polling
3. **Cached data**: Informers maintain local cache, reducing API server load
4. **Standard pattern**: Follows Kubernetes controller best practices
5. **Integration**: Works seamlessly with OpenShift library-go framework

## Future Enhancements

Potential improvements:
- [ ] Add metrics for drift detection events
- [ ] Implement exponential backoff for reconciliation failures
- [ ] Add status reporting to ClusterOperator CR
- [ ] Support for custom resource priorities/tolerations via configuration

## Related Documentation

- [CLAUDE.md](../../../CLAUDE.md) - Project-wide development guidelines
- [Cluster Transfer Controller](../../../pkg/ocm/clustertransfer/) - Similar controller pattern
- [SCA Controller](../../../pkg/ocm/sca/) - Another periodic controller example
