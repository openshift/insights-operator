# AGENTS.md

This file provides guidance for AI coding agents and human contributors working on the OpenShift Insights Operator.

## Project Overview

The Insights Operator is a cluster operator that gathers anonymized system configuration and reports it to Red Hat Insights. It helps with debugging cluster failures and unexpected errors by collecting non-secret cluster configuration data and generating anonymized `.tar.gz` archives.

**Key Technologies:**
- Language: Go (1.11+)
- Platform: OpenShift Container Platform / Kubernetes
- License: Apache License 2.0

## Repository Structure

```
insights-operator/
├── pkg/
│   ├── gatherers/           # Data collection modules
│   │   ├── clusterconfig/   # Cluster configuration gatherers
│   │   └── workloads/       # Workload-related gatherers
│   ├── controller/          # Operator controller logic
│   ├── config/              # Configuration handling
│   └── insights/            # Insights API integration
├── config/                  # Configuration files
├── docs/                    # Documentation
├── .githooks/              # Git hooks for pre-commit checks
└── .openshiftci/           # OpenShift CI configuration
```

## Development Environment Setup

### Prerequisites
- Go 1.11 or higher
- Access to an OpenShift/Kubernetes cluster (for testing)
- golangci-lint >= 1.39

### Building
```bash
make build
```

### Running Tests
```bash
make test
```

### Installing Git Hooks
```bash
make githooks
```
This sets up pre-commit hooks to run tests and linting automatically.

### Running Locally
```bash
bin/insights-operator start --config=config/local.yaml --kubeconfig=$KUBECONFIG
```

### Generating Documentation
```bash
make docs
```

## Code Style Guidelines

**IMPORTANT:** Read [STYLEGUIDE.md](STYLEGUIDE.md) before contributing.

### File Naming
- Use lowercase dash-separated names (e.g., `gather-cluster-version.go`)
- Exceptions: `Dockerfile`, `Makefile`, `README.md`

### Go Code Formatting
- Use `gofmt` for all code
- Import statement grouping (in order):
  1. Standard library packages
  2. External dependencies
  3. Current project packages

### Test Naming
Follow these patterns:
- `Test_<FunctionName>`
- `Test_<GatherName>_<FunctionName>`

Example:
```go
func Test_GatherClusterVersion_FetchesCorrectData(t *testing.T) { ... }
```

### String Handling
Prefer: `if string != ""` over `if len(string) > 0`

## Testing

### Running All Tests
```bash
make test
```

### Writing Tests
- Every new gatherer MUST have corresponding unit tests
- Place test files alongside implementation: `gather_foo.go` → `gather_foo_test.go`
- Mock Kubernetes clients appropriately
- Test both success and error paths

### CI/CD
- Tests run automatically in OpenShift CI (`.openshiftci/`)
- Pre-commit hooks run linting and tests locally (install with `make githooks`)

## Pull Request Guidelines

### Commit Message Format
```
<short description of what>

<longer explanation of why this change is needed>
```

Example:
```
Add gatherer for PodDisruptionBudgets

This gatherer collects PDB information from openshift namespaces
to help identify cluster stability issues.
```

### PR Title Format

**For enhancements/bug fixes:**
```
Bug <BUGZILLA_ID>: <Description>
```
Example: `Bug 1940432: Gather datahubs resources`

**For backports:**
```
[release-X.Y] Bug <BUGZILLA_ID>: <Description>
```
Example: `[release-4.6] Bug 1942907: Gather resources`

### PR Checklist
- [ ] Use the PR template
- [ ] Include Bugzilla bug reference (for enhancements)
- [ ] Tests pass (`make test`)
- [ ] Linting passes (`golangci-lint`)
- [ ] Git hooks installed and passing (`make githooks`)
- [ ] Documentation updated if adding new gatherer
- [ ] Changelog considered (use included script if needed)

### Branching Strategy
- Base all PRs on `master` branch
- Backports go to release branches (e.g., `release-4.11`, `release-4.12`)
- Create topic branches from `master` for new work

## Adding New Gatherers

When adding a new data gatherer:

1. **Create the gatherer file** in appropriate directory:
   - `pkg/gatherers/clusterconfig/` for cluster configuration
   - `pkg/gatherers/workloads/` for workload data

2. **Implement the gatherer interface**
   ```go
   func GatherFoo(ctx context.Context, client kubernetes.Interface) ([]record.Record, []error) {
       // Implementation
   }
   ```

3. **Add unit tests** in `*_test.go` file

4. **Update documentation**:
   - Run `make docs` to regenerate gathered data documentation
   - Update any relevant docs in `docs/` directory

5. **Consider data privacy**:
   - No secrets or sensitive data
   - Anonymize hostnames and URLs
   - Review data collected carefully

## Common Patterns and Gotchas

### Gatherer Implementation
- Always use context for cancellation support
- Return both records and errors (don't fail fast on single errors)
- Use structured logging
- Handle nil pointers when accessing Kubernetes objects

### Testing Gatherers
- Mock the Kubernetes client properly
- Test with both empty and populated clusters
- Verify error handling paths
- Check that sensitive data is filtered

### Performance Considerations
- Gatherers run periodically; avoid expensive operations
- Use list operations efficiently
- Consider pagination for large result sets
- Be mindful of API server load

## Debugging

### Profiling
Set the `OPENSHIFT_PROFILE` environment variable to enable profiling.

### Metrics
The operator exposes Prometheus metrics. Access them via:
- Local endpoint when running locally
- Kubernetes service when deployed

### Logs
- Use structured logging throughout
- Include relevant context in log messages
- Different log levels for different scenarios

## AI Agent Limitations and Guidance

### What AI Can Help With
- Writing new gatherers following existing patterns
- Adding unit tests for gatherers
- Refactoring code while maintaining structure
- Updating documentation
- Fixing bugs with clear symptoms

### What AI Should Be Cautious About
- **Security/Privacy**: Verify no sensitive data is collected
- **Kubernetes API changes**: Be aware of version compatibility
- **Backporting**: Requires understanding of version differences
- **Breaking changes**: Operator runs in production clusters
- **Performance impact**: Changes affect cluster performance

### When to Ask for Human Review
- Changes to core controller logic
- New types of data collection (privacy implications)
- Changes to archive format or structure
- API client modifications
- Release/backport decisions

## Additional Resources

- [CONTRIBUTING.md](CONTRIBUTING.md) - Detailed contribution guidelines
- [STYLEGUIDE.md](STYLEGUIDE.md) - Complete coding style guide
- [README.md](README.md) - General project information
- [OpenShift Insights Documentation](https://docs.openshift.com/container-platform/latest/support/remote_health_monitoring/about-remote-health-monitoring.html)

## Support and Issues

- **Product issues**: File through Red Hat JIRA
- **Community contributions**: Use GitHub issues and pull requests
- **Questions**: Ask in pull request comments or issues

---

*This AGENTS.md file helps AI coding agents understand the project structure, conventions, and best practices for the OpenShift Insights Operator.*
