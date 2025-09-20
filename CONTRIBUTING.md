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

## License

By contributing to Gorest, you agree that your contributions will be licensed under the project's [MIT License](LICENSE).
