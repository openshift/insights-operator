# GPU Node Setup on OpenShift (AWS us-east-2)

## Prerequisites

- Access to an OpenShift cluster on AWS (`oc` logged in)

---

## Step 1: Create the MachineSet

Start from an existing worker MachineSet so all cluster-specific values (AMI, security groups, IAM profile, tags) are already correct.

### 1a. Export an existing MachineSet

Pick any worker MachineSet and export it:
```
oc get machineset -n openshift-machine-api
oc get machineset <EXISTING_WORKER_MACHINESET> -n openshift-machine-api -o yaml > gpu-machineset.yaml
```

### 1b. Edit the following fields

Open `gpu-machineset.yaml` and make these changes:

| Field | What to change |
|-------|---------------|
| `metadata.name` | Rename to something unique, e.g. `<your-username>-gpu-worker` |
| `metadata.resourceVersion`, `metadata.uid`, `metadata.creationTimestamp` | Delete these lines entirely |
| `spec.selector.matchLabels[machine.openshift.io/cluster-api-machineset]` | Update to match the new `metadata.name` |
| `spec.template.metadata.labels[machine.openshift.io/cluster-api-machineset]` | Update to match the new `metadata.name` |
| `spec.template.spec.providerSpec.value.instanceType` | Change to `g4dn.4xlarge` (Tesla T4) |
| `spec.template.spec.providerSpec.value.spotMarketOptions` | Set to `{}` to use spot instances |
| `spec.template.spec.providerSpec.value.placement` | Keep `region`, remove `availabilityZone` if present |
| `spec.template.spec.providerSpec.value.subnet` | Keep the subnet filter as-is (start with `us-east-2a`) |
| `status` | Delete the entire `status` block |

**Why these choices:**
- `g4dn.4xlarge` over `g5.4xlarge` — `g5` frequently has capacity issues in `us-east-2`
- `spotMarketOptions: {}` — spot pool is separate from on-demand, often available when on-demand isn't
- Keep the subnet filter explicit — do **not** remove it or set `subnet: {}`, as AWS may auto-pick a subnet from a different VPC which breaks security group attachment

### 1c. Apply

```
oc apply -f gpu-machineset.yaml
```

### 1d. If you get `InsufficientInstanceCapacity`

MachineSet label selectors are immutable, so you must delete and recreate. Switch the subnet to the other AZ and update the two `cluster-api-machineset` labels to match:

```
oc delete machineset <YOUR_USERNAME>-gpu-worker -n openshift-machine-api
# edit subnet filter from us-east-2a -> us-east-2c (or vice versa)
# edit the two cluster-api-machineset labels to match
oc apply -f gpu-machineset.yaml
```

Check which AZ subnets exist in this cluster:
```
oc get machineset -n openshift-machine-api -o json | grep subnet-private
```

### 1e. Wait for the machine to reach `Running`

```
oc get machines -n openshift-machine-api | grep gpu
```

---

## Step 2: Install NFD Operator

1. In the OpenShift console, go to **OperatorHub** and install **Node Feature Discovery** into the `openshift-nfd` namespace.
2. Create a `NodeFeatureDiscovery` CR (default config is sufficient):
   ```
   oc apply -f - <<EOF
   apiVersion: nfd.openshift.io/v1
   kind: NodeFeatureDiscovery
   metadata:
     name: nfd-instance
     namespace: openshift-nfd
   spec:
     operand:
       servicePort: 12000
     workerConfig:
       configData: |
         sources:
           pci:
             deviceClassWhitelist:
               - "0200"
               - "03"
               - "12"
             deviceLabelFields:
               - "vendor"
   EOF
   ```
3. Verify NFD workers are running on all nodes including the GPU node:
   ```
   oc get pods -n openshift-nfd -o wide
   ```
4. Confirm the GPU node was detected (NVIDIA PCI vendor ID is `10de`):
   ```
   oc get node <GPU_NODE> --show-labels | grep pci-10de
   ```

---

## Step 3: Install NVIDIA GPU Operator

1. In the OpenShift console, go to **OperatorHub** and install **NVIDIA GPU Operator** (`gpu-operator-certified`) into the `nvidia-gpu-operator` namespace.
2. Create a `ClusterPolicy` using `gpu-clusterpolicy.yaml`:

```yaml
apiVersion: nvidia.com/v1
kind: ClusterPolicy
metadata:
  name: gpu-cluster-policy
spec:
  operator:
    defaultRuntime: crio
    use_ocp_driver_toolkit: true
  driver:
    enabled: true
    usePrecompiled: false
  toolkit:
    enabled: true
  devicePlugin:
    enabled: true
  dcgm:
    enabled: true
  dcgmExporter:
    enabled: true
  gfd:
    enabled: true
  migManager:
    enabled: true
  nodeStatusExporter:
    enabled: true
  validator:
    plugin:
      env:
        - name: WITH_WORKLOAD
          value: "false"
  daemonsets:
    rollingUpdate:
      maxUnavailable: "1"
  sandboxDevicePlugin:
    enabled: false
  vgpuDeviceManager:
    enabled: false
  vgpuManager:
    enabled: false
```

Apply:
```
oc apply -f gpu-clusterpolicy.yaml
```

3. Watch the operator deploy daemonsets (~5 minutes for driver compilation):
   ```
   oc get pods -n nvidia-gpu-operator -w
   ```

---

## Step 4: Verify

Check the GPU node has labels and capacity set by the device plugin and GPU Feature Discovery:
```
oc get node <GPU_NODE> -o json | python3 -c "
import json, sys
node = json.load(sys.stdin)
labels = {k: v for k, v in node['metadata']['labels'].items() if 'nvidia' in k}
capacity = {k: v for k, v in node['status']['capacity'].items() if 'nvidia' in k}
print('labels:', labels)
print('capacity:', capacity)
"
```

Expected output includes:
- `nvidia.com/gpu.product: Tesla-T4`
- `nvidia.com/gpu: 1`

---

## Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| `InsufficientInstanceCapacity` | No `g4dn`/`g5` capacity in that AZ | Delete MachineSet, switch subnet to the other AZ, recreate |
| `no subnet IDs were found` | Subnet tag doesn't exist in this cluster | Check available subnets: `oc get machineset -n openshift-machine-api -o json \| grep subnet-private` |
| `Security group and subnet belong to different networks` | `subnet: {}` caused AWS to pick a subnet from wrong VPC | Always specify the subnet filter explicitly |
| GPU node has no `nvidia.com/gpu` labels | ClusterPolicy not created | Apply `gpu-clusterpolicy.yaml` |
