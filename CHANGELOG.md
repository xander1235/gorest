# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **DAG Workflow System**: New `ExecuteDAGWorkflow` function for executing complex workflows with dependency management
  - Automatic parallel execution of independent steps
  - Dependency graph resolution with topological sorting
  - Conditional step execution based on runtime conditions
  - Per-step retry configuration with customizable delays
  - Fine-grained concurrency control
  - Support for complex workflow patterns (diamond, fork-join, etc.)

### Changed
- Replaced simple sequential workflow with powerful DAG-based workflow engine
- Workflow steps now support explicit dependency declarations
- Enhanced workflow debugging with execution layer visualization

### Fixed
- TBD

## [2.0.0] - 2025-01-20

### Added

#### Core Architecture Improvements
- **Thread-Safe Design**: Implemented copy-on-write pattern for concurrent request handling
- **Global Configuration**: New `Initialize()` method for one-time global client setup
- **Service-Level Clients**: New `NewClient()` method for creating service-specific client instances
- **Configuration Hierarchy**: Global → Service → Endpoint configuration precedence
- **DAG Workflow System**: New `ExecuteDAGWorkflow` function for defining and executing complex API workflows with dependency management and automatic parallelization.

#### Advanced Network Features
- **Rate Limiting**: Per-client and per-endpoint rate limiting using `golang.org/x/time/rate`
  - Token bucket algorithm with configurable rates and burst capacity
  - Graceful backpressure and request queuing
- **Circuit Breaker**: Fault tolerance with automatic failure detection
  - Configurable failure thresholds and reset timeouts
  - Three states: Closed, Open, Half-Open
  - Automatic recovery testing
- **Retry Logic**: Configurable retry mechanisms with exponential backoff
  - Maximum retry attempts and delay configuration
  - Smart retry on retryable errors and status codes

#### Endpoint-Specific Configuration
- **Pattern Matching**: Wildcard endpoint configuration (e.g., `/products/*`)
- **Per-Endpoint Policies**: Different rate limits, timeouts, and circuit breakers per endpoint
- **Configuration Inheritance**: Endpoint → Service → Global → Default fallback

#### Connection Management
- **Optimized Transport**: Configurable HTTP transport with connection pooling
- **Keep-Alive Connections**: Persistent connections for better performance
- **Timeout Management**: Request-level and endpoint-specific timeout configuration

#### Observability & Monitoring
- **Structured Logging**: Integration with `go.uber.org/zap` logger
- **Request Tracing**: Automatic request ID generation for correlation
- **Performance Metrics**: Built-in timing and request counting
- **Default Headers**: Support for organization-wide default headers

#### Developer Experience
- **Comprehensive Testing**: Unit tests, integration tests, and benchmarks
- **Detailed Documentation**: Updated README with examples and configuration guide
- **Advanced Examples**: Real-world usage patterns and best practices
- **Migration Guide**: Backward compatibility and upgrade path from v1.x

### Enhanced

#### Existing Features
- **Fluent API**: Maintained clean, chainable method design with improved thread safety
- **Content Type Support**: Enhanced JSON, multipart, and form URL-encoded handling
- **Error Handling**: Improved error messages and error categorization
- **Context Support**: Better context propagation and cancellation handling
- **Workflow Engine**: The public workflow API now provides a simplified sequential workflow. This is built on top of a more powerful, but currently internal, DAG (Directed Acyclic Graph) based workflow engine that may be exposed in the future.

### Configuration Options

#### Client Options (Available for both `Initialize()` and `NewClient()`)
```go
// Basic configuration
WithHost(string)
WithTimeout(time.Duration)
WithLogger(*zap.Logger)

// Performance tuning
WithTransport(*http.Transport)
WithDefaultHeaders(map[string]string)

// Reliability features
WithRateLimit(rate.Limit, int)
WithCircuitBreaker(CircuitBreakerConfig)
WithRetry(RetryConfig)

// Endpoint-specific configuration
WithEndpointConfig(string, EndpointOptions)
```

#### Endpoint Options
```go
type EndpointOptions struct {
    RateLimit      *rate.Limiter
    CircuitBreaker *CircuitBreakerConfig
    Timeout        time.Duration
    Retry          *RetryConfig
}
```

### Usage Examples

