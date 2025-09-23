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
- **DAG Workflows**: Execute complex workflows with dependency management
- **Multi-Endpoint Requests**: Send requests to multiple endpoints in parallel or sequence
- **Middleware System**: Interceptors for request/response processing
- **Server-Sent Events (SSE)**: Real-time streaming with automatic reconnection
- **Rate Limiting**: Token bucket algorithm with per-endpoint limits
- **Circuit Breaker**: Fault tolerance with automatic failure detection
- **Retry Mechanism**: Exponential backoff with configurable policies
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

## 🔄 DAG Workflows

### Overview
DAG (Directed Acyclic Graph) Workflows enable complex request orchestration with dependency management. Steps can execute in parallel when they have no dependencies, and dependent steps wait for their prerequisites to complete.

### Key Features
- **Dependency Management**: Define dependencies between steps
- **Parallel Execution**: Independent steps run concurrently
- **Data Sharing**: Pass data between steps using WorkflowContext
- **Conditional Execution**: Skip steps based on conditions
- **Error Handling**: Per-step error handling with optional workflow halt
- **Retry Support**: Built-in retry mechanism for failed steps

### Basic DAG Workflow Example
```go
package main

import (
    "github.com/xander1235/gorest"
    "github.com/xander1235/gorest/constants/enums"
)

type AuthResponse struct {
    Token  string `json:"token"
    UserID int    `json:"user_id"`
}

