# Contributing to opd-ai/store

Thank you for your interest in contributing to opd-ai/store! This document provides guidelines and instructions for contributing.

## Code of Conduct

Be respectful, inclusive, and professional in all interactions.

## Getting Started

### Fork & Clone

```bash
# Fork the repository on GitHub
git clone https://github.com/your-username/store.git
cd store
git remote add upstream https://github.com/opd-ai/store.git
```

### Set Up Development Environment

```bash
# Install dependencies
go mod download

# Copy environment template
cp .env.example .env

# Start services
docker-compose up -d

# Run tests
make test
```

## Development Workflow

### 1. Create a Feature Branch

```bash
git checkout -b feature/descriptive-name
```

Branch naming conventions:
- `feature/` - New features
- `fix/` - Bug fixes
- `docs/` - Documentation
- `refactor/` - Code refactoring
- `test/` - Test improvements

### 2. Make Changes

Follow Go best practices:
- Write clear, idiomatic Go code
- Add comments for exported functions
- Keep functions small and focused
- Use meaningful variable names

### 3. Run Quality Checks

```bash
make check  # Runs fmt, lint, vet, and test
```

Or individually:
```bash
make fmt    # Format code
make vet    # Run go vet
make lint   # Run golangci-lint
make test   # Run tests
```

### 4. Write Tests

- Add unit tests for new functions
- Add integration tests for handler implementations
- Aim for >80% code coverage

```bash
# Run tests with coverage
make test-coverage
```

### 5. Commit Changes

```bash
git add .
git commit -m "feat: add new handler type"
```

Use conventional commits:
- `feat:` - New feature
- `fix:` - Bug fix
- `docs:` - Documentation
- `refactor:` - Code refactoring
- `test:` - Test additions/changes
- `chore:` - Maintenance, dependencies

### 6. Push & Open Pull Request

```bash
git push origin feature/descriptive-name
```

Open a pull request with:
- Clear title and description
- Reference to related issues
- Screenshots (if UI changes)
- Testing instructions

## Adding a New Handler

### Structure Example

```go
// internal/handlers/my_handler.go
package handlers

import (
    "context"
    "github.com/opd-ai/store/pkg/handler"
    "github.com/opd-ai/store/pkg/models"
)

// MyHandler executes custom fulfillment logic.
type MyHandler struct {}

// Handle implements FulfillmentHandler.
func (h *MyHandler) Handle(ctx context.Context, payment *models.Payment, item *models.Item) (map[string]interface{}, error) {
    // Your fulfillment logic
    return map[string]interface{}{...}, nil
}

// Validate implements FulfillmentHandler.
func (h *MyHandler) Validate(config models.JSONMap) error {
    // Validate handler configuration
    return nil
}

// Metadata implements FulfillmentHandler.
func (h *MyHandler) Metadata() handler.HandlerMetadata {
    return handler.HandlerMetadata{
        Type: "my_handler",
        DisplayName: "My Custom Handler",
        Description: "...",
        RequiredFields: []handler.Field{...},
        OptionalFields: []handler.Field{...},
    }
}
```

### Testing

```go
// test/handlers_test.go
func TestMyHandler(t *testing.T) {
    handler := handlers.NewMyHandler()
    
    // Test validation
    config := models.JSONMap{...}
    if err := handler.Validate(config); err != nil {
        t.Errorf("validation failed: %v", err)
    }
    
    // Test fulfillment
    ctx := context.Background()
    payment := &models.Payment{Status: "confirmed"}
    item := &models.Item{BackendConfig: config}
    
    result, err := handler.Handle(ctx, payment, item)
    if err != nil {
        t.Errorf("Handle failed: %v", err)
    }
    
    // Verify result structure
    if _, ok := result["status"]; !ok {
        t.Error("expected 'status' in result")
    }
}
```

### Register in main.go

```go
// cmd/store/main.go
func registerHandlers(registry *handler.Registry) error {
    handlersToRegister := []handler.FulfillmentHandler{
        handlers.NewDigitalMediaHandler(),
        handlers.NewShippingFormHandler(),
        handlers.NewPrintOnDemandHandler(),
        handlers.NewCustomHandler(),
        handlers.NewMyHandler(),  // Add here
    }
    // ...
}
```

## Code Style

### Naming
- Use `CamelCase` for function/type names
- Use `snake_case` for database columns
- Use `kebab-case` for URLs and config keys

### Comments
- Explain the "why", not the "what"
- Use `//` for single-line comments
- Use `/* */` for multi-line blocks
- Write godoc comments for exported functions

Example:
```go
// FetchUserByID retrieves a user from the database by their ID.
// Returns an error if the user is not found.
func FetchUserByID(id string) (*User, error) {
    // ...
}
```

### Error Handling
```go
// Check for errors immediately
if err != nil {
    return nil, fmt.Errorf("failed to do X: %w", err)
}

// Use wrapped errors for context
```

### Dependencies
- Minimize external dependencies
- Use standard library when possible
- Document why each dependency is needed

## Testing Best Practices

1. **Table-driven tests** for multiple scenarios:
```go
tests := []struct {
    name    string
    input   string
    want    string
    wantErr bool
}{
    {"valid", "test", "result", false},
    {"invalid", "", "", true},
}
```

2. **Mocks for external services**:
Use httptest for HTTP mocks, implement interfaces for logic mocks.

3. **Isolation**:
Run tests in isolation, don't rely on external services in unit tests.

4. **Assertions**:
Use clear assertions that fail with helpful messages.

### Test Database

Integration tests use BoltDB for fast embedded database testing.

**Why BoltDB for tests?**
- Fast in-memory operation using temporary files
- No external dependencies required (pure Go)
- Clean state for each test run via temp directories
- Same database used in production (realistic testing)

**Running tests:**
```bash
# Standard test run
go test ./...

# With coverage
go test -coverprofile=coverage.out ./...

# Skip integration tests
go test -short ./...
```

**Important:** Tests use the same BoltDB implementation as production, ensuring consistency between test and production behavior.

## Documentation

- Update DESIGN.md for architecture changes
- Update ARCHITECTURE.md for system flow changes
- Include inline code comments for complex logic
- Add API documentation in README.md for new endpoints

## Performance Considerations

- Use prepared statements for database queries (GORM handles this)
- Implement connection pooling for databases
- Cache frequently accessed data
- Use indexes on frequently queried fields

## Security

- Never commit secrets or API keys
- Use environment variables for sensitive config
- Validate and sanitize user input
- Use parameterized queries (GORM protects against SQL injection)
- Add HTTPS support for production

## Pull Request Checklist

- [ ] Code follows style guidelines
- [ ] Tests pass: `make test`
- [ ] Coverage maintained or improved
- [ ] Linting passes: `make lint`
- [ ] Documentation updated
- [ ] Commit messages follow conventions
- [ ] No breaking changes documented (or clearly marked)

## Review Process

1. Automated checks must pass (tests, lint, vet)
2. At least one maintainer review required
3. Maintainers may request changes
4. Once approved, PR can be merged

## Troubleshooting

### Tests failing locally but passing in CI
- Ensure you're using the same Go version
- Check environment variables
- Run `go mod tidy` and rebuild

### Lint errors
- Run `make fmt` to auto-fix formatting
- Check golangci-lint configuration in `.golangci.yml`

### Build issues
- Run `go mod download`
- Run `go clean -modcache`
- Rebuild: `make clean && make build`

## Questions?

- Open an issue for questions
- Check existing issues/discussions
- Read the design documents (DESIGN.md, ARCHITECTURE.md)

## License

By contributing, you agree that your contributions will be licensed under the same license as the project.

Thank you for contributing!