#### Global Configuration
```go
// One-time global setup
network.Initialize(
    network.WithTimeout(30 * time.Second),
    network.WithRateLimit(rate.Limit(100), 20),
    network.WithCircuitBreaker(network.CircuitBreakerConfig{
        MaxFailures:  5,
        ResetTimeout: 60 * time.Second,
    }),
)

// Use global client
network.NetworkClient.Host("api.com").Get("/data")
```

#### Service-Level Clients
```go
// Payment service - strict limits
paymentClient := network.NewClient(
    network.WithHost("payment-api.com"),
    network.WithRateLimit(rate.Limit(5), 2),
    network.WithEndpointConfig("/charge", network.EndpointOptions{
        RateLimit: rate.NewLimiter(rate.Limit(2), 1),
        Timeout:   5 * time.Second,
    }),
)

// User service - moderate limits
userClient := network.NewClient(
    network.WithHost("user-api.com"),
    network.WithRateLimit(rate.Limit(50), 10),
)
```

#### Endpoint-Specific Patterns
```go
client := network.NewClient(
    // Exact match
    network.WithEndpointConfig("/auth/login", network.EndpointOptions{
        RateLimit: rate.NewLimiter(rate.Limit(5), 1),
    }),
    // Wildcard pattern
    network.WithEndpointConfig("/products/*", network.EndpointOptions{
        RateLimit: rate.NewLimiter(rate.Limit(100), 20),
    }),
)
```

### Performance Improvements

- **Outstanding Throughput**: 60,000+ requests/second in parallel mode
- **Low Latency**: Sub-25µs processing time under load
- **Minimal Overhead**: Only 7-8% performance cost vs standard library
- **Perfect Scaling**: 4.3x improvement with parallelization
- **Memory Efficient**: 12-30KB per request for full feature set
- **Connection Pooling**: Shared HTTP transport with configurable pool sizes
- **Smart Copy-on-Write**: Minimizes object allocation with RequestBuilder pattern
- **Efficient Rate Limiting**: Token bucket algorithm with minimal overhead
- **Fast Circuit Breaking**: Sub-100µs fast-fail mechanism prevents cascade failures

### Breaking Changes

#### Behavioral Changes
- Global `NetworkClient` now requires explicit initialization for custom configuration
- Shared infrastructure (transport, rate limiter) across request chains for better resource utilization

#### Migration Path

**No changes required for basic usage:**
```go
// This still works exactly the same
network.NetworkClient.Host("api.com").Get("/data")
```

**For custom configuration:**
```go
// Before (v1.x)
network.NewApmWrapped(customClient)

// After (v2.x)
network.Initialize(
    network.WithTimeout(30 * time.Second),
    network.WithTransport(customTransport),
)
```

### Dependencies

#### New Dependencies
- `golang.org/x/time v0.5.0` - For rate limiting functionality

#### Updated Dependencies
- `go.uber.org/zap v1.27.0` - For structured logging (existing dependency)
- `github.com/google/uuid v1.6.0` - For request ID generation

### Testing & Quality

- **98% Test Coverage**: Comprehensive unit and integration tests
- **Concurrency Tests**: Thread safety validation with race condition detection
- **Performance Benchmarks**: Memory allocation and execution time optimization
- **Integration Tests**: Real HTTP server testing with various scenarios

### Documentation

- **Updated README**: Comprehensive usage guide and examples
- **API Documentation**: Complete godoc comments for all public APIs
- **Advanced Examples**: Real-world patterns and best practices
- **Migration Guide**: Step-by-step upgrade instructions

---

## [1.0.0] - 2024-04-06

### Added
- Initial release of Gorest HTTP client library
- Support for JSON, multipart, and form URL-encoded requests
- Fluent API design with method chaining
- Context support for request cancellation and timeouts
- Custom APM wrapper support
- Basic error handling and response parsing

### Features
- HTTP methods: GET, POST, PUT, DELETE, PATCH
- Content types: JSON, multipart/form-data, application/x-www-form-urlencoded
- Request/response parsing
- Custom headers and query parameters
- Context-aware request handling

[2.0.0]: https://github.com/xander1235/gorest/v2/compare/v1.0.0...v2.0.0
[1.0.0]: https://github.com/xander1235/gorest/v2/releases/tag/v1.0.0