type UserData struct {
    ID    int    `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

type ProfileData struct {
    UserID  int    `json:"user_id"`
    Bio     string `json:"bio"`
    Avatar  string `json:"avatar"`
}

func main() {
    client := gorest.NewClient()
    
    var authResp AuthResponse
    var userData UserData
    var profileData ProfileData
    
    // Define workflow steps with dependencies
    steps := []*gorest.DAGWorkflowStep{
        {
            Name:     "authenticate",
            Endpoint: "/auth/login",
            Method:   enums.POST,
            BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
                return map[string]string{
                    "username": "admin",
                    "password": "secret",
                }
            },
            Response:    &authResp,
            StopOnError: true, // Critical step - halt workflow on failure
        },
        {
            Name:         "get_user",
            Dependencies: []string{"authenticate"}, // Depends on auth
            Endpoint:     "/users/me",
            Method:       enums.GET,
            HeadersBuilder: func(ctx *gorest.WorkflowContext) map[string]string {
                authResult := ctx.GetStepResult("authenticate")
                auth := authResult.ResponseData.(*AuthResponse)
                return map[string]string{
                    "Authorization": "Bearer " + auth.Token,
                }
            },
            Response: &userData,
        },
        {
            Name:         "get_profile",
            Dependencies: []string{"authenticate"}, // Also depends on auth
            Endpoint:     "/profiles/me",
            Method:       enums.GET,
            HeadersBuilder: func(ctx *gorest.WorkflowContext) map[string]string {
                authResult := ctx.GetStepResult("authenticate")
                auth := authResult.ResponseData.(*AuthResponse)
                return map[string]string{
                    "Authorization": "Bearer " + auth.Token,
                }
            },
            Response: &profileData,
        },
        {
            Name:         "update_last_login",
            Dependencies: []string{"get_user", "get_profile"}, // Waits for both
            Endpoint:     "/users/last-login",
            Method:       enums.PUT,
            BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
                userResult := ctx.GetStepResult("get_user")
                user := userResult.ResponseData.(*UserData)
                return map[string]interface{}{
                    "user_id": user.ID,
                    "timestamp": time.Now().Unix(),
                }
            },
        },
    }
    
    // Execute the DAG workflow
    response := client.Host("https://api.example.com").
        ExecuteDAGWorkflow(steps, nil, nil)
    
    if response.Success {
        fmt.Printf("Workflow completed: %d steps executed\n", response.CompletedSteps)
        fmt.Printf("User: %+v\n", userData)
        fmt.Printf("Profile: %+v\n", profileData)
    } else {
        fmt.Printf("Workflow failed: %d completed, %d failed\n", 
            response.CompletedSteps, len(response.FailedSteps))
    }
}
```

### Advanced DAG Features

#### Conditional Step Execution
```go
steps := []*gorest.DAGWorkflowStep{
    {
        Name:     "check_permission",
        Endpoint: "/permissions/check",
        Method:   enums.GET,
        Response: &permissionResp,
    },
    {
        Name:         "admin_action",
        Dependencies: []string{"check_permission"},
        Endpoint:     "/admin/action",
        Method:       enums.POST,
        Condition: func(ctx *gorest.WorkflowContext) bool {
            // Only execute if user has admin permission
            permResult := ctx.GetStepResult("check_permission")
            perm := permResult.ResponseData.(*PermissionResponse)
            return perm.IsAdmin
        },
        Response: &adminResp,
    },
}
```

#### Parallel Processing with Dependencies
```go
// This creates a diamond-shaped DAG:
//      step1
//     /     \
//   step2  step3
//     \     /
//      step4
steps := []*gorest.DAGWorkflowStep{
    {
        Name:     "step1",
        Endpoint: "/init",
        Method:   enums.POST,
    },
    {
        Name:         "step2",
        Dependencies: []string{"step1"},
        Endpoint:     "/process-a",
        Method:       enums.POST,
    },
    {
        Name:         "step3",
        Dependencies: []string{"step1"},
        Endpoint:     "/process-b",
        Method:       enums.POST,
    },
    {
        Name:         "step4",
        Dependencies: []string{"step2", "step3"},
        Endpoint:     "/finalize",
        Method:       enums.POST,
    },
}
```

#### With Options for Fine Control
```go
response := client.ExecuteDAGWorkflowWithOptions(
    steps,
    initialBody,
    metadata,
    10,   // maxConcurrency: limit parallel executions
    true, // stopOnFirstError: halt on any failure
)
```

For more advanced workflow examples, see the [Advanced Features Documentation](docs/ADVANCED_FEATURES.md).

## 🔧 Middleware System

### Overview
Gorest provides a powerful middleware system for intercepting and modifying HTTP requests and responses. Middlewares can transform data, add headers, handle errors, implement custom logging, and more.

### Basic Middleware Example
```go
// Custom authentication middleware
type AuthMiddleware struct {
    token string
}

func (m *AuthMiddleware) Execute(ctx *gorest.MiddlewareContext, next gorest.NextFunc) bool {
    // Add auth header to all requests
    ctx.Request.Header.Set("Authorization", "Bearer " + m.token)
    
    // Continue to next middleware
    return next()
}

// Usage
client := gorest.NewClient(
    gorest.WithMiddleware(&AuthMiddleware{token: "your-token"}),
)
```

### Response Processing Middleware
```go
type LoggingMiddleware struct {
    logger *zap.Logger
}

func (m *LoggingMiddleware) Execute(ctx *gorest.MiddlewareContext, next gorest.NextFunc) bool {
    // Log request
    m.logger.Info("Request", 
        zap.String("method", ctx.Method),
        zap.String("endpoint", ctx.Endpoint))
    
    // Execute request
    result := next()
    
    // Log response
    if ctx.Response != nil {
        m.logger.Info("Response",
            zap.Int("status", ctx.Response.StatusCode),
            zap.Duration("duration", ctx.Duration))
    }
    
    return result
}
```

## 🌊 Server-Sent Events (SSE)

### Overview
Gorest supports Server-Sent Events for real-time streaming with automatic reconnection, event filtering, and buffering.

### Basic SSE Streaming
```go
stream, err := client.
    WithSSEConfig(&types.SSEConfig{
        ReadTimeout:     30 * time.Second,
        EnableReconnect: true,
        BufferSize:      100,
    }).
    StreamGet("/events")

if err != nil {
    panic(err)
}
defer stream.Close()

// Read events
for event := range stream.Events {
    fmt.Printf("Event: %s, Data: %s\n", event.Event, event.Data)
}
```

### Filtered SSE with Reconnection
```go
stream, err := client.
    WithSSEConfig(&types.SSEConfig{
        EnableReconnect:       true,
        MaxReconnectAttempts: 5,
        ReconnectInterval:    5 * time.Second,
        EventFilter: func(event types.SSEEvent) bool {
            return event.Event == "update" // Only process updates
        },
        OnReconnect: func(attempt int) {
            fmt.Printf("Reconnecting... attempt %d\n", attempt)
        },
    }).
    StreamGet("/live-updates")
```

## 🔄 Multi-Endpoint Requests

### Overview
Send the same request to multiple endpoints with transformations, perfect for microservice architectures.

### Parallel Health Checks
```go
targets := []*gorest.MultiRequestTarget{
    {
        Endpoint: "/service1/health",
        Method:   enums.GET,
        Response: &service1Health,
    },
    {
        Endpoint: "/service2/health",
        Method:   enums.GET,
        Response: &service2Health,
    },
    {
        Endpoint: "/service3/health",
        Method:   enums.GET,
        Response: &service3Health,
    },
}

result := client.ExecuteMultiEndpoints(targets, true) // true = parallel

if result.Success {
    fmt.Printf("All services healthy!\n")
} else {
    for _, failed := range result.GetFailedResults() {
        fmt.Printf("Service %s is down\n", failed.Target.Endpoint)
    }
}
```

### Data Broadcasting with Transformations
```go
userData := map[string]interface{}{
    "name": "John",
    "email": "john@example.com",
}

targets := []*gorest.MultiRequestTarget{
    {
        Endpoint: "/user-service/users",
        Method:   enums.POST,
        Transform: func(body interface{}) interface{} {
            // Add service-specific field
            data := body.(map[string]interface{})
            data["source"] = "api-gateway"
            return data
        },
    },
    {
        Endpoint: "/analytics/events",
        Method:   enums.POST,
        Transform: func(body interface{}) interface{} {
            // Transform for analytics service
            return map[string]interface{}{
                "event_type": "user_created",
                "user_data":  body,
                "timestamp":  time.Now().Unix(),
            }
        },
    },
}

result := client.Body(userData).ExecuteMultiEndpoints(targets, false) // sequential
```

## 🚦 Rate Limiting

### Global Rate Limiting
```go
client := gorest.NewClient(
    gorest.WithRateLimit(
        rate.Limit(100), // 100 requests per second
        20,              // burst size of 20
    ),
)
```

### Per-Endpoint Rate Limiting
```go
client := gorest.NewClient(
    gorest.WithEndpointConfig("/api/expensive/*", gorest.EndpointConfig{
        RateLimit: &gorest.RateLimitConfig{
            Limit: rate.Limit(5), // 5 req/sec for expensive endpoints
            Burst: 2,
        },
    }),
    gorest.WithEndpointConfig("/api/cheap/*", gorest.EndpointConfig{
        RateLimit: &gorest.RateLimitConfig{
            Limit: rate.Limit(1000), // 1000 req/sec for cheap endpoints
            Burst: 50,
        },
    }),
)
```

## 🔌 Circuit Breaker

### Configuration
```go
client := gorest.NewClient(
    gorest.WithCircuitBreaker(gorest.CircuitBreakerConfig{
        MaxFailures:         5,                // Open after 5 failures
        SuccessiveFailures:  3,                // Or 3 successive failures
        ResetTimeout:        60 * time.Second, // Try recovery after 60s
        HalfOpenRequests:    3,                // Allow 3 test requests
        FailureRatioThreshold: 0.5,            // Open if 50% requests fail
        SampleSize:          100,              // Sample size for ratio
    }),
)
```

### Per-Endpoint Circuit Breaker
```go
client := gorest.NewClient(
    gorest.WithEndpointConfig("/external-api/*", gorest.EndpointConfig{
        CircuitBreaker: &gorest.CircuitBreakerConfig{
            MaxFailures:  3,                // More sensitive for external
            ResetTimeout: 30 * time.Second,
        },
    }),
)
```

## 🔁 Retry Mechanism

### Global Retry Configuration
```go
client := gorest.NewClient(
    gorest.WithRetry(gorest.RetryConfig{
        MaxRetries:      3,
        BaseDelay:       100 * time.Millisecond,
        MaxDelay:        5 * time.Second,
        Multiplier:      2.0,              // Exponential backoff
        RetryableErrors: []int{502, 503}, // Retry on specific status codes
    }),
)
```

### Per-Endpoint Retry
```go
client := gorest.NewClient(
    gorest.WithEndpointConfig("/flaky-service/*", gorest.EndpointConfig{
        Retry: &gorest.RetryConfig{
            MaxRetries: 5,              // More retries for flaky service
            BaseDelay:  200 * time.Millisecond,
            MaxDelay:   10 * time.Second,
        },
    }),
)
```

### Custom Retry Logic
```go
client := gorest.NewClient(
    gorest.WithRetry(gorest.RetryConfig{
        RetryIf: func(resp *http.Response, err error) bool {
            // Custom retry logic
            if err != nil {
                return true // Retry on any error
            }
            if resp.StatusCode == 429 {
                // Check rate limit headers
                retryAfter := resp.Header.Get("Retry-After")
                return retryAfter != ""
            }
            return resp.StatusCode >= 500
        },
    }),
)
```

For more advanced examples, see the [Advanced Features Documentation](docs/ADVANCED_FEATURES.md).

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
## 𝌙 Advanced Features

Advanced features are available in the [ADVANCED_FEATURES.md](docs/ADVANCED_FEATURES.md) file.

## 📜 Changelog

All notable changes to this project are documented in the [CHANGELOG.md](CHANGELOG.md) file.

## 🤝 Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details.

## 📄 License

This project is licensed under the [MIT License](LICENSE).

## 📞 Support

For questions and support:
- 📧 Email: [rathodveerender25@gmail.com](rathodveerender25@gmail.com)
- 🐛 Issues: [GitHub Issues](https://github.com/xander1235/gorest/issues)
- 📖 Documentation: [API Docs](https://pkg.go.dev/github.com/xander1235/gorest)

## 🛡️ Security

For security vulnerabilities, please refer to our [Security Policy](SECURITY.md).

---

**Gorest v2.0** - Production-ready HTTP client for Go applications 🚀