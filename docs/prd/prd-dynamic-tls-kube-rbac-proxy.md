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

## Proposed Solution

Follow the pattern established by [cluster-dns-operator PR #466](https://github.com/openshift/cluster-dns-operator/pull/466/):

### 1. Extract kube-rbac-proxy TLS arg builder

Create a function (e.g., in `pkg/insights/insightsclient/apiserver_config.go` or a new dedicated file) that converts the `apiservers.config.openshift.io/cluster` TLS profile into kube-rbac-proxy CLI arguments:

```go
func KubeRBACProxyTLSArgs(configClient configclientset.Interface) ([]string, error)
```

This function must:
- Fetch `apiservers.config.openshift.io/cluster` and read `Spec.TLSSecurityProfile`.
- Fall back to `TLSProfileIntermediateType` if unset.
- Resolve the profile type (Old/Intermediate/Modern/Custom) to a concrete `TLSProfileSpec` using `configv1.TLSProfiles`.
- Convert OpenSSL cipher names to IANA names using `crypto.OpenSSLToIANACipherSuites()` from `github.com/openshift/library-go/pkg/crypto`.
- If all cipher names are unrecognized, fall back to the Intermediate profile.
- Return args: `--tls-cipher-suites=<comma-separated IANA names>` and `--tls-min-version=<version>`.

### 2. Operator manages the DaemonSet dynamically

The operator must take ownership of the `insights-runtime-extractor` DaemonSet lifecycle instead of relying solely on the static manifest:

- **Embed the base manifest** in the operator binary (using `//go:embed`) as a template.
- **On startup and on TLS config changes**, the operator must:
  1. Read the base DaemonSet manifest.
  2. Fetch the current TLS profile from the API server.
  3. Build the kube-rbac-proxy args with the resolved cipher suites and min TLS version.
  4. Patch/replace the kube-rbac-proxy container args in the DaemonSet spec.
  5. Apply the DaemonSet (create or update).

### 3. Watch for TLS configuration changes

The operator must watch `apiservers.config.openshift.io/cluster` for changes and reconcile the DaemonSet when the TLS profile is updated. This ensures the kube-rbac-proxy picks up new cipher suites or TLS version without requiring a cluster upgrade or manual intervention.

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
| `manifests/10-insights-runtime-extractor.yaml` | Base DaemonSet manifest (to be embedded and used as template) |
| `pkg/insights/insightsclient/apiserver_config.go` | Existing TLS profile fetching and conversion (to be extended) |
| `pkg/controller/operator.go` | Main operator controller (DaemonSet reconciliation to be added here) |
| `manifests/03-clusterrole.yaml` | RBAC (already has apiserver read permissions) |

## Reference

- [cluster-dns-operator PR #466](https://github.com/openshift/cluster-dns-operator/pull/466/) - Reference implementation of the same pattern.
- `configv1.TLSProfiles` - Profile type to spec mapping from `github.com/openshift/api/config/v1`.
- `crypto.OpenSSLToIANACipherSuites` - Cipher name conversion from `github.com/openshift/library-go/pkg/crypto`.
