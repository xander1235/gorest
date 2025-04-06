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

Please ensure all tests pass before submitting a PR:

```bash
go test ./...
```

### Coding Standards

- Follow Go's official [style guide](https://github.com/golang/go/wiki/CodeReviewComments)
- Write clear, commented code
- Include tests for new features

## License

By contributing to Gorest, you agree that your contributions will be licensed under the project's [MIT License](LICENSE).
