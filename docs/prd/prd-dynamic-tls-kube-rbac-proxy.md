# PRD: Dynamic TLS Configuration for kube-rbac-proxy

**Bug**: OCPBUGS-78774
**Date**: 2026-04-23

## Problem Statement

The `kube-rbac-proxy` container in the `insights-runtime-extractor` DaemonSet (`manifests/10-insights-runtime-extractor.yaml`) uses default values for the TLS Cipher suites and the minimum version.
 OpenShift clusters allow administrators to configure a cluster-wide TLS security profile via `apiservers.config.openshift.io/cluster`. The kube-rbac-proxy must honor this central configuration by specifying its `--tls-cipher-suites` and  `--tls-min-version` arguments instead of relying on default values.

## Current State

1. **Static manifest**: The DaemonSet is defined in `manifests/10-insights-runtime-extractor.yaml` and deployed as part of the release payload. The operator does not manage it programmatically.

2. **Default TLS seetings**: `--tls-cipher-suites` & `--tls-min-version` are not set and do not honor OpenShift central configuration.

3. **Existing TLS helpers**: `pkg/insights/insightsclient/apiserver_config.go` already fetches the TLS profile from `apiservers.config.openshift.io/cluster` and converts it to a `crypto/tls.Config` using `library-go/pkg/crypto`. This is used for the operator's own outbound HTTP connections.

4. **RBAC**: The operator's ClusterRole already has `get`, `list`, `watch` on `apiservers` in the `config.openshift.io` API group.

## Implementation

Follows the pattern established by [cluster-dns-operator PR #466](https://github.com/openshift/cluster-dns-operator/pull/466/).

### 1. Exported TLS helpers (`pkg/insights/insightsclient/apiserver_config.go`)

Two existing helpers were exported for reuse:

- `GetTLSSecurityProfile(configClient)` — fetches `apiservers.config.openshift.io/cluster` and reads `Spec.TLSSecurityProfile`, falling back to `TLSProfileIntermediateType` if unset.
- `GetTLSProfileSpec(profile)` — resolves a profile type (Old/Intermediate/Modern/Custom) to a concrete `TLSProfileSpec` using `configv1.TLSProfiles`.

These are also used by the existing `GetTLSConfigFromAPIServer` for the operator's own outbound HTTP connections.

### 2. kube-rbac-proxy TLS arg builder (`pkg/controller/runtime_extractor.go`)

`buildKubeRBACProxyArgs(profile)` converts a `TLSSecurityProfile` into kube-rbac-proxy CLI arguments:

- Resolves the profile to a `TLSProfileSpec` via `insightsclient.GetTLSProfileSpec`.
- For TLS 1.3 (Modern profile): only emits `--tls-min-version` since TLS 1.3 cipher suites are fixed by the protocol and not configurable by kube-rbac-proxy.
- For other profiles: converts OpenSSL cipher names to IANA names using `crypto.OpenSSLToIANACipherSuites()`. If all cipher names are unrecognized, falls back to the Intermediate profile.
- Returns args: `--tls-cipher-suites=<comma-separated IANA names>` and `--tls-min-version=<version>`.

### 3. DaemonSet reconciliation (`pkg/controller/runtime_extractor.go`)

The operator manages the `insights-runtime-extractor` DaemonSet lifecycle instead of relying solely on the static manifest:

- **Embeds the base manifest** (`manifests/10-insights-runtime-extractor.yaml`) in the operator binary using `//go:embed` (copied to `pkg/controller/manifests/`).
- `reconcileRuntimeExtractorDaemonSet` performs create-or-update reconciliation:
  1. Parses the embedded DaemonSet manifest.
  2. Fetches the current TLS profile via `insightsclient.GetTLSSecurityProfile`.
  3. Builds kube-rbac-proxy args via `buildKubeRBACProxyArgs`.
  4. Patches the kube-rbac-proxy container args (strips existing `--tls-cipher-suites` / `--tls-min-version` and appends new ones).
  5. Creates the DaemonSet if not found, or updates it if the spec has changed (skips update if unchanged).

### 4. Watch for TLS configuration changes (`pkg/controller/operator.go`)

- On startup, performs an initial DaemonSet reconciliation.
- Registers a `tlsReconcileHandler` on the `configInformers.Config().V1().APIServers()` informer, which triggers reconciliation on add/update events for `apiservers.config.openshift.io/cluster`.

## Cipher Suite Mapping

The mapping between OpenShift TLS profiles and kube-rbac-proxy arguments uses `github.com/openshift/library-go/pkg/crypto`:

| Step | Function | Purpose |
|------|----------|---------|
| 1 | `configv1.TLSProfiles[profileType]` | Resolve profile type to `TLSProfileSpec` (ciphers + min version) |
| 2 | `crypto.OpenSSLToIANACipherSuites(spec.Ciphers)` | Convert OpenSSL names (e.g., `ECDHE-RSA-AES128-GCM-SHA256`) to IANA names (e.g., `TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256`) |
| 3 | Format as `--tls-cipher-suites=<joined>` | Pass to kube-rbac-proxy CLI |
| 4 | `spec.MinTLSVersion` (e.g., `VersionTLS12`) | Pass as `--tls-min-version=VersionTLS12` |

## Acceptance Criteria

- [ ] kube-rbac-proxy parameters (`--tls-cipher-suites` and `--tls-min-version`) are based on `apiservers.config.openshift.io/cluster` TLS security profile.
- [ ] There is a mapping between the cipher suites returned by OpenShift and the arguments passed to kube-rbac-proxy using `github.com/openshift/library-go/pkg/crypto`.
- [ ] When the TLS security profile is not configured, the Intermediate profile is used as the default.
- [ ] When the TLS security profile changes on the cluster, the DaemonSet is reconciled with the updated args.
- [ ] Unit tests cover the TLS arg builder, including all profile types (Old, Intermediate, Modern, Custom) and edge cases (nil profile, unrecognized ciphers).

## Key Files

| File | Role |
|------|------|
| `manifests/10-insights-runtime-extractor.yaml` | Base DaemonSet manifest (source of truth, copied into `pkg/controller/manifests/` for embedding) |
| `pkg/controller/manifests/10-insights-runtime-extractor.yaml` | Embedded copy of the base manifest used at runtime |
| `pkg/controller/runtime_extractor.go` | DaemonSet reconciliation, TLS arg builder, manifest parsing, informer event handler |
| `pkg/controller/runtime_extractor_test.go` | Unit tests for arg builder, patching, reconciliation |
| `pkg/insights/insightsclient/apiserver_config.go` | Exported `GetTLSSecurityProfile` and `GetTLSProfileSpec` helpers |
| `pkg/controller/operator.go` | Initial reconciliation call and APIServer informer watch registration |
| `manifests/03-clusterrole.yaml` | RBAC (already has apiserver read permissions) |

## Reference

- [cluster-dns-operator PR #466](https://github.com/openshift/cluster-dns-operator/pull/466/) - Reference implementation of the same pattern.
- `configv1.TLSProfiles` - Profile type to spec mapping from `github.com/openshift/api/config/v1`.
- `crypto.OpenSSLToIANACipherSuites` - Cipher name conversion from `github.com/openshift/library-go/pkg/crypto`.
