# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Common Development Commands

### Building and Testing
- `make build` - Compiles the insights-operator binary to `./bin/insights-operator`
- `make build-debug` - Compiles with debug symbols for debugging
- `make test` or `make unit` - Runs unit tests with race detection and coverage profiling
- `VERBOSE=-v make unit` - Run unit tests with verbose output
- `VERBOSE=-count=1 make test` - Run tests without caching

### Linting and Code Quality
- `make lint` - Run golangci-lint with project configuration (requires golangci-lint >= 1.39)
- `make precommit` - Execute pre-commit hook (checks stashed changes)
- `make githooks` - Configure repository to use git hooks (recommended setup)

### Running the Operator
- `make run` - Execute insights-operator with default config (`config/local.yaml`)
- `CONFIG=config/custom.yaml make run` - Run with custom configuration
- `bin/insights-operator start --config=config/local.yaml --kubeconfig=$KUBECONFIG` - Direct execution

### Container Operations
- `make build-container` - Build container image using podman/docker
- `make build-debug-container` - Build debug container image

### Documentation and Tools
- `make docs` - Generate gathered-data documentation from code comments
- `make changelog` - Update changelog (requires GITHUB_TOKEN environment variable)
- `make vendor` - Update Go module dependencies (`go mod tidy && go mod vendor && go mod verify`)

## Project Architecture

### Core Components
The Insights Operator is a Kubernetes operator that periodically gathers anonymized cluster data and uploads it to Red Hat Insights for analysis.

**Main Entry Point**: `cmd/insights-operator/main.go` - Sets up cobra CLI with subcommands for start, receive, gather, and gather-and-upload operations.

**Operator Controller**: `pkg/controller/operator.go` - Main operator logic that coordinates periodic data gathering, uploading, and status reporting.

### Key Packages Structure
- `pkg/gather/` - Data gathering logic and gatherer implementations
- `pkg/gatherers/` - Three main gatherer types:
  - `clusterconfig/` - Regular cluster configuration gathering (default 2h interval)
  - `workloads/` - Workload fingerprint data (12h interval, not configurable)
  - `conditional/` - Conditional gathering based on external rules from console.redhat.com
- `pkg/insights/` - Insights client for uploading/downloading data and reports
- `pkg/config/` - Configuration management and observation
- `pkg/controller/` - Operator controllers and status management
  - `gather_commands.go` - Contains `GatherJob` type for non-periodic gathering (gather, gather-and-upload commands)
  - `operator.go` - Main `Operator` type for periodic gathering in production
- `pkg/recorder/` - Archive recording and disk management
- `pkg/anonymization/` - Data obfuscation and anonymization

### Configuration
The operator reads configuration from multiple sources (in order of precedence):
1. `insights-config` ConfigMap in `openshift-insights` namespace
2. `support` Secret in `openshift-config` namespace
3. Default configuration from `config/pod.yaml`

Authentication uses tokens from the `pull-secret` secret in `openshift-config` namespace.

### Scheduling Pattern
The operator uses `wait.Until` for periodic tasks:
- **Gatherer**: Collects cluster data
- **Uploader**: Uploads archives to console.redhat.com
- **Downloader**: Downloads Insights analysis reports
- **Config Observer**: Monitors configuration changes (5min interval)
- **Disk Pruner**: Removes old archives (runs every second interval)
- **SCA Controller**: Simple Content Access certificate management
- **Cluster Transfer Controller**: Handles cluster transfer operations

### Service Accounts
- `operator`: Main service account for the insights-operator deployment
- `gather`: Privileged service account for cluster-wide data gathering (impersonated by operator)

### Custom Resource Definitions

The Insights Operator uses two main CRDs defined in the `openshift/api` repository (both v1alpha2):

#### DataGather (insights.openshift.io/v1alpha2)
Provides configuration and status for on-demand Insights data gathering (TechPreview):
- **Purpose**: Trigger and configure individual data gathering operations
- **Key spec fields**:
  - `dataPolicy` - Optional obfuscation settings (ObfuscateNetworking, WorkloadNames)
  - `gatherers` - Optional gatherer configuration (All or Custom mode with enable/disable per gatherer)
  - `storage` - Optional persistent storage configuration (PersistentVolume or Ephemeral)
- **Key status fields**:
  - `conditions` - Status of gathering phases (DataUploaded, DataRecorded, DataProcessed, RemoteConfiguration*)
  - `gatherers` - Individual gatherer statuses with execution time
  - `insightsRequestID` - Tracking ID for console.redhat.com processing
  - `insightsReport` - Downloaded analysis with health checks and recommendations
  - `startTime`/`finishTime` - Gathering execution timestamps
