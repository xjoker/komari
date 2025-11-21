# Testing Strategy

This document outlines Komari's testing philosophy and guidelines for writing meaningful tests.

## Philosophy

**Quality over Quantity** - We focus on tests that provide real value, not just code coverage metrics.

### What Makes a Good Test?

1. **Tests Business Logic** - Validates critical functionality, not just API existence
2. **Catches Real Bugs** - Would fail if the code breaks
3. **Easy to Maintain** - Clear, concise, and doesn't require frequent updates
4. **Fast to Execute** - Runs quickly in CI/CD pipeline
5. **Reproducible** - Same input always produces same output

### What We Avoid

❌ **Tests that only check if code exists** (e.g., "does this function return non-nil?")
❌ **Tests that just call functions without assertions**
❌ **Mocks that exist only to satisfy test coverage**
❌ **Integration tests without clear business value**
❌ **Tests that duplicate what the compiler already checks**

## Test Categories

### 1. Unit Tests

**Purpose**: Test individual functions with clear inputs/outputs

**Good Examples**:
- RPC parameter binding (`utils/rpc/rpc_test.go`)
- GeoIP emoji conversion (`utils/geoip/geoip_test.go`)
- Data transformation functions

**Guidelines**:
```go
// ✅ Good: Tests specific behavior with assertions
func TestBindParamsPositionalStruct(t *testing.T) {
    req := &JsonRpcRequest{Params: []any{1, 2}}
    var p Pair
    if err := req.BindParams(&p); err != nil {
        t.Fatalf("BindParams failed: %v", err)
    }
    if p.First != 1 || p.Second != 2 {
        t.Fatalf("unexpected struct values: %+v", p)
    }
}

// ❌ Bad: Just checks if function returns something
func TestMessageSenders(t *testing.T) {
    senders := factory.GetAllMessageSenders()
    if len(senders) == 0 {
        t.Error("No message senders found")
    }
}
```

### 2. Integration Tests

**Purpose**: Test multiple components working together with real dependencies

**Good Examples**:
- Authentication flow (`api/login_test.go`, `api/me_test.go`)
- Database operations (`database/records/records_test.go`)
- Critical business logic with state

**Guidelines**:
```go
// ✅ Good: Tests complete user flow with real database
func TestLogin(t *testing.T) {
    accounts.CreateAccount("testuser", "correctpassword")

    tests := []struct {
        name           string
        requestBody    LoginRequest
        expectedStatus int
    }{
        {
            name: "成功登录",
            requestBody: LoginRequest{
                Username: "testuser",
                Password: "correctpassword",
            },
            expectedStatus: http.StatusOK,
        },
        // ... more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation with assertions
        })
    }

    // Cleanup
    accounts.DeleteAccountByUsername("testuser")
}

// ❌ Bad: Starts process without verification
func TestRunCloudflared(t *testing.T) {
    err := cloudflared.RunCloudflared()
    if err != nil {
        t.Fatalf("RunCloudflared failed: %v", err)
    }
    time.Sleep(2 * time.Second) // Does nothing else
}
```

### 3. End-to-End Tests

**Purpose**: Test complete user journeys through the system

**Current Status**: We rely on manual testing for E2E scenarios

**Future**: Consider adding critical path E2E tests:
- Client metrics submission → storage → retrieval
- Admin login → configuration change → effect visible
- Alert trigger → notification delivery

## Test Organization

### File Structure
```
package/
├── implementation.go       # Main code
├── implementation_test.go  # Tests for this package
└── testdata/              # Test fixtures (if needed)
```

### Naming Conventions
```go
// Test function naming
func TestFunctionName(t *testing.T)           // Tests FunctionName()
func TestFunctionName_EdgeCase(t *testing.T)  // Specific scenario

// Table-driven tests
func TestAuthentication(t *testing.T) {
    tests := []struct {
        name           string
        input          InputType
        expectedOutput OutputType
        expectedError  bool
    }{
        {name: "valid input", input: ..., expectedOutput: ..., expectedError: false},
        {name: "invalid input", input: ..., expectedOutput: ..., expectedError: true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

## What to Test

### ✅ Always Test

1. **Critical Business Logic**
   - Data compression/aggregation (`records.CompactRecord()`)
   - Authentication and authorization
   - Rate limiting enforcement
   - Data validation and sanitization

2. **Edge Cases**
   - Empty inputs
   - Boundary values (0, -1, MAX_INT)
   - Nil pointers
   - Concurrent access scenarios

3. **Error Handling**
   - Database connection failures
   - Invalid user input
   - Network timeouts
   - Resource exhaustion

4. **Data Transformations**
   - Protobuf serialization/deserialization
   - JSON encoding/decoding
   - Time zone conversions
   - Unit conversions (bytes → MB)

### ❌ Don't Test

1. **Third-Party Libraries**
   - Don't test that GORM works
   - Don't test that zerolog formats JSON correctly
   - Trust well-maintained dependencies

2. **Trivial Getters/Setters**
   ```go
   // ❌ Don't test this
   func (c *Client) GetUUID() string {
       return c.UUID
   }
   ```

3. **Compiler-Checked Behavior**
   - Type safety
   - Interface compliance (unless testing specific behavior)

4. **Configuration Loading**
   - Unless complex validation logic exists
   - Environment variable reading is straightforward

## Mocking Strategy

### When to Mock

**Mock external services**:
- HTTP APIs (use `httptest.NewServer()`)
- Time-dependent operations (inject time source)
- File system operations (use interfaces)
- Database (only for unit tests; prefer real DB for integration)

### When NOT to Mock

**Don't mock for test coverage sake**:
```go
// ❌ Bad: Unnecessary mock just to test
type MockOAuthProvider struct{}
func (m *MockOAuthProvider) GetConfig() Config { return Config{} }

