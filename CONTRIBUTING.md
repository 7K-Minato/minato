# Contributing to Minato

Thank you for your interest in contributing to Minato! This document provides guidelines and instructions for contributing.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [Getting Started](#getting-started)
- [Development Environment](#development-environment)
- [Code Style](#code-style)
- [Testing](#testing)
- [Pull Request Process](#pull-request-process)
- [Commit Message Conventions](#commit-message-conventions)
- [Release Process](#release-process)

## Code of Conduct

This project adheres to a code of conduct. By participating, you are expected to uphold this code:
- Be respectful and inclusive
- Welcome newcomers
- Focus on constructive feedback
- Respect different viewpoints and experiences

## Getting Started

1. Fork the repository on GitHub
2. Clone your fork locally
3. Create a new branch for your feature or bug fix
4. Make your changes
5. Run tests and ensure they pass
6. Submit a pull request

## Development Environment

### Prerequisites

- Go 1.23+
- Kubernetes cluster (1.28+) or Docker with Kind
- kubectl configured
- Make

### Setup

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/minato.git
cd minato

# Install dependencies
make setup-envtest

# Verify setup
make test
```

### Running Locally

```bash
# Run the operator locally
make run-operator

# Run the control plane
./bin/controlplane

# Run integration tests
make test-integration
```

## Code Style

We follow standard Go conventions:

- Use `gofmt` for formatting
- Use `go vet` for static analysis
- Use `golangci-lint` for comprehensive linting
- Follow [Effective Go](https://go.dev/doc/effective_go) guidelines
- Write clear, idiomatic Go code

### Key Style Rules

- **Error handling**: Always check errors, wrap with context using `fmt.Errorf("...: %w", err)`
- **Logging**: Use structured logging via `sigs.k8s.io/controller-runtime/pkg/log`
- **Constants**: Define magic numbers as named constants
- **Comments**: Document all exported types, functions, and packages
- **Tests**: Write table-driven tests, aim for 80%+ coverage

## Testing

### Running Tests

```bash
# Unit tests only
make test

# Integration tests (requires envtest binaries)
make test-integration

# End-to-end tests (requires Kind cluster)
make test-e2e

# All tests with coverage
go test ./... -coverprofile=cover.out
go tool cover -html=cover.out -o coverage.html
```

### Test Requirements

- All new code must have unit tests
- Bug fixes should include regression tests
- Integration tests for controller reconciler logic
- Table-driven tests preferred
- Use `testify/assert` for assertions
- Mock external dependencies (gRPC, K8s client)

### Writing Controller Tests

```go
func TestMyReconciler(t *testing.T) {
    scheme := runtime.NewScheme()
    // Register types...
    
    cl := fake.NewClientBuilder().WithScheme(scheme).Build()
    reconciler := &MyReconciler{Client: cl, Scheme: scheme}
    
    // Test cases...
}
```

## Pull Request Process

1. **Before submitting**:
   - Run `make verify` (format, vet, test, lint)
   - Ensure all tests pass
   - Update documentation if needed
   - Add CHANGELOG entry

2. **PR Description**:
   - Clear title following conventional commits format
   - Description of changes
   - Link to related issues
   - Testing performed
   - Screenshots (if UI changes)

3. **Review Process**:
   - At least one maintainer approval required
   - Address review feedback promptly
   - Keep PRs focused and reasonably sized

4. **After Merge**:
   - Delete your branch
   - Monitor CI for any issues

## Commit Message Conventions

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
<type>(<scope>): <description>

[optional body]

[optional footer(s)]
```

### Types

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `style`: Code style changes (formatting, semicolons, etc.)
- `refactor`: Code refactoring
- `perf`: Performance improvements
- `test`: Adding or updating tests
- `chore`: Build process, dependencies, etc.

### Examples

```
feat(gameserver): add idle timeout support

fix(controller): handle missing profile gracefully

docs(readme): update installation instructions

test(actions): add catalog loading tests
```

## Release Process

1. Releases are automated via release-please
2. Version bumps follow [Semantic Versioning](https://semver.org/)
3. CHANGELOG is auto-generated from conventional commits
4. Docker images are built and pushed on release

## Questions?

- Open an issue for bugs or feature requests
- Start a discussion for questions or ideas
- Join our community chat (if available)

Thank you for contributing to Minato!
