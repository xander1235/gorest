# Gorest v2.0

[![Go Reference](https://pkg.go.dev/badge/github.com/xander1235/gorest.svg)](https://pkg.go.dev/github.com/xander1235/gorest)
[![Go Report Card](https://goreportcard.com/badge/github.com/xander1235/gorest)](https://goreportcard.com/report/github.com/xander1235/gorest)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Performance](https://img.shields.io/badge/Performance-60K%2B%20ops%2Fsec-brightgreen)](https://github.com/xander1235/gorest#-performance)

## Overview
Gorest is a powerful, production-ready Go HTTP client library with advanced features like rate limiting, circuit breaker, retry mechanisms, and endpoint-specific configurations. It supports JSON, multipart, and form URL-encoded request types with a clean, fluent API.

**🚀 Performance**: 60,000+ requests/second with sub-25µs latency and only 7-8% overhead vs standard library.

## 🚀 Features

### Core Features
- **Thread-Safe**: Copy-on-write pattern for concurrent requests
- **Fluent API**: Clean, chainable method calls
- **Multiple Content Types**: JSON, multipart, form URL-encoded
- **Context Support**: Request cancellation and timeouts
- **APM Integration**: Custom HTTP client wrapper support

### Advanced Features
- **Endpoint Isolation**: Independent rate limiting and circuit breaking per endpoint
- **Circuit Breaker**: Fault tolerance with failure detection
- **Retry Logic**: Configurable retry with exponential backoff
- **Endpoint-Specific Config**: Different policies for different endpoints
- **Connection Pooling**: Optimized HTTP transport configuration
- **Request/Response Logging**: Structured logging with zap
- **Pattern Matching**: Wildcard endpoint configuration

### Configuration Levels
1. **Global Configuration**: Organization-wide defaults
2. **Service-Level Configuration**: Per-service client instances  
3. **Endpoint-Specific Configuration**: Fine-grained endpoint policies

## 📦 Installation

```bash
go get -u github.com/xander1235/gorest
```

## 🏁 Quick Start

### Basic Usage (No Configuration Required)
```go
package main

import \"github.com/xander1235/gorest\"

type User struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}

func main() {
    var user User
    
    // Works out of the box with sensible defaults
    err := gorest.Client.
        Host("https://api.example.com").
        Headers(map[string]string{"Authorization": "Bearer token"}).
        Response(&user).
        Get("/users/1")
        
    if err != nil {
        panic(err)
    }
}
```

## ⚙️ Configuration

### 1. Global Configuration (One-time Setup)

```go
func main() {
    // Initialize global defaults once at application startup
    gorest.Initialize(
        gorest.WithTimeout(30 * time.Second),
        gorest.WithRateLimit(rate.Limit(100), 20), // 100 req/sec, burst 20
        gorest.WithTransport(&http.Transport{
            MaxIdleConns:        200,
            MaxIdleConnsPerHost: 20,
            IdleConnTimeout:     90 * time.Second,
        }),
        gorest.WithLogger(zapLogger),
        gorest.WithCircuitBreaker(gorest.CircuitBreakerConfig{
            MaxFailures:  5,
            ResetTimeout: 60 * time.Second,
        }),
        gorest.WithRetry(gorest.RetryConfig{
            MaxRetries: 3,
            BaseDelay:  100 * time.Millisecond,
            MaxDelay:   5 * time.Second,
        }),
    )
    
    // Subsequent Initialize calls are ignored
    gorest.Initialize(/* ignored */)
    
    // Use global client
    gorest.Client.Host("https://api.com").Get("/data")
}
```

### 2. Service-Level Clients

```go
type APIClients struct {
    Payment  *gorest.NetworkClient
    User     *gorest.NetworkClient
    Internal *gorest.NetworkClient
}

func NewAPIClients() *APIClients {
    return &APIClients{
        // Payment service - strict limits
        Payment: gorest.NewClient(
            gorest.WithHost("https://payment.api.com"),
            gorest.WithRateLimit(rate.Limit(5), 2),
            gorest.WithCircuitBreaker(gorest.CircuitBreakerConfig{
                MaxFailures:  2,
                ResetTimeout: 30 * time.Second,
            }),
        ),
        
        // User service - moderate limits
        User: gorest.NewClient(
            gorest.WithHost("https://user.api.com"),
            gorest.WithRateLimit(rate.Limit(50), 10),
        ),
        
        // Internal service - no limits
        Internal: gorest.NewClient(
            gorest.WithHost("https://internal.api.com"),
            gorest.WithTimeout(60 * time.Second),
            // No rate limiting for internal services
        ),
    }
}
```

### 3. Endpoint-Specific Configuration

```go
client := gorest.NewClient(
    gorest.WithHost("https://api.example.com"),
    gorest.WithRateLimit(rate.Limit(50), 10), // Default for all endpoints
    
    // Endpoint-specific overrides
    gorest.WithEndpointConfig("/auth/login", gorest.EndpointOptions{
        RateLimit: rate.NewLimiter(rate.Limit(5), 1), // Stricter for login
        Timeout:   5 * time.Second,
        CircuitBreaker: &gorest.CircuitBreakerConfig{
            MaxFailures:  2,
            ResetTimeout: 30 * time.Second,
        },
    }),
    
    // Wildcard pattern matching
    gorest.WithEndpointConfig("/products/*", gorest.EndpointOptions{
        RateLimit: rate.NewLimiter(rate.Limit(100), 20), // Higher for products
        Timeout:   10 * time.Second,
    }),
    
    gorest.WithEndpointConfig("/admin/*", gorest.EndpointOptions{
        RateLimit: rate.NewLimiter(rate.Limit(10), 2), // Very strict for admin
    }),
)
```

## 📋 Usage Examples

### JSON Request
```go
type CreateUserRequest struct {
    Name  string `json:"name"`
    Email string `json:"email"`
}

type UserResponse struct {
    ID   int    `json:"id"`
    Name string `json:"name"`
}

func createUser() {
    request := CreateUserRequest{
        Name:  "John Doe",
        Email: "john@example.com",
    }
    
    var response UserResponse
    
    err := client.Host("https://api.example.com").
        Headers(map[string]string{
            "Authorization": "Bearer token",
            "Content-Type":  "application/json",
        }).
        Body(request).
        Response(&response).
        WithContext(context.Background()).
        Post("/users")
        
    if err != nil {
        log.Printf("Error: %v", err)
        return
    }
    
    fmt.Printf("Created user: %+v", response)
}
```

### Multipart Request
```go
func uploadFile() {
    multipartBody := &types.MultipartBody{}
    multipartBody.Add("name", "John Doe")
    multipartBody.Add("email", "john@example.com")
    // multipartBody.AddFile("avatar", fileHandle)
    
    err := client.Host("https://api.example.com").
        Headers(map[string]string{"Authorization": "Bearer token"}).
        MultipartBody(multipartBody).
        Post("/users/upload")
        
    if err != nil {
        log.Printf("Upload failed: %v", err)
    }
}
```

### Form URL-Encoded Request
```go
func submitForm() {
    err := client.Host("https://api.example.com").
        Headers(map[string]string{"Authorization": "Bearer token"}).
        Body(map[string]string{
            "username": "john",
            "password": "secret",
        }).
        RequestType(enums.FormUrlEncoded).
        Post("/auth/login")
        
    if err != nil {
        log.Printf("Login failed: %v", err)
    }
}
```

### Concurrent Usage
```go
func testConcurrentRequests() {
    clients := NewAPIClients()
    
    // Multiple goroutines can safely use the same client
    for i := 0; i < 10; i++ {
        go func(id int) {
            var user User
            err := clients.User.
                Headers(map[string]string{"User-ID": fmt.Sprintf("%d", id)}).
                Response(&user).
                Get(fmt.Sprintf("/users/%d", id))
                
            if err != nil {
                log.Printf("Goroutine %d error: %v", id, err)
            } else {
                log.Printf("Goroutine %d success: %s", id, user.Name)
            }
        }(i)
    }
}
```

## 🔧 Configuration Options

### Client Options (Available for both Initialize and NewClient)

```go
// Basic Configuration
gorest.WithHost("https://api.example.com")
gorest.WithTimeout(30 * time.Second)
gorest.WithLogger(zapLogger)

// Rate Limiting
gorest.WithRateLimit(rate.Limit(100), 20) // 100 req/sec, burst 20

// Circuit Breaker
gorest.WithCircuitBreaker(gorest.CircuitBreakerConfig{
    MaxFailures:  5,                // Open circuit after 5 failures
    ResetTimeout: 60 * time.Second, // Try again after 60 seconds
})

// Retry Configuration
gorest.WithRetry(gorest.RetryConfig{
    MaxRetries: 3,
    BaseDelay:  100 * time.Millisecond,
    MaxDelay:   5 * time.Second,
})

// HTTP Transport
gorest.WithTransport(&http.Transport{
    MaxIdleConns:        200,
    MaxIdleConnsPerHost: 20,
    IdleConnTimeout:     90 * time.Second,
    TLSHandshakeTimeout: 10 * time.Second,
})

// Default Headers
gorest.WithDefaultHeaders(map[string]string{
    "User-Agent": "MyApp/1.0",
    "Accept":     "application/json",
})

// Endpoint-Specific Configuration
gorest.WithEndpointConfig("/api/v1/users/*", gorest.EndpointOptions{
    RateLimit: rate.NewLimiter(rate.Limit(50), 10),
    Timeout:   15 * time.Second,
    CircuitBreaker: &gorest.CircuitBreakerConfig{
        MaxFailures:  3,
        ResetTimeout: 30 * time.Second,
    },
    Retry: &gorest.RetryConfig{
        MaxRetries: 5,
        BaseDelay:  200 * time.Millisecond,
    },
})
```

## 🏗️ Architecture

### Thread Safety
Gorest uses a **copy-on-write pattern** to ensure thread safety:

- **Shared Infrastructure**: HTTP client, rate limiter, circuit breaker (thread-safe)
- **Request-Specific Data**: Headers, params, body (copied per request chain)
- **Zero Interference**: Multiple goroutines can use the same client safely

### Configuration Hierarchy
```
Endpoint Config → Service Config → Global Config → Default Config
```

The most specific configuration wins. For example:
1. Endpoint `/auth/login` rate limit: 5 req/sec
2. Service default rate limit: 20 req/sec  
3. Global default rate limit: 100 req/sec
4. Built-in default: No rate limit

→ `/auth/login` requests will be limited to 5 req/sec

### Pattern Matching
Endpoint configurations support wildcard patterns:

```go
"/users/*"        // Matches /users/123, /users/profile, etc.
"/admin/*"        // Matches /admin/users, /admin/settings, etc.
"/api/v1/orders" // Exact match only
```

## 📊 Monitoring & Observability

### Logging
```go
logger, _ := zap.NewProduction()

client := gorest.NewClient(
    gorest.WithLogger(logger),
    // Logger will automatically record:
    // - Request details (method, URL, headers)
    // - Response details (status, duration)
    // - Rate limit events
    // - Circuit breaker state changes
    // - Retry attempts
)
```

### Circuit Breaker States
- **Closed**: Normal operation, requests pass through
- **Open**: Circuit is open, requests fail fast
- **Half-Open**: Testing if service recovered

## 🧪 Testing

Gorest is designed to be testable:

```go
// Create isolated clients for testing
testClient := gorest.NewClient(
    gorest.WithHost("http://localhost:8080"), // Test server
    gorest.WithTimeout(5 * time.Second),       // Short timeout for tests
    // No rate limiting in tests
)

// Use in tests
func TestAPICall(t *testing.T) {
    var result TestResponse
    err := testClient.
        Body(testRequest).
        Response(&result).
        Post("/test/endpoint")
        
    assert.NoError(t, err)
    assert.Equal(t, expectedResult, result)
}
```

## 📈 Performance

### Connection Pooling
- Automatic HTTP connection reuse
- Configurable pool sizes
- Keep-alive connections
- Optimized for high-throughput scenarios

### Rate Limiting
- Token bucket algorithm
- Per-service and per-endpoint limits
- Burst capacity support
- Graceful backpressure

### Circuit Breaker
- Fast failure detection
- Automatic recovery testing
- Prevents cascade failures
- Configurable failure thresholds

## 🔄 Migration from v1.x

### Breaking Changes
- Global `NetworkClient` is now thread-safe but requires initialization for custom config
- New `Initialize()` method for global configuration
- New `NewClient()` method for service instances

### Migration Steps

1. **No changes needed** for basic usage:
   ```go
   // This still works exactly the same
   gorest.Client.Host("api.com").Get("/data")
   ```

2. **Add global initialization** (optional):
   ```go
   func main() {
       gorest.Initialize(
           gorest.WithTimeout(30 * time.Second),
           gorest.WithRateLimit(rate.Limit(100), 20),
       )
       
       // Rest of your code unchanged
   }
   ```

3. **Use service clients** for different services:
   ```go
   // Replace global client with service-specific clients
   paymentClient := gorest.NewClient(
       gorest.WithHost("payment-api.com"),
       gorest.WithRateLimit(rate.Limit(5), 2),
   )
   ```

## 🤝 Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details.

## 📄 License

This project is licensed under the [MIT License](LICENSE).

## 📞 Support

For questions and support:
- 📧 Email: [xander1235](https://github.com/xander1235)
- 🐛 Issues: [GitHub Issues](https://github.com/xander1235/gorest/issues)
- 📖 Documentation: [API Docs](https://pkg.go.dev/github.com/xander1235/gorest)

---

**Gorest v2.0** - Production-ready HTTP client for Go applications 🚀