func TestOAuthExists(t *testing.T) {
    provider := &MockOAuthProvider{}
    if provider.GetConfig() == nil {
        t.Error("Config is nil")
    }
}
```

**Use real implementations when possible**:
```go
// ✅ Good: Use real in-memory database
func setupTestDB(t *testing.T) *gorm.DB {
    db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
    require.NoError(t, err)
    return db
}
```

## Test Environment

### Local Development
```bash
# Run all tests
go test ./...

# Run specific package
go test ./database/records

# Run with coverage
go test -cover ./...

# Verbose output
go test -v ./api
```

### CI/CD Integration
- Tests run on every pull request
- Must pass before merge
- Coverage reporting (but not a gate)

### Optional Integration Tests

Some tests require external services (PostgreSQL):
```bash
# Enable PostgreSQL integration tests
export TEST_POSTGRES_ENABLED=true
export TEST_POSTGRES_HOST=localhost
export TEST_POSTGRES_PORT=5432
export TEST_POSTGRES_USER=postgres
export TEST_POSTGRES_PASSWORD=password
export TEST_POSTGRES_DB=komari_test

go test ./database/records
```

## Test Data Management

### Cleanup

**Always clean up test data**:
```go
func TestUserCreation(t *testing.T) {
    user, _ := accounts.CreateAccount("test", "pass")
    defer accounts.DeleteAccountByUsername("test") // Cleanup

    // Test assertions
}
```

### Test Isolation

**Tests must be independent**:
- Don't rely on execution order
- Don't share state between tests
- Clean up before and after

```go
func TestDatabaseOperations(t *testing.T) {
    db := setupTestDB(t)
    defer cleanupTestDB(t, db) // Ensure cleanup

    // Test implementation
}
```

## Removed Tests

The following tests were removed for lack of value:

1. **`utils/messageSender/loader_test.go`**
   - Only checked if senders list was non-empty
   - No actual behavior testing
   - Trivial validation

2. **`utils/cloudflared/cloudflared_test.go`**
   - Just started process and waited
   - No verification of behavior
   - Not reproducible (relies on external binary)

3. **`utils/oauth/factory_test.go`**
   - Only checked if providers list existed
   - No actual OAuth flow testing
   - Superficial validation

## Best Practices

### 1. Use Table-Driven Tests

```go
func TestRateLimiting(t *testing.T) {
    tests := []struct {
        name       string
        requests   int
        limit      int
        shouldFail bool
    }{
        {"under limit", 50, 100, false},
        {"at limit", 100, 100, false},
        {"over limit", 101, 100, true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Test implementation
        })
    }
}
```

### 2. Use testify/assert

```go
import "github.com/stretchr/testify/assert"

assert.Equal(t, expected, actual, "values should match")
assert.NoError(t, err)
assert.True(t, condition)
```

### 3. Use subtests for Organization

```go
func TestComplexFeature(t *testing.T) {
    t.Run("ValidInput", func(t *testing.T) {
        // Test valid input
    })

    t.Run("InvalidInput", func(t *testing.T) {
        // Test invalid input
    })

    t.Run("EdgeCases", func(t *testing.T) {
        // Test edge cases
    })
}
```

### 4. Test Error Messages

```go
err := ValidateInput(invalidData)
assert.Error(t, err)
assert.Contains(t, err.Error(), "invalid format")
```

### 5. Use Fixtures for Complex Data

```go
//go:embed testdata/valid_report.json
var validReportJSON string

func TestReportParsing(t *testing.T) {
    report, err := ParseReport(validReportJSON)
    assert.NoError(t, err)
    // assertions
}
```

## Coverage Goals

**Target**: 70-80% coverage for critical paths

**Don't obsess over 100% coverage** - Focus on:
- Core business logic: 90%+
- API handlers: 80%+
- Utilities: 70%+
- Error handling: Test all error paths

**Coverage != Quality** - A well-tested critical path is better than 100% coverage of trivial code.

## Future Improvements

1. **Performance Tests** - Benchmark critical paths (rate limiter, database queries)
2. **Load Tests** - Verify system handles 1000+ concurrent clients
3. **Security Tests** - Automated vulnerability scanning
4. **Fuzz Testing** - For input parsing (Protobuf, JSON)

## Resources

- [Go Testing Best Practices](https://golang.org/doc/code#Testing)
- [Table-Driven Tests in Go](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests)
- [testify Documentation](https://github.com/stretchr/testify)

---

**Remember**: Tests are code too. Write them with the same care as production code.
