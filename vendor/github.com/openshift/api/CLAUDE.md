# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

This is the OpenShift API repository, containing the canonical API type definitions and serialization code for OpenShift. APIs defined here ship inside OCP payloads and are used by [openshift/client-go](https://github.com/openshift/client-go).

## Key Commands

### Code Generation
- `make update-codegen-crds` - Regenerate CRDs and update manifests (run after modifying API types)
- `make update-scripts` - Update all generated code (deepcopy, openapi, protobuf, swagger docs)
- `make update` - Runs both update-codegen-crds and update-scripts

### Testing and Validation
- `make test-unit` - Run unit tests
- `make verify` - Run all verification scripts (scripts, crd-schema, codegen-crds)
- `make integration` - Run integration tests in tests/ directory
- `make verify-scripts` - Verify deepcopy, openapi, protobuf, swagger docs, CRDs, types, compatibility

### Linting
- `make lint` - Run linter against changes from master branch
- `make lint-fix` - Run linter with auto-fix enabled

### Build
- `make build` - Build render and write-available-featuresets binaries

### Container Operations
- `make verify-with-container` - Run verification in container
- `make generate-with-container` - Run code generation in container

## Architecture and Code Organization

### API Structure
The repository is organized by OpenShift API groups, each in its own directory:
- `config/` - Cluster configuration APIs (v1, v1alpha1, v1alpha2)
- `apps/`, `build/`, `image/`, `network/`, `route/` - Core OpenShift APIs
- `operator/`, `machine/`, `console/` - Operator and management APIs
- `security/`, `authorization/`, `quota/` - Security and policy APIs

### Feature Gates
- Feature gates are defined in `features/features.go`
- New features must start as v1alpha1 APIs with appropriate feature gates
- Feature promotion requires 99% passing tests or QE sign-off
- Use `features.md` to track feature gate status across cluster profiles

### CRD Generation
The repo uses a three-stage CRD generation process:
1. `empty-partial-schema` - Creates empty CRD manifests for each FeatureGate
2. `schemapatch` - Fills in schemas using kubebuilder with FeatureGate awareness
3. `manifest-merge` - Combines per-FeatureGate manifests for different ClusterProfile/FeatureSet combinations

### Testing Framework
- API validation tests are defined in `<group>/<version>/tests/<crd-name>/FeatureGate.yaml`
- Integration tests use envtest with temporary API servers
- Test suites support onCreate and onUpdate scenarios with validation ratcheting

### Important Files
- `install.go` - Registers all OpenShift API groups with runtime schemes
- `go.mod` - Uses Go 1.24.0, depends on k8s.io/api v0.33.2
- `Makefile` - Comprehensive build and verification targets
- `hack/` - Contains all code generation and verification scripts

## Development Workflow

### Adding New APIs
1. Create new API types in appropriate `<group>/<version>/` directory
2. Add FeatureGate definition in `features/features.go` 
3. Create test suite files: `<api-name>.testsuite.yaml`
4. Run `make update-codegen-crds` to generate CRDs
5. Run `make verify` to ensure all checks pass

### Modifying Existing APIs
1. Make changes to type definitions
2. Update test suites if validation changes
3. Run `make update-codegen-crds` to regenerate manifests
4. Run `make verify` to validate changes

### Required Labels for PRs
- Either `bugzilla/valid-bug` OR all three: `qe-approved`, `docs-approved`, `px-approved`
- Standard `lgtm` and `approved` labels

### FeatureGate Guidelines
- New APIs start as v1alpha1 with TechPreviewNoUpgrade feature gates
- Promotion to Default requires extensive testing coverage
- Breaking changes require new API versions (v1alpha2, etc.)
- Never make API changes when promoting to v1

## Test Patterns

### API Validation Tests
Create test files in `<group>/<version>/tests/` following patterns:
- `AAA_ungated.yaml` - For ungated APIs
- `<FeatureGateName>.yaml` - For feature-gated APIs

### Integration Tests
Located in `tests/` directory with comprehensive test suite definitions supporting:
- onCreate validation testing
- onUpdate immutability testing  
- Status subresource testing
- Validation ratcheting scenarios