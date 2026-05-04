---
name: write-tests
description: Write unit tests for Go code following the insights-operator project conventions
---

# write-tests

Write unit tests for Go code following the insights-operator project conventions.

## Usage

```
/write-tests <source-file> [test-file]
```

**Arguments:**
- `<source-file>` - Path to the Go source file containing the functions to test
- `[test-file]` - Optional path to the test file (defaults to `<source-file>_test.go`)

## What this skill does

1. Reads the source file to identify all exported and unexported functions
2. Reads the existing test file (if it exists) to identify what's already tested
3. Analyzes each function to identify critical code paths (error paths, branches, edge cases)
4. Generates minimal table-driven tests covering all code paths
5. Follows insights-operator conventions:
   - Test method naming: `Test_<FunctionName>` or `Test_<GatherName>_<FunctionName>`
   - Table-driven test structure
   - Uses `testify/assert` for assertions
   - Uses fake Kubernetes clients when needed
   - Imports grouped: stdlib, external dependencies, current project
   - Focuses on path coverage, not exhaustive edge cases

## Examples

```bash
# Test a utility file
/write-tests pkg/utils/myutil.go

# Specify custom test file location
/write-tests pkg/gather/data.go pkg/gather/data_custom_test.go

# Test a gatherer
/write-tests pkg/gatherers/clusterconfig/gather_nodes.go
```

---
prompt: |
  You are writing unit tests for the insights-operator project. Follow these strict guidelines:

  ## Project Test Conventions (from STYLEGUIDE.md)
  - Use table-driven tests
  - Method names: `Test_<FunctionName>` or `Test_<GatherName>_<FunctionName>`
  - Import groups: stdlib, external dependencies, current project
  - Use `github.com/stretchr/testify/assert` for assertions
  - Use `k8s.io/client-go/kubernetes/fake` for Kubernetes client mocks

  ## Your Task
  1. Read the source file: {{arg1}}
  2. Read the test file (if exists): {{arg2 || arg1 with _test.go suffix}}
  3. Identify untested functions or functions with incomplete coverage
  4. For each function, identify the critical code paths:
     - Error return paths
     - Conditional branches (if/else, switch cases)
     - Loop edge cases (empty, single item, multiple items)
     - Nil/empty input handling
     - Boundary conditions
  5. Write MINIMAL tests that cover these paths - avoid testing the same logic multiple times
  6. Use table-driven structure with clear test case names

  ## Test Structure Pattern

  ```go
  func Test_FunctionName(t *testing.T) {
      tests := []struct {
          name    string
          // input fields
          want    expectedType
          wantErr error  // or wantErrsCount int
      }{
          {
              name: "descriptive case name",
              // test data
              want: expected,
          },
      }
      for _, tt := range tests {
          t.Run(tt.name, func(t *testing.T) {
              // setup (e.g., fake clients)
              // execute function
              // assert results
          })
      }
  }
  ```

  ## What NOT to do
  - Don't create dozens of similar test cases - one per path is enough
  - Don't test trivial getter/setter functions
  - Don't duplicate tests that already exist
  - Don't add tests for unexported helper functions unless they have complex logic
  - Don't over-test - focus on the essential paths

  ## Process
  1. First, output a brief analysis of what functions need testing
  2. Then write or update the test file
  3. Explain what coverage was added

  Source file: {{arg1}}
  Test file: {{arg2}}