- **Feature gate**: `InsightsOnDemandDataGather`
- **Usage**: Created automatically for periodic gathering or manually for on-demand gathering
- **Scope**: Cluster-scoped resource

#### InsightsDataGather (config.openshift.io/v1alpha2)
Provides global configuration for Insights data gathering (TechPreview):
- **Purpose**: Configure cluster-wide Insights gathering behavior
- **Key spec fields**:
  - `gatherConfig.dataPolicy` - Global obfuscation options (ObfuscateNetworking, WorkloadNames)
  - `gatherConfig.gatherers` - Gatherer mode configuration (All, None, or Custom)
  - `gatherConfig.storage` - Persistent storage configuration for gathering jobs
- **Gathering modes**:
  - `All` - All gatherers run and gather data
  - `None` - All gatherers disabled, no data gathered
  - `Custom` - Fine-grained control via custom configuration
- **Feature gate**: `InsightsConfig`
- **Usage**: Singleton cluster-scoped resource named "cluster"
- **Scope**: Cluster-scoped resource

### Data Gathering and Reporting

#### Periodic Data Gathering
The operator has three main gatherers that run periodically:
- **Cluster Config Gatherer** (`pkg/gatherers/clusterconfig/`) - Collects cluster configuration (default 2h interval)
- **Workloads Gatherer** (`pkg/gatherers/workloads/`) - Collects workload fingerprints (12h interval, not configurable)
- **Conditional Gatherer** (`pkg/gatherers/conditional/`) - Collects data based on external rules (see below)

#### On-Demand Data Gathering
The operator supports on-demand gathering via `DataGather` custom resources:
- **GatherAndUpload workflow** (`pkg/controller/gather_commands.go`):
  1. Creates gatherers and executes data collection
  2. Records data to archive
  3. Uploads archive to console.redhat.com
  4. Polls processing status endpoint to verify data was processed
  5. Updates `DataGather` CR status with results
- **Processing status polling** (`wasDataProcessed` function):
  - Polls `insights-results-aggregator` service with InsightsRequestID
  - Retries independently for network errors, HTTP errors, and processing status
  - Uses configurable delay between retries (from `DataReporting.ReportPullingDelay`)

#### Conditional Gathering
Conditional gathering enables dynamic data collection based on external rules:
- **Purpose**: Collect specific data only when certain conditions are met (e.g., specific alerts firing)
- **Workflow**:
  1. Downloads gathering rules from console.redhat.com endpoint
  2. Validates rules against JSON schema (`pkg/gatherers/conditional/gathering_rules.schema.json`)
  3. Evaluates conditions (alert firing, cluster version, etc.)
  4. Executes matching gathering functions with parameters
- **Key features**:
  - Rules defined externally in `insights-operator-gathering-conditions` repository
  - Conditions include: alert firing, cluster version ranges, and more
  - JSON schema validation (fails safely if invalid)
  - Requires Prometheus connection for alert-based conditions
  - Gathers targeted data to reduce archive size

#### Data Anonymization
The Anonymizer (`pkg/anonymization/`) protects sensitive information before upload:
- **Purpose**: Obfuscate sensitive data (IPs, MAC addresses, hostnames) in gathered data
- **Key features**:
  - Configurable via `DataPolicy` in DataGather CR
  - Can disable specific anonymization types
  - Applies to networking data, cluster infrastructure, and more
  - Maintains consistency (same input always produces same obfuscated output)
  - See `docs/anonymized-data.md` for full list of anonymized fields

#### Data Uploading
The Uploader Controller (`pkg/insights/insightsuploader/`) handles periodic upload of gathered data:
- **Purpose**: Uploads Insights archives to console.redhat.com for analysis
- **Workflow**:
  1. Periodically checks for new data since last upload
  2. Creates tar.gz archive from recorded data
  3. Uploads with exponential backoff on failures
  4. Notifies report downloader on successful upload
  5. Updates last reported timestamp
- **Key features**:
  - Configurable upload interval (default 2 hours)
  - Exponential backoff with 4 retry steps on failures
  - Returns `InsightsRequestID` for tracking (TechPreview)
  - Dry-run mode when reporting is disabled (logs archive contents)
  - Authorization error handling with extended retry delays

