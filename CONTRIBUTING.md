# Contributing to Gorest

Thank you for considering contributing to Gorest! This document outlines the process for contributing to the project.

## Code of Conduct

By participating in this project, you agree to abide by the [Code of Conduct](CODE_OF_CONDUCT.md).

## How Can I Contribute?

### Reporting Bugs

Bug reports help make Gorest better. When you report a bug, please include:

- A clear and descriptive title
- Steps to reproduce the issue
- Expected behavior
- Actual behavior
- Go version and operating system

### Suggesting Enhancements

Suggestions for enhancements are always welcome. Please provide:

- A clear description of the feature
- Rationale for why this would benefit the project
- If possible, examples of how the feature might be used

### Pull Requests

1. Fork the repository
2. Create a new branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Run tests and ensure code quality
5. Commit your changes (`git commit -m 'Add some amazing feature'`)
6. Push to the branch (`git push origin feature/amazing-feature`)
7. Open a Pull Request

## Development Process

### Setting Up Your Development Environment

```bash
# Clone your fork
git clone https://github.com/your-username/gorest.git

# Navigate to the project directory
cd gorest

# Install dependencies
go mod download
```

### Testing

GoRest v2.0 has comprehensive testing requirements. Please ensure all tests pass before submitting a PR:

```bash
# Run all unit tests
go test ./...

# Run tests with race condition detection
go test -race ./...

# Run benchmarks
go test -bench=. -benchmem ./...
```

#### Test Structure
- **Unit Tests**: `*_test.go` files for individual components
- **Integration Tests**: End-to-end functionality testing
- **Benchmark Tests**: Performance regression testing with `Benchmark*` functions
- **Concurrent Safety Tests**: Race condition validation

### Coding Standards

- Follow Go's official [style guide](https://github.com/golang/go/wiki/CodeReviewComments)
- Write clear, comprehensive docstrings explaining the "why" not just the "what"
- Include tests for new features with both unit and integration coverage
- Ensure thread safety for concurrent usage
- Add benchmark tests for performance-critical features
- Use structured logging with appropriate levels

### Performance Considerations

When contributing to GoRest, please consider:

- **Memory Efficiency**: Avoid unnecessary allocations in hot paths
- **Concurrency**: Ensure new features are thread-safe
- **Backward Compatibility**: Maintain API compatibility unless it's a breaking change
- **Documentation**: Update README.md and add godoc comments
- **Benchmarks**: Add benchmarks for performance-sensitive code

### DAG Workflow Development Guidelines

When working on the DAG workflow system:

#### Design Principles
- **Dependency Clarity**: Ensure dependencies are explicit and form a valid DAG (no cycles)
- **Parallel Efficiency**: Maximize opportunities for parallel execution
- **Error Isolation**: Failed steps should not affect unrelated parallel branches
- **Resource Management**: Properly manage goroutines and prevent resource leaks

#### Implementation Guidelines
```go
// Good: Clear dependencies that allow parallelism
steps := []*DAGWorkflowStep{
    {Name: "init"},
    {Name: "fetchA", Dependencies: []string{"init"}},
    {Name: "fetchB", Dependencies: []string{"init"}},
    {Name: "combine", Dependencies: []string{"fetchA", "fetchB"}},
}

// Avoid: Unnecessary sequential dependencies
steps := []*DAGWorkflowStep{
    {Name: "step1"},
    {Name: "step2", Dependencies: []string{"step1"}},
    {Name: "step3", Dependencies: []string{"step2"}},
}
```

#### Testing Requirements
- **Dependency Resolution**: Test various DAG topologies (linear, diamond, fork-join)
- **Parallel Execution**: Verify concurrent step execution with race detection
- **Error Propagation**: Test StopOnError behavior and partial failures
- **Conditional Logic**: Validate step conditions and skipping behavior
- **Performance**: Benchmark parallel vs sequential execution

#### Common Patterns
1. **Fork-Join**: Multiple parallel paths converging
2. **Diamond**: Split and merge pattern
3. **Pipeline**: Sequential processing with parallel branches
4. **Conditional Branching**: Different paths based on conditions

## License

By contributing to Gorest, you agree that your contributions will be licensed under the project's [MIT License](LICENSE).
