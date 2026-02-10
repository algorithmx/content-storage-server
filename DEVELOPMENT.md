# Development Guide

This guide provides comprehensive information for contributors to the Content Storage Server project.

## Table of Contents

1. [Development Environment Setup](#development-environment-setup)
2. [Project Structure](#project-structure)
3. [Coding Standards](#coding-standards)
4. [Testing Guidelines](#testing-guidelines)
5. [Pull Request Process](#pull-request-process)
6. [Code Review Checklist](#code-review-checklist)
7. [Release Process](#release-process)

---

## Development Environment Setup

### Prerequisites

- Go 1.24 or later
- Docker (for containerized testing)
- git
- Make (optional, for build automation)

### Initial Setup

```bash
# Clone your fork
git clone https://github.com/YOUR_USERNAME/content-storage-server.git
cd content-storage-server

# Add upstream remote
git remote add upstream https://github.com/ORIGINAL_OWNER/content-storage-server.git

# Install dependencies
go mod download

# Run tests to verify setup
go test ./...
```

### Recommended Tools

- **GoLand** or **VSCode** with Go extension
- **dlv** for debugging (`go install github.com/go-delve/delve/cmd/dlv@latest`)
- **golangci-lint** for linting
- **swag** for Swagger doc generation

---

## Project Structure

### Directory Overview

```
content-storage-server/
├── cmd/                    # Main applications
│   ├── server/            # HTTP server application
│   └── stress-test/       # Load testing tool
├── internal/              # Private application code
│   └── handlers/          # HTTP request handlers
├── pkg/                   # Public library code
│   ├── config/            # Configuration management
│   ├── logger/            # Logging utilities
│   ├── middleware/        # HTTP middleware
│   ├── models/            # Data models
│   ├── storage/           # Storage abstraction and implementation
│   └── tls/               # TLS/HTTPS support
├── static/                # Web UI assets
├── docs/                  # Generated documentation
├── .env.example           # Configuration template
├── Dockerfile             # Container image
└── docker-compose.yml     # Development environment
```

### Module Responsibilities

#### `cmd/server/`
- Application entry point
- Server initialization and startup
- Signal handling and shutdown

#### `internal/handlers/`
- HTTP request/response handling
- Request validation
- Error response formatting

#### `pkg/storage/`
- Storage interface and implementation
- Database operations
- Queue management
- Backup and recovery

#### `pkg/config/`
- Environment variable loading
- Configuration validation
- Default value management

---

## Coding Standards

### Go Conventions

Follow [Effective Go](https://go.dev/doc/effective_go) and the standard Go style guide.

### Naming Conventions

```go
// Package names: lowercase, single word
package storage

// Constants: CamelCase or UPPER_CASE for exported
const MaxContentSize = 10 * 1024 * 1024

// Variables: camelCase
var writeQueueSize = 1000

// Functions: PascalCase for exported, camelCase for private
func NewBadgerStorage() *BadgerStorage { }
func processWrite() { }

// Interfaces: PascalCase, usually -er suffix
type Storage interface { }
type Reader interface { }
```

### Error Handling

```go
// Always handle errors
if err != nil {
    return fmt.Errorf("failed to initialize storage: %w", err)
}

// Use wrapped errors with context
if err := store.Save(content); err != nil {
    return fmt.Errorf("failed to save content %s: %w", content.ID, err)
}

// Create reusable error variables
var (
    ErrContentNotFound = errors.New("content not found")
    ErrInvalidInput    = errors.New("invalid input")
)
```

### Comments

```go
// Package comment describes what the package provides
package storage // Storage provides content persistence with BadgerDB

// Exported functions must have comments
// NewBadgerStorage creates a new BadgerDB-backed storage instance
// with the given options. It initializes the database, backup system,
// and garbage collection.
func NewBadgerStorage(opts BadgerOptions) (*BadgerStorage, error) {
    // ...
}

// Comment complex logic
// Process writes in batches for efficiency:
// 1. Accumulate writes until batch size or timeout
// 2. Process entire batch in single transaction
// 3. Repeat until queue is empty
for batch := range queue.batches {
    // ...
}
```

### Logging

```go
import "go.uber.org/zap"

// Use structured logging
logger.Info("Content stored",
    zap.String("id", content.ID),
    zap.String("type", content.Type),
    zap.Int("size", len(content.Data)),
)

// Log errors with context
logger.Error("Failed to store content",
    zap.String("id", content.ID),
    zap.Error(err),
)

// Use appropriate log levels
logger.Debug("Detailed debugging info")      // Development only
logger.Info("Normal operation messages")      // General information
logger.Warn("Warning messages")               // Potential issues
logger.Error("Error messages")                // Errors that don't stop execution
// logger.Fatal() - Use only in main(), calls os.Exit()
```

---

## Testing Guidelines

### Test Structure

```go
func TestBadgerStorage_Store(t *testing.T) {
    tests := []struct {
        name    string
        content *models.Content
        wantErr bool
    }{
        {
            name: "successful store",
            content: &models.Content{
                ID:   "test-1",
                Data: "test data",
                Type: "text/plain",
            },
            wantErr: false,
        },
        {
            name:    "nil content",
            content: nil,
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Setup
            store := setupTestStore(t)
            defer store.Close()

            // Execute
            err := store.Store(tt.content)

            // Assert
            if (err != nil) != tt.wantErr {
                t.Errorf("Store() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Test Categories

1. **Unit Tests** - Test individual functions/packages
2. **Integration Tests** - Test component interactions
3. **Functional Tests** - Test end-to-end functionality
4. **Stress Tests** - Test performance under load

### Running Tests

```bash
# All tests
go test ./...

# Verbose output
go test -v ./...

# With coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Specific package
go test ./pkg/storage/...

# Run specific test
go test -run TestBadgerStorage_Store ./pkg/storage/
```

### Test Helpers

Create reusable test helpers in `testdata` or `*_test.go` files:

```go
// setupTestStore creates a test storage instance
func setupTestStore(t *testing.T) storage.Storage {
    t.Helper()

    dir := t.TempDir()
    opts := storage.BadgerOptions{
        DataDir:   filepath.Join(dir, "badger"),
        BackupDir: filepath.Join(dir, "backups"),
    }

    store, err := storage.NewBadgerStorage(opts)
    if err != nil {
        t.Fatal(err)
    }

    return store
}
```

---

## Pull Request Process

### Before Submitting

1. **Update documentation** - Update README, comments, and docs
2. **Add tests** - Ensure new code has test coverage
3. **Run linters** - Fix any linting issues
4. **Format code** - Run `go fmt ./...`
5. **Test locally** - Verify all tests pass

### Branch Naming

```
feature/add-content-expiration
fix/resolve-queue-race-condition
docs/update-api-documentation
refactor/optimize-storage-layer
test/add-integration-tests
```

### Commit Messages

Follow [Conventional Commits](https://www.conventionalcommits.org/):

```
feat: add content expiration feature
fix: resolve race condition in access manager
docs: update README with deployment instructions
refactor: simplify queue processing logic
test: add integration tests for backup system
chore: update dependencies to latest versions
```

### Pull Request Template

```markdown
## Description
Brief description of changes

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
- [ ] Unit tests added/updated
- [ ] Integration tests added/updated
- [ ] Manual testing completed

## Checklist
- [ ] Code follows style guidelines
- [ ] Self-review completed
- [ ] Documentation updated
- [ ] No new warnings generated
- [ ] Tests pass locally
- [ ] Added/updated tests for changes

## Related Issues
Fixes #123
Related to #456
```

---

## Code Review Checklist

### Functionality
- [ ] Code implements the requested feature
- [ ] Edge cases are handled
- [ ] Error handling is comprehensive
- [ ] No memory leaks or resource issues

### Code Quality
- [ ] Code is readable and understandable
- [ ] Functions are appropriately sized
- [ ] No code duplication
- [ ] Proper use of existing patterns

### Testing
- [ ] Tests cover new functionality
- [ ] Tests cover edge cases
- [ ] Tests are not flaky
- [ ] Test names are descriptive

### Documentation
- [ ] Public functions have comments
- [ ] Complex logic is explained
- [ ] README updated if needed
- [ ] Breaking changes documented

### Performance
- [ ] No performance regressions
- [ ] Efficient algorithms used
- [ ] Appropriate caching
- [ ] Resource usage reasonable

---

## Release Process

### Version Numbering

Follow [Semantic Versioning](https://semver.org/):
- **MAJOR**: Incompatible API changes
- **MINOR**: Backwards-compatible functionality
- **PATCH**: Backwards-compatible bug fixes

### Release Steps

1. **Update version** in `go.mod` if needed
2. **Update CHANGELOG** with all changes
3. **Create release tag**:
   ```bash
   git tag -a v1.2.3 -m "Release v1.2.3"
   git push origin v1.2.3
   ```
4. **Create GitHub release** with changelog
5. **Build and push Docker image**:
   ```bash
   docker build -t content-storage-server:v1.2.3 .
   docker tag content-storage-server:v1.2.3 content-storage-server:latest
   docker push content-storage-server:v1.2.3
   docker push content-storage-server:latest
   ```

### Changelog Format

```markdown
## [1.2.3] - 2025-02-10

### Added
- Content expiration feature
- Access limit enforcement

### Changed
- Improved queue processing performance
- Updated BadgerDB to v4.7.0

### Fixed
- Race condition in access manager
- Memory leak in cleanup process

### Security
- Added input sanitization for content IDs
```

---

## Getting Help

### Resources

- **[On-boarding Guide](ONBOARDING.md)** - Start here for new developers
- **[Main README](README.md)** - Project overview and usage
- **[Storage Docs](pkg/storage/README.md)** - Storage layer details
- **[Handler Docs](internal/handlers/README.md)** - API documentation

### Asking Questions

1. Check existing documentation first
2. Search existing GitHub issues
3. Create detailed issue with:
   - Steps to reproduce
   - Expected behavior
   - Actual behavior
   - Environment details

### Contributing

We welcome contributions! Please:
1. Read this development guide
2. Check existing issues for work to do
3. Follow the pull request process
4. Be patient with code review

---

**Happy coding! 🚀**