#### Insights Report Downloading
The Insights Report Controller (`pkg/insights/insightsreport/`) downloads and processes analysis reports:
- **Purpose**: Retrieves recommendations and health analysis for the cluster from Insights backend
- **Workflow**:
  1. Waits for archive upload notification
  2. Downloads analysis report from Smart Proxy or insights-results-aggregator (TechPreview)
  3. Parses recommendations with risk levels (critical, important, moderate, low)
  4. Updates Prometheus metrics (`health_statuses_insights`)
  5. Updates `InsightsOperator` CR status with active health checks
  6. Creates Insights Advisor links for each recommendation
- **Key features**:
  - Retry logic for failed downloads (up to 2 retries)
  - 5-minute download timeout with configurable initial delay
  - Supports both legacy Smart Proxy and new TechPreview endpoints
  - Exposes recommendations as Prometheus metrics for alerting
  - Skips duplicate reports (checks `LastCheckedAt` timestamp)

### Prometheus Alerting
The Prometheus Rules Controller (`pkg/insights/prometheus_rules.go`) manages Insights-specific alerts:
- **Managed alerts**:
  - `InsightsDisabled` - Fires when Insights operator is disabled
  - `SimpleContentAccessNotAvailable` - Fires when SCA certificates are unavailable
  - `InsightsRecommendationActive` - Fires for each active Insights recommendation
- **Key features**:
  - Dynamically creates/removes rules based on `Alerting.Disabled` configuration
  - All alerts have `info` severity level
  - 5-minute evaluation period (`for: 5m`)
  - Rules created in `openshift-insights` namespace

### OCM Integration

#### SCA Certificate Management
The SCA (Simple Content Access) Controller (`pkg/ocm/sca/`) periodically pulls Red Hat entitlement certificates from the OCM API:
- **Purpose**: Manages Red Hat entitlement certificates for clusters to access subscription content
- **Workflow**:
  1. Gathers cluster node architectures via Kubernetes API (supports multi-arch clusters)
  2. Requests SCA certificates from OCM API for each detected architecture
  3. Creates/updates secrets in `openshift-config-managed` namespace:
     - `etc-pki-entitlement` - Default secret with control plane architecture certificates
     - `etc-pki-entitlement-<arch>` - Architecture-specific secrets for multi-arch clusters
  4. Each secret contains `entitlement.pem` and `entitlement-key.pem` data
- **Key features**:
  - Exponential backoff retry for HTTP 5xx errors from OCM API
  - Architecture mapping between Kubernetes format (amd64, arm64) and SCA API format (x86_64, aarch64)
  - Default interval: configurable via `SCA.Interval` (typically 8 hours)
  - Can be disabled via configuration (`SCA.Disabled`)

### Cluster Transfer Management
The Cluster Transfer Controller (`pkg/ocm/clustertransfer/`) handles cluster ownership transfers between organizations:
- **Purpose**: Automatically updates the cluster's pull-secret when a cluster transfer is initiated in OCM
- **Workflow**:
  1. Periodically queries OCM API for accepted cluster transfer requests
  2. If exactly one accepted transfer exists, retrieves new pull-secret data
  3. Compares new pull-secret with existing one in `openshift-config` namespace
  4. If different, applies JSON merge patch to update `pull-secret` secret
  5. Updates controller status with operation result
- **Key features**:
  - Exponential backoff retry for HTTP 5xx errors from OCM API
  - Validates only one accepted transfer exists (prevents conflicts)
  - Uses JSON merge patch to preserve existing pull-secret data while adding new registry credentials
  - Default interval: configurable via `ClusterTransfer.Interval` (typically 12 hours)
  - Handles disconnected environments gracefully (logs error but remains healthy)

## Development Workflow

### Code Style (from STYLEGUIDE.md)
- All Go code must be formatted with `gofmt`
- Import groups: stdlib, external dependencies, current project
- Test methods: `Test_<FunctionName>` or `Test_<GatherName>_<FunctionName>`
- Use table-driven tests
- File names: lowercase dash-separated (except Dockerfile, Makefile, README)

### Git Workflow (from CONTRIBUTING.md)
- Install git hooks: `make githooks`
- Base work on master branch
- Commit message format: descriptive subject line with detailed body
- Pull requests must reference Bugzilla bugs for enhancements/backports
- Backport branches follow `release-X.Y` format

### Before Submitting Changes
Always run before creating pull requests:
- `make lint` - Ensure code quality
- `make test` - Verify all tests pass
- Install githooks for automatic pre-commit validation

## Go Module Configuration
- Go version: 1.23.0 with toolchain go1.23.4
- Uses vendored dependencies (`GOFLAGS=-mod=vendor`)
- Local replacements for `github.com/openshift/api` and `github.com/openshift/client-go`
- Main dependencies: OpenShift APIs, Kubernetes client libraries, Prometheus client, Cobra CLI
