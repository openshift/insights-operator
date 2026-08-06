---
name: create-gatherer
description: Create a new gathering function including implementation, tests, sample archive data, registration, and docs regeneration
---

# create-gatherer

Create a new gathering function in the insights-operator, following the project's established patterns exactly.

## Usage

```
/create-gatherer <jira-ticket-or-description>
```

**Arguments:**
- `<jira-ticket-or-description>` - Either a Jira ticket key (e.g., `CCXDEV-12345`) or a natural language description of what to gather. When a Jira ticket is provided, the skill fetches the ticket details using `acli` and extracts requirements from it.

## Examples

```bash
# From a Jira ticket (preferred — pulls requirements automatically)
/create-gatherer CCXDEV-16040

# Plain description — gather a single cluster-scoped OpenShift config resource
/create-gatherer Gather the cluster DNS config (config.openshift.io/v1 DNS)

# Plain description — gather a list of namespaced core K8s resources
/create-gatherer Gather all Deployments in openshift-* namespaces, limit to 200

# Plain description — gather a custom resource using dynamic client
/create-gatherer Gather VirtualMachines from kubevirt.io/v1, include only status
```

---
prompt: |
  You are creating a new gathering function for the insights-operator project.
  Follow EVERY convention below exactly. Do not deviate.

  ## Step 0: Fetch Jira ticket (if applicable)

  Check if the user's input matches a Jira ticket key pattern (e.g., `CCXDEV-12345`, `OCPBUGS-99999` — typically uppercase letters, a dash, and digits).

  If it is a Jira ticket key, fetch the ticket details:
  ```bash
  acli jira workitem view <TICKET_KEY> --fields summary,description,comment,labels,priority
  ```

  Extract from the ticket:
  - **Summary**: the one-line title (often contains the resource type and API group)
  - **Description**: detailed requirements — look for resource kinds, API groups/versions, expected instance counts, backport requirements, linked Insights recommendations (INSIGHTOCP tickets), and any specific fields mentioned
  - **Comments**: may contain additional context, decisions, or clarifications from the team

  Use this information to pre-fill the requirements in Step 1. Present what you extracted to the user and ask them to confirm or correct before proceeding.

  If the input is NOT a Jira ticket key, treat it as a plain text description and proceed directly to Step 1.

  ## Step 1: Gather requirements from the user's input

  From the user's description (or the Jira ticket content from Step 0), determine:
  1. **Resource type** (e.g., `DNS`, `Deployment`, `VirtualMachine`)
  2. **API group/version** (e.g., `config.openshift.io/v1`, `apps/v1`, `kubevirt.io/v1`)
  3. **Scope**: cluster-scoped singleton (`Get`) vs list of resources (`List`)
  4. **Namespace filter**: all namespaces, specific namespace, or `openshift-*` prefix filter
  5. **What to store**: full resource, status only, or specific fields
  6. **Limits**: maximum number of records if listing many resources
  7. **Which gatherer package** to add the function to (default: `clusterconfig`; other options: `workloads`, `conditional`)
  8. **Backport requirements**: if the ticket specifies backport versions, note them for the doc comment

  If anything is ambiguous, ASK the user before proceeding.

  ## Step 2: Investigate the resource schema for sensitive data and data size

  BEFORE writing any code, research the resource type to identify:
  - Fields that may contain sensitive data requiring anonymization
  - The expected data volume and whether size mitigation is needed

  ### How to investigate:
  1. **For OpenShift/Kubernetes API types**: Look up the Go type definition in the vendored dependencies or the OpenShift API repository. Search for the struct definition in `vendor/` (e.g., `vendor/github.com/openshift/api/config/v1/types.go` for config.openshift.io resources, or `vendor/k8s.io/api/` for core K8s types).
  2. **For CRDs**: Search the web for the CRD schema documentation or look at the API reference.
  3. **For any resource type**: Use `kubectl explain` or the OpenShift API docs to understand the fields.

  ### Sensitive data — fields that MUST be anonymized:
  - **IP addresses** (node IPs, cluster IPs, load balancer IPs, endpoint IPs)
  - **Hostnames and DNS names** (especially external-facing ones)
  - **URLs** (API server URLs, webhook URLs, external service URLs)
  - **Cloud provider identifiers** (AWS region, account ID, VPC ID; GCP project ID, region; Azure subscription, resource group, cloud name)
  - **Infrastructure names** (cluster name in infrastructure status)
  - **Certificate data** (PEM-encoded certificates, keys)
  - **Secret references** (values, not names)
  - **User identifiers** (usernames, email addresses, organization names)
  - **MAC addresses**
  - **Any field that could identify a specific customer or their infrastructure**

  ### Sensitive data — fields that do NOT need anonymization:
  - Resource names (metadata.name) — unless they contain user-chosen values that could identify the customer
  - Labels and annotations with well-known keys (e.g., `app.kubernetes.io/name`)
  - Enum/type fields (e.g., platform type, storage class provisioner name)
  - Kubernetes version strings
  - Resource versions, UIDs, timestamps
  - Boolean flags, counts, quantities

  ### Data size — evaluate and mitigate:
  Every gathered resource ends up in a tar.gz archive uploaded to Red Hat. Large data bloats the archive, increases upload time, and can cause failures. Evaluate the expected size:

  **Questions to consider:**
  - How many instances of this resource typically exist in a cluster? (1? 10? 1000? 10000+?)
  - How large is each resource when serialized to JSON? (1KB? 10KB? 100KB+?)
  - Does the resource contain large embedded data? (embedded certificates, logs, binary data, large status sections)

  **Size mitigation strategies (apply as needed):**
  - **Cap the number of records**: Use a `limit` constant (e.g., `const maxResources = 200`) and truncate with an error noting the truncation. See existing patterns like `GatherActiveAlerts` (max 1000).
  - **Strip unnecessary fields**: Remove `.metadata.annotations` (if large/noisy), `.metadata.labels` (if noisy), `.status.conditions[*].message` (if verbose), embedded specs of sub-resources. Only store what's needed for analysis.
  - **Store only .status or .spec**: If only part of the resource is useful, use `record.JSONMarshaller` with a custom struct containing only the needed fields rather than `record.ResourceMarshaller` with the full resource.
  - **Use field selectors or label selectors**: Filter at the API level to reduce the amount of data fetched.
  - **Paginate large lists**: Use `metav1.ListOptions{Limit: N}` for very large resource lists.

  **Size thresholds to flag:**
  - Single resource > 50KB serialized: consider stripping fields
  - Total gathered data for this function > 500KB: strongly consider limiting or filtering
  - More than 500 individual records: cap with a limit constant

  ### Present findings to the user BEFORE implementing:

  After investigating, present your findings:
  ```
  Resource: {ResourceType} ({api-group/version})

  Anonymization:
    Fields to anonymize:
      - .status.infrastructureName (contains cluster-identifying infrastructure name)
      - .status.platformStatus.aws.region (reveals cloud region)
      - ...
    Fields safe as-is:
      - .spec.platformSpec.type (enum value, not sensitive)
      - .metadata.name (well-known singleton name "cluster")
      - ...

  Data size assessment:
    - Expected instance count: {N} (singleton / ~10 / up to thousands)
    - Estimated size per resource: {X}KB
    - Total estimated archive contribution: {Y}KB
    - Mitigation needed: {yes/no}
      - {if yes: specific mitigation strategy}

  Concerns:
    - {any fields you're unsure about}
    - {any fields that might contain user-generated content}
    - {any size-related concerns}
  ```

  **ASK the user to confirm** the anonymization and data size plan before proceeding. Wait for confirmation.

  ## Step 3: Determine the archive path

  Explore `docs/insights-archive-sample/` to understand the existing archive structure and determine where the new data should be placed.

  **Archive path conventions:**
  - `config/` — main cluster configuration data (most clusterconfig gatherer data goes here)
    - `config/{resource}.json` — singleton cluster-scoped resources (e.g., `config/infrastructure.json`, `config/proxy.json`)
    - `config/{resource_plural}/{name}.json` — list of cluster-scoped resources (e.g., `config/storage/storageclasses/standard-csi.json`)
    - `config/pod/{namespace}/{pod-name}.json` — pod definitions
    - `config/configmaps/{namespace}/{name}/{key}` — configmap data
    - `config/secrets/{namespace}/{name}/data.json` — secret metadata
    - `config/clusteroperator/{name}.json` — cluster operator statuses
  - `cluster-scoped-resources/{api-group}/{resource_plural}/{name}.json` — cluster-scoped CRDs not in the config.openshift.io API group (e.g., `cluster-scoped-resources/nmstate.io/nodenetworkstates/`)
  - `namespaces/{namespace}/{api-group}/{resource_plural}/{name}.json` — namespace-scoped custom resources (e.g., `namespaces/openstack/core.openstack.org/openstackcontrolplanes/`)
  - `customresources/{api-group}/{resource_plural}/{namespace}/{name}.json` — another pattern for custom resources
  - `aggregated/` — pre-aggregated or computed data
  - `conditional/` — conditional gatherer data
  - `events/` — Kubernetes events by namespace
  - `machinesets/` — MachineSet resources
  - `insights-operator/` — operator metadata

  The `Record.Name` field in Go code corresponds to the file path WITHOUT the extension. The extension comes from `Record.Item.GetExtension()` (`.json` for `ResourceMarshaller` and `JSONMarshaller`).

  ## Step 4: Choose the client type

  Pick ONE of these based on the API group:

  ### A. OpenShift config client (`configv1client`)
  Use for resources in `config.openshift.io` API group (Infrastructure, OAuth, Network, Ingress, Image, FeatureGate, APIServer, Authentication, Proxy, Scheduler, DNS).
  - KubeConfig: `g.gatherKubeConfig`
  - Client creation: `configv1client.NewForConfig(g.gatherKubeConfig)`
  - Inner function parameter type: `configv1client.ConfigV1Interface`
  - Fake client import: `configfake "github.com/openshift/client-go/config/clientset/versioned/fake"`
  - Fake client creation: `configfake.NewClientset(objects...)`
  - Reference: `pkg/gatherers/clusterconfig/gather_cluster_infrastructure.go`

  ### B. Core Kubernetes typed client (`kubernetes`)
  Use for core K8s resources (Pods, Nodes, Secrets, ConfigMaps, Services, PVs, etc.) and sub-APIs (StorageV1, PolicyV1, RBAC, etc.).
  - KubeConfig: `g.gatherKubeConfig` for JSON encoding, `g.gatherProtoKubeConfig` for protobuf encoding (use proto for high-volume resources like Nodes, Pods)
  - Client creation: `kubernetes.NewForConfig(g.gatherKubeConfig)` or `kubernetes.NewForConfig(g.gatherProtoKubeConfig)`
  - Inner function parameter type: the specific sub-client interface (e.g., `corev1client.CoreV1Interface`, `storagev1client.StorageV1Interface`)
  - Fake client import: `kubefake "k8s.io/client-go/kubernetes/fake"` (or just `"k8s.io/client-go/kubernetes/fake"`)
  - Fake client creation: `fake.NewClientset(objects...)`
  - Reference: `pkg/gatherers/clusterconfig/gather_storageclass.go`

  ### C. Dynamic client
  Use for custom resources (CRDs) not in the core K8s or OpenShift config APIs.
  - KubeConfig: `g.gatherKubeConfig`
  - Client creation: `dynamic.NewForConfig(g.gatherKubeConfig)`
  - Inner function parameter type: `dynamic.Interface`
  - Fake client import: `dynamicfake "k8s.io/client-go/dynamic/fake"`
  - GVR constant must be added to `pkg/gatherers/clusterconfig/const.go`
  - Reference: `pkg/gatherers/clusterconfig/gather_sap_config.go`

  ### D. REST client (special cases only)
  Use for non-standard APIs (Prometheus, Alertmanager).
  - KubeConfig: `g.metricsGatherKubeConfig` for Prometheus, `g.alertsGatherKubeConfig` for Alertmanager

  ## Step 5: Derive names

  From the resource type, derive:
  - **File name**: `gather_{snake_case_resource}.go` (e.g., `gather_cluster_dns.go`)
  - **Test file name**: `gather_{snake_case_resource}_test.go`
  - **Exported method**: `Gather{PascalCaseResource}` (e.g., `GatherClusterDNS`)
  - **Unexported inner function**: `gather{PascalCaseResource}` (e.g., `gatherClusterDNS`)
  - **Function ID** (for the registry map): `{snake_case}` (e.g., `cluster_dns`)
  - **Archive path**: determined in Step 3 (e.g., `config/dns` for singleton, `config/dns/{name}` for list)

  ## Step 6: Create the implementation file

  Create the file in the appropriate gatherer package directory (e.g., `pkg/gatherers/clusterconfig/gather_{name}.go`).

  The implementation MUST follow this exact three-layer structure. Read the reference files to match the pattern precisely:

  1. **Exported method on `*Gatherer`** — creates the client from the Gatherer's rest.Config fields and delegates to the inner function. Does NO business logic. Has the structured doc comment with ALL mandatory sections.
  2. **Unexported inner function** — accepts client interfaces as parameters (for testability), calls the Kubernetes API, returns `([]record.Record, []error)`.
  3. **Anonymization function** (if needed based on Step 2) — unexported, mutates the resource in place to redact sensitive fields.

  ### Exported method signature (MUST be exactly this):
  ```go
  func (g *Gatherer) GatherXxx(ctx context.Context) ([]record.Record, []error)
  ```

  ### Doc comment format (MANDATORY for `make docs` / `cmd/gendoc/main.go`):
  ```go
  // GatherXxx Collects the ...
  //
  // ### API Reference
  // - https://TODO_FILL_IN_API_REFERENCE
  //
  // ### Sample data
  // - docs/insights-archive-sample/{archive_path}.json
  //
  // ### Location in archive
  // - `{archive_path}.json`
  //
  // ### Config ID
  // `{gatherer_name}/{function_id}`
  //
  // ### Released version
  // - TODO_VERSION
  //
  // ### Backported versions
  // None (or list versions from Jira ticket if specified)
  //
  // ### Changes
  // None
  ```

  The Config ID MUST use the format `{gatherer_name}/{function_id}` where `gatherer_name` is `clusterconfig`, `workloads`, or `conditional`, and `function_id` matches the key in the gathering functions registry.

  ### Record type selection:
  - `record.ResourceMarshaller{Resource: obj}` — for typed K8s/OpenShift objects. Automatically strips `managedFields`. Extension: `.json`.
  - `record.JSONMarshaller{Object: obj}` — for arbitrary Go structs, maps, or subsets of a resource. Extension: `.json`.
  - `marshal.Raw{Str: s}` — for raw string data (logs, metrics). No extension. Import: `"github.com/openshift/insights-operator/pkg/utils/marshal"`.

  ### Anonymization implementation (based on Step 2 findings):
  Use functions from `"github.com/openshift/insights-operator/pkg/utils/anonymize"`:
  - `anonymize.String(s)` — replaces each character with 'x', preserving length
  - `anonymize.URL(s)` — anonymizes URL components
  - `anonymize.Bytes(b)` — byte slice variant

  ### Import grouping order (MANDATORY):
  1. Standard library
  2. External dependencies (blank line separator)
  3. Project packages (blank line separator)

  ## Step 7: Create the test file

  Create the test file alongside the implementation file.

  ### Test rules:
  - Tests call the **UNEXPORTED inner function**, never the exported method
  - Test function naming: `Test_gatherXxx` (matching the unexported function name)
  - Use `github.com/stretchr/testify/assert` for assertions
  - Table-driven tests with `t.Run`
  - Use `context.Background()` as the context

  ### Required test cases (at minimum):
  1. **Successful retrieval** — seed valid data, assert correct records returned
  2. **Not found / empty** — seed no data, assert nil/empty records and no errors
  3. **Anonymization** (if applicable) — seed data with sensitive fields, assert they are anonymized in returned records
  4. **Size limits** (if applicable) — seed data exceeding the limit, assert truncation and error message

  ### Test patterns by client type:

  **OpenShift config client test:**
  ```go
  configClient := configfake.NewClientset(tc.seedObject)
  records, errs := gatherXxx(context.Background(), configClient.ConfigV1())
  ```

  **Core K8s typed client test:**
  ```go
  kubeClient := fake.NewClientset(&someList{Items: tc.seedItems})
  records, errs := gatherXxx(context.Background(), kubeClient.SubAPI())
  ```

  **Dynamic client test:**
  ```go
  client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
      map[schema.GroupVersionResource]string{myGVR: "MyResourceList"})
  ```

  Reference test files:
  - `pkg/gatherers/clusterconfig/gather_cluster_infrastructure_test.go` (configfake pattern)
  - `pkg/gatherers/clusterconfig/gather_storageclass_test.go` (kubefake pattern)

  ## Step 8: Register the function

  ### For clusterconfig gatherer:
  Add an entry to `gatheringFunctions` map in `pkg/gatherers/clusterconfig/clusterconfig_gatherer.go`.
  Insert in **alphabetical order** by key:
  ```go
  "function_id": (*Gatherer).GatherXxx,
  ```

  ### For workloads gatherer:
  Add the function inline in the `GetGatheringFunctions()` method in `pkg/gatherers/workloads/workloads_gatherer.go`.

  ### For conditional gatherer:
  Add a builder function and register it in `gatheringFunctionBuilders` map in `pkg/gatherers/conditional/gathering_functions.go`.

  ## Step 9: Add GVR constant (dynamic client only)

  If using a dynamic client, add the GroupVersionResource to `pkg/gatherers/clusterconfig/const.go`:
  ```go
  myResourceGVR = schema.GroupVersionResource{
      Group: "example.io", Version: "v1", Resource: "myresources",
  }
  ```

  ## Step 10: Create sample archive data

  Create a sample JSON file in `docs/insights-archive-sample/` at the path matching the `Record.Name` + extension.

  The sample file should contain:
  - A realistic but anonymized example of the gathered resource
  - Sensitive fields already anonymized (e.g., IPs replaced with `xxxx`, UUIDs with placeholder values)
  - Proper JSON formatting with 2-space indentation
  - The `metadata` section should include `name`, `resourceVersion`, `creationTimestamp` at minimum
  - No `managedFields` (these are stripped by `ResourceMarshaller`)

  Look at existing sample files in `docs/insights-archive-sample/` for the exact format expected. For example, `docs/insights-archive-sample/config/infrastructure.json` shows the pattern for a singleton config resource.

  For list-type gatherers, create ONE representative sample file (not all possible variants).

  ## Step 11: Run `make docs`

  After creating the implementation, run:
  ```
  make docs
  ```

  This regenerates `docs/gathered-data.md` using `cmd/gendoc/main.go`, which:
  - Scans all Go files under `pkg/gatherers/` for exported functions matching `^((Build)?Gather)(.*)`
  - Extracts their doc comments
  - Validates the Config ID format (must match `^[a-z]+[_a-z]*[a-z]([/a-z][_a-z]*)?[a-z]$`)
  - Generates the documentation

  If `make docs` fails, fix the doc comment format and re-run.

  ## Step 12: Verify

  After creating all files:
  1. Run `go build ./pkg/gatherers/{package}/...` to check compilation
  2. Run `go test ./pkg/gatherers/{package}/... -run Test_gatherXxx` to check tests pass
  3. Run `make lint` to check linting
  4. Verify `make docs` ran successfully

  ## Step 13: Summary

  After completing all steps, print a checklist:
  ```
  Jira ticket: {TICKET_KEY} (if applicable)

  Created files:
    - pkg/gatherers/{package}/gather_{name}.go
    - pkg/gatherers/{package}/gather_{name}_test.go
    - docs/insights-archive-sample/{archive_path}.json

  Modified files:
    - pkg/gatherers/{package}/{registration_file} (added to function registry)
    - pkg/gatherers/clusterconfig/const.go (if dynamic client, added GVR)
    - docs/gathered-data.md (regenerated by make docs)

  TODO items for you to complete:
    [ ] Fill in the API Reference URL in the doc comment
    [ ] Fill in the Released version number
    [ ] Verify sample data matches actual cluster output format
    [ ] Run full test suite: make test
  ```

  ## Critical rules

  - The exported method signature MUST be exactly: `func (g *Gatherer) GatherXxx(ctx context.Context) ([]record.Record, []error)`
  - The exported method creates the client and delegates to the unexported function. It does NO business logic.
  - The unexported function accepts client interfaces (not concrete types, not rest.Config) for testability.
  - Use `errors.IsNotFound(err)` checks for Get operations on optional resources (return `nil, nil`).
  - Import grouping order: stdlib, then external deps, then project packages. Separated by blank lines.
  - Tests call the UNEXPORTED inner function, never the exported method.
  - Test function naming: `Test_gatherXxx` (matching the unexported function name).
  - The registration key in `gatheringFunctions` uses snake_case and must be alphabetically ordered.
  - ALWAYS investigate the resource schema for sensitive fields AND data size BEFORE implementing.
  - ALWAYS present anonymization and data size findings and get user confirmation BEFORE writing code.
  - ALWAYS create sample archive data in `docs/insights-archive-sample/`.
  - ALWAYS run `make docs` after creating the gathering function.
  - ALWAYS explore the existing archive structure before choosing an archive path.

  User's request: {{args}}
