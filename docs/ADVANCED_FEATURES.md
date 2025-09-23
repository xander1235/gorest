# Advanced Features Documentation

GoRest v2.0 provides powerful features for sophisticated HTTP request orchestration and management:

1. **Multi-Endpoint Requests** - Send requests to multiple endpoints with parallel or sequential execution
2. **DAG Workflow System** - Execute complex workflows with dependency management and automatic parallelization
3. **Middleware System** - Intercept and modify requests/responses with custom logic
4. **Server-Sent Events (SSE)** - Real-time streaming with automatic reconnection
5. **Rate Limiting** - Control request rates with token bucket algorithm
6. **Circuit Breaker** - Fault tolerance with automatic failure detection
7. **Retry Mechanism** - Configurable retry policies with exponential backoff

## Multi-Endpoint Requests

### Overview

Multi-endpoint requests allow you to send the same request body (with optional transformations) to multiple endpoints either in parallel for speed or sequentially for order guarantees.

### Use Cases

- **Health Checks**: Monitor multiple services simultaneously
- **Data Broadcasting**: Send data to multiple services with service-specific transformations
- **Fan-out Operations**: Distribute the same operation across multiple endpoints
- **A/B Testing**: Send identical requests to different service versions

### Basic Usage

```go
package main

import (
    "github.com/xander1235/gorest"
    "github.com/xander1235/gorest/constants/enums"
)

func main() {
    client := gorest.NewClient()

    // Define target endpoints
    targets := []*gorest.MultiRequestTarget{
        {
            Endpoint: "/api/health",
            Method:   enums.GET,
        },
        {
            Endpoint: "/db/health", 
            Method:   enums.GET,
        },
        {
            Endpoint: "/cache/health",
            Method:   enums.GET,
        },
    }

    // Execute in parallel for faster results
    result := client.Host("https://api.example.com").ExecuteMultiEndpoints(targets, true)

    if result.Success {
        fmt.Printf("All %d services are healthy!\n", result.SuccessCount)
    } else {
        fmt.Printf("Issues found: %d failed\n", result.FailureCount)
    }
}
```

### Advanced Features

#### 1. Request Body Transformations

Transform the base request body for each specific endpoint:

```go
userData := map[string]interface{}{
    "name":  "John Doe",
    "email": "john@example.com",
}

targets := []*gorest.MultiRequestTarget{
    {
        Endpoint: "/users",
        Method:   enums.POST,
        Transform: func(body interface{}) interface{} {
            data := body.(map[string]interface{})
            // Add service-specific field
            data["type"] = "user"
            data["source"] = "api"
            return data
        },
    },
    {
        Endpoint: "/profiles",
        Method:   enums.POST,
        Transform: func(body interface{}) interface{} {
            originalData := body.(map[string]interface{})
            // Transform for profile service
            return map[string]interface{}{
                "full_name":    originalData["name"],
                "email_addr":   originalData["email"],
                "profile_type": "standard",
            }
        },
    },
}

result := client.Body(userData).ExecuteMultiEndpoints(targets, false)
```

#### 2. Per-Endpoint Headers and Parameters

Customize headers and query parameters for each endpoint:

```go
targets := []*gorest.MultiRequestTarget{
    {
        Endpoint: "/service1/data",
        Method:   enums.POST,
        Headers: map[string]string{
            "X-Service-Version": "v1",
            "X-Priority":        "high",
        },
        Params: map[string]string{
            "format": "json",
            "sync":   "true",
        },
    },
    {
        Endpoint: "/service2/data",
        Method:   enums.POST,
        Headers: map[string]string{
            "X-Service-Version": "v2",
            "X-Priority":        "normal",
        },
    },
}
```

#### 3. Response Capture

Capture responses from each endpoint in separate variables:

```go
var service1Response Service1Response
var service2Response Service2Response

targets := []*gorest.MultiRequestTarget{
    {
        Endpoint: "/service1/status",
        Method:   enums.GET,
        Response: &service1Response,
    },
    {
        Endpoint: "/service2/status",
        Method:   enums.GET,
        Response: &service2Response,
    },
}

result := client.Host("https://api.example.com").ExecuteMultiEndpoints(targets, true)

// Access individual responses
fmt.Printf("Service 1: %+v\n", service1Response)
fmt.Printf("Service 2: %+v\n", service2Response)
```

#### 4. Error Handling and Analysis

```go
result := client.ExecuteMultiEndpoints(targets, true)

// Check overall success
if !result.Success {
    fmt.Printf("Some requests failed: %d succeeded, %d failed\n", 
        result.SuccessCount, result.FailureCount)
    
    // Analyze failures
    failedResults := result.GetFailedResults()
    for _, failed := range failedResults {
        fmt.Printf("Endpoint %s failed: %s\n", 
            failed.Target.Endpoint, failed.Error.Message)
    }
    
    // Get error summary
    errorSummary := result.GetErrorSummary()
    for endpoint, err := range errorSummary {
        fmt.Printf("%s: HTTP %d - %s\n", endpoint, err.ResponseCode, err.Message)
    }
}
```

## DAG Workflow System

### Overview

The DAG (Directed Acyclic Graph) workflow system enables complex request orchestration with automatic dependency management. Unlike linear workflows, DAG workflows can execute independent steps in parallel while ensuring dependent steps wait for their prerequisites, maximizing efficiency and reducing overall execution time.

### Key Advantages Over Sequential Workflows

- **Automatic Parallelization**: Independent steps run concurrently
- **Dependency Management**: Define complex dependencies between steps
- **Optimal Execution**: The system automatically determines the most efficient execution order
- **Better Performance**: Reduced overall execution time through parallelization
- **Flexible Architecture**: Support for complex workflow patterns (diamond, fork-join, etc.)

### Use Cases

- **Microservice Orchestration**: Coordinate multiple service calls with dependencies
- **Data Aggregation**: Fetch data from multiple sources in parallel, then combine
- **Complex Authentication**: Parallel token refresh and permission checks
- **Resource Provisioning**: Create dependent resources with optimal ordering
- **Pipeline Processing**: ETL operations with parallel transformation steps

### Basic DAG Usage

```go
package main

import (
    "github.com/xander1235/gorest"
    "github.com/xander1235/gorest/constants/enums"
    "time"
)

type AuthResponse struct {
    Token  string `json:"token"`
    UserID int    `json:"user_id"`
}

type UserProfile struct {
    ID    int    `json:"id"`
    Name  string `json:"name"`
    Email string `json:"email"`
}

type Permissions struct {
    Roles []string `json:"roles"`
    Scope string   `json:"scope"`
}

func main() {
    client := gorest.NewClient()

    var authResp AuthResponse
    var userProfile UserProfile
    var permissions Permissions

    // Define a DAG workflow where profile and permissions
    // are fetched in parallel after authentication
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
            StopOnError: true, // Critical step
        },
        {
            Name:         "get_profile",
            Dependencies: []string{"authenticate"}, // Depends on auth
            Endpoint:     "/users/profile",
            Method:       enums.GET,
            HeadersBuilder: func(ctx *gorest.WorkflowContext) map[string]string {
                authResult := ctx.GetStepResult("authenticate")
                auth := authResult.ResponseData.(*AuthResponse)
                return map[string]string{
                    "Authorization": "Bearer " + auth.Token,
                }
            },
            Response: &userProfile,
        },
        {
            Name:         "get_permissions",
            Dependencies: []string{"authenticate"}, // Also depends on auth
            Endpoint:     "/users/permissions",
            Method:       enums.GET,
            HeadersBuilder: func(ctx *gorest.WorkflowContext) map[string]string {
                authResult := ctx.GetStepResult("authenticate")
                auth := authResult.ResponseData.(*AuthResponse)
                return map[string]string{
                    "Authorization": "Bearer " + auth.Token,
                }
            },
            Response: &permissions,
        },
        {
            Name:         "initialize_session",
            Dependencies: []string{"get_profile", "get_permissions"}, // Waits for both
            Endpoint:     "/session/init",
            Method:       enums.POST,
            BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
                profileResult := ctx.GetStepResult("get_profile")
                permResult := ctx.GetStepResult("get_permissions")
                
                profile := profileResult.ResponseData.(*UserProfile)
                perms := permResult.ResponseData.(*Permissions)
                
                return map[string]interface{}{
                    "user_id":     profile.ID,
                    "user_name":   profile.Name,
                    "roles":       perms.Roles,
                    "scope":       perms.Scope,
                    "initialized": time.Now().Unix(),
                }
            },
        },
    }

    // Execute the DAG workflow
    result := client.Host("https://api.example.com").
        ExecuteDAGWorkflow(steps, nil, nil)

    if result.Success {
        fmt.Printf("Session initialized successfully\n")
        fmt.Printf("User: %+v\n", userProfile)
        fmt.Printf("Permissions: %+v\n", permissions)
    } else {
        fmt.Printf("Workflow failed: %d steps completed, %d failed\n",
            result.CompletedSteps, len(result.FailedSteps))
    }
}
```

### Advanced Workflow Features

#### 1. Conditional Step Execution

Execute steps based on previous results or conditions:

```go
steps := []*gorest.WorkflowStep{
    {
        Name:     "check_user_tier",
        Endpoint: "/users/123/tier",
        Method:   enums.GET,
        Response: &tierCheck,
    },
    {
        Name:     "premium_operation",
        Endpoint: "/premium/feature",
        Method:   enums.POST,
        Condition: func(ctx *gorest.WorkflowContext) bool {
            // Only execute if user is premium
            tierResult := ctx.GetStepResult("check_user_tier")
            if tierResult != nil && tierResult.Success {
                tierData := tierResult.ResponseData.(*map[string]interface{})
                if tier, exists := (*tierData)["tier"]; exists {
                    return tier == "premium"
                }
            }
            return false
        },
        BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
            return map[string]interface{}{
                "feature": "advanced_analytics",
            }
        },
    },
}
```

#### 2. Error Handling and Recovery

Control workflow behavior when steps fail:

```go
steps := []*gorest.WorkflowStep{
    {
        Name:        "critical_operation",
        Endpoint:    "/critical/task",
        Method:      enums.POST,
        StopOnError: true, // Halt workflow if this fails
    },
    {
        Name:        "optional_notification",
        Endpoint:    "/notifications",
        Method:      enums.POST,
        StopOnError: false, // Continue even if this fails
    },
    {
        Name:     "cleanup",
        Endpoint: "/cleanup",
        Method:   enums.POST,
        BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
            // Always runs, can check which steps failed
            failedSteps := []string{}
            for name, result := range ctx.StepResults {
                if !result.Success && !result.Skipped {
                    failedSteps = append(failedSteps, name)
                }
            }
            return map[string]interface{}{
                "failed_steps": failedSteps,
            }
        },
        StopOnError: false,
    },
}
```

#### 3. Metadata and Context Sharing

Share data between workflow steps using metadata:

```go
// Initial metadata
metadata := map[string]interface{}{
    "workflow_id": "user-onboarding",
    "started_at":  time.Now().Unix(),
}

steps := []*gorest.WorkflowStep{
    {
        Name: "step1",
        BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
            // Set metadata for later steps
            ctx.SetMetadata("step1_completed", true)
            ctx.SetMetadata("processing_time", time.Now().Unix())
            
            return map[string]interface{}{
                "workflow_id": ctx.GetMetadata("workflow_id"),
            }
        },
    },
    {
        Name: "step2",
        BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
            // Use metadata from previous step
            step1Completed := ctx.GetMetadata("step1_completed")
            processingTime := ctx.GetMetadata("processing_time")
            
            return map[string]interface{}{
                "previous_step_completed": step1Completed,
                "processing_time":         processingTime,
            }
        },
    },
}

result := client.ExecuteWorkflow(steps, nil, metadata)
```

#### 4. Per-Step Context and Timeouts

Set individual timeouts and contexts for steps:

```go
// Create context with timeout for specific step
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

steps := []*gorest.WorkflowStep{
    {
        Name:     "fast_operation",
        Endpoint: "/fast",
        Method:   enums.GET,
    },
    {
        Name:     "slow_operation",
        Endpoint: "/slow",
        Method:   enums.GET,
        Context:  ctx, // This step has a 30-second timeout
    },
}
```

### Workflow Analysis and Debugging

#### 1. Comprehensive Result Analysis

```go
result := client.ExecuteWorkflow(steps, initialData, metadata)

// Overall status
fmt.Printf("Workflow: %s\n", result.String())
fmt.Printf("Success: %t\n", result.Success)
fmt.Printf("Completed: %d, Skipped: %d\n", result.CompletedSteps, result.SkippedSteps)

// Analyze individual steps
for _, stepResult := range result.StepResults {
    status := "SUCCESS"
    if stepResult.Skipped {
        status = "SKIPPED"
    } else if !stepResult.Success {
        status = "FAILED"
    }
    fmt.Printf("Step %s: %s\n", stepResult.Step.Name, status)
}

// Get specific step results
authStep := result.GetStepByName("authenticate")
if authStep != nil && authStep.Success {
    fmt.Printf("Authentication successful\n")
}

// Error analysis
if !result.Success {
    errorSummary := result.GetErrorSummary()
    for stepName, err := range errorSummary {
        fmt.Printf("Step %s failed: %s (HTTP %d)\n", 
            stepName, err.Message, err.ResponseCode)
    }
}
```

#### 2. Helper Methods

```go
// Get successful/failed/skipped steps
successfulSteps := result.GetSuccessfulSteps()
failedSteps := result.GetFailedSteps()
skippedSteps := result.GetSkippedSteps()

// Context helpers
ctx := result.Context
hasAuth := ctx.HasSuccessfulStep("authenticate")
authResponse := ctx.GetStepResponse("authenticate")
metadata := ctx.GetMetadata("workflow_id")
```

### DAG Workflow Architecture

The DAG workflow system is built on a powerful execution engine that:

1. **Analyzes Dependencies**: Automatically determines which steps can run in parallel
2. **Optimizes Execution**: Runs independent steps concurrently to minimize total execution time
3. **Manages State**: Tracks step completion and handles data flow between dependent steps
4. **Handles Failures**: Provides granular error handling with optional workflow termination

The DAG engine organizes steps into execution layers based on their dependencies. Steps in the same layer have no dependencies on each other and can execute in parallel, while steps in subsequent layers wait for their dependencies to complete.

## Performance Considerations

### Multi-Endpoint Requests

- **Parallel Execution**: Use for independent requests to maximize throughput
- **Sequential Execution**: Use when order matters or to avoid overwhelming services
- **Connection Pooling**: The client automatically reuses connections across requests
- **Error Isolation**: Failed requests don't affect successful ones

### DAG Workflows

- **Parallel Execution**: Independent steps run concurrently for optimal performance
- **Memory Management**: Response data is efficiently shared through WorkflowContext
- **Dependency Resolution**: Automatic topological sorting ensures correct execution order
- **Context Isolation**: Each step maintains its own request configuration
- **Concurrency Control**: Configurable max concurrent executions to prevent overload

## Best Practices

### Multi-Endpoint Requests

1. **Choose Execution Mode Wisely**
   ```go
   // Use parallel for independent operations
   healthResult := client.ExecuteMultiEndpoints(healthTargets, true)
   
   // Use sequential for ordered operations
   dataFlowResult := client.ExecuteMultiEndpoints(dataTargets, false)
   ```

2. **Handle Partial Failures Gracefully**
   ```go
   if result.HasErrors {
       // Process successful results
       for _, success := range result.GetSuccessfulResults() {
           // Handle successful response
       }
       
       // Handle failures
       for _, failure := range result.GetFailedResults() {
           log.Printf("Failed: %s - %s", failure.Target.Endpoint, failure.Error.Message)
       }
   }
   ```

3. **Use Transformations for Service-Specific Data**
   ```go
   Transform: func(body interface{}) interface{} {
       data := body.(map[string]interface{})
       // Add service-specific fields
       return enhanceForService(data, "user-service")
   }
   ```

### DAG Workflows

1. **Optimize Dependencies for Parallelism**
   ```go
   // Good: Allows parallel execution
   steps := []*gorest.DAGWorkflowStep{
       {Name: "auth"},
       {Name: "fetch_user", Dependencies: []string{"auth"}},
       {Name: "fetch_settings", Dependencies: []string{"auth"}},
       {Name: "combine", Dependencies: []string{"fetch_user", "fetch_settings"}},
   }
   
   // Less optimal: Forces sequential execution
   steps := []*gorest.DAGWorkflowStep{
       {Name: "step1"},
       {Name: "step2", Dependencies: []string{"step1"}},
       {Name: "step3", Dependencies: []string{"step2"}},
       {Name: "step4", Dependencies: []string{"step3"}},
   }
   ```

2. **Use Meaningful Step Names**
   ```go
   steps := []*gorest.DAGWorkflowStep{
       {Name: "authenticate_admin"},
       {Name: "validate_permissions", Dependencies: []string{"authenticate_admin"}},
       {Name: "create_resource"},
       {Name: "send_notification"},
       {Name: "audit_log"},
   }
   ```

3. **Leverage Context for Data Sharing**
   ```go
   BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
       authResult := ctx.GetStepResult("authenticate")
       if authResult.Success {
           auth := authResult.ResponseData.(*AuthResponse)
           return buildRequestWithAuth(auth.Token)
       }
       return nil
   }
   ```

4. **Use Metadata for Workflow Tracking**
   ```go
   metadata := map[string]interface{}{
       "workflow_id":   generateWorkflowID(),
       "user_id":       userID,
       "started_at":    time.Now().Unix(),
       "correlation_id": correlationID,
   }
   ```

## Integration with Existing Features

Both multi-endpoint requests and workflows integrate seamlessly with existing GoRest features:

- **Rate Limiting**: Applied per endpoint configuration
- **Circuit Breakers**: Individual endpoint circuit breakers are respected
- **Retries**: Automatic retry logic applies to each request
- **Middleware**: Request/response middleware executes for each request
- **Logging**: Comprehensive logging for debugging and monitoring
- **SSE Streaming**: Can be used within workflow steps
- **Authentication**: Headers and authentication apply to all requests

## Middleware System

### Overview

The middleware system provides a powerful way to intercept, modify, and enhance HTTP requests and responses. Middlewares execute in a chain, allowing multiple processing steps.

### Use Cases

- **Authentication**: Add auth tokens to all requests
- **Logging**: Track all API calls and responses
- **Metrics**: Collect performance and usage statistics
- **Error Handling**: Transform or retry on specific errors
- **Request/Response Transformation**: Modify data in flight
- **Caching**: Implement request caching logic

### Creating Custom Middleware

```go
type RateLimitMiddleware struct {
    limiter *rate.Limiter
}

func (m *RateLimitMiddleware) Execute(ctx *gorest.MiddlewareContext, next gorest.NextFunc) bool {
    // Check rate limit before request
    if !m.limiter.Allow() {
        ctx.Error = &errors.ErrorDetails{
            Message: "Rate limit exceeded",
            Code:    "RATE_LIMITED",
            ResponseCode: 429,
        }
        return false // Stop chain execution
    }
    
    return next() // Continue to next middleware
}
```

### Middleware Chain Example

```go
client := gorest.NewClient(
    gorest.WithMiddleware(
        &AuthMiddleware{token: "secret"},
        &LoggingMiddleware{logger: logger},
        &RetryMiddleware{maxAttempts: 3},
        &MetricsMiddleware{collector: metrics},
    ),
)
```

### Request Modification Middleware

```go
type RequestEnrichmentMiddleware struct{}

func (m *RequestEnrichmentMiddleware) Execute(ctx *gorest.MiddlewareContext, next gorest.NextFunc) bool {
    // Add custom headers
    ctx.Request.Header.Set("X-Request-ID", uuid.New().String())
    ctx.Request.Header.Set("X-Client-Version", "2.0")
    
    // Store metadata for other middlewares
    ctx.Metadata["request_time"] = time.Now()
    
    // Continue chain
    result := next()
    
    // Process after response
    if ctx.Response != nil {
        requestTime := ctx.Metadata["request_time"].(time.Time)
        fmt.Printf("Request took: %v\n", time.Since(requestTime))
    }
    
    return result
}
```

### Error Handling Middleware

```go
type ErrorTransformMiddleware struct{}

func (m *ErrorTransformMiddleware) Execute(ctx *gorest.MiddlewareContext, next gorest.NextFunc) bool {
    result := next()
    
    if ctx.Error != nil {
        // Transform specific errors
        if ctx.Error.ResponseCode == 401 {
            // Trigger token refresh
            ctx.Metadata["needs_refresh"] = true
        }
        
        // Log errors
        log.Printf("Error on %s %s: %s", 
            ctx.Method, ctx.Endpoint, ctx.Error.Message)
    }
    
    return result
}
```

## Server-Sent Events (SSE)

### Overview

SSE support enables real-time streaming of server events with automatic reconnection, event filtering, and buffering capabilities.

### Use Cases

- **Live Updates**: Real-time dashboard updates
- **Notifications**: Push notifications to clients
- **Progress Tracking**: Long-running operation status
- **Log Streaming**: Real-time log monitoring
- **Market Data**: Stock prices, crypto rates

### Basic SSE Streaming

```go
func streamEvents(client *gorest.NetworkClient) {
    stream, err := client.
        WithSSEConfig(&types.SSEConfig{
            ReadTimeout:     30 * time.Second,
            BufferSize:      100,
            EnableReconnect: true,
        }).
        StreamGet("/api/events")
    
    if err != nil {
        log.Fatal(err)
    }
    defer stream.Close()
    
    // Process events
    for event := range stream.Events {
        switch event.Event {
        case "update":
            fmt.Printf("Update: %s\n", event.Data)
        case "delete":
            fmt.Printf("Deleted: %s\n", event.ID)
        case "error":
            fmt.Printf("Error: %s\n", event.Data)
        }
    }
}
```

### Advanced SSE with Filtering and Reconnection

```go
config := &types.SSEConfig{
    ReadTimeout:           30 * time.Second,
    ReconnectInterval:     5 * time.Second,
    MaxReconnectAttempts: 10,
    BufferSize:           200,
    EnableReconnect:      true,
    
    // Filter events before processing
    EventFilter: func(event types.SSEEvent) bool {
        // Only process high-priority events
        return event.Event == "critical" || event.Event == "warning"
    },
    
    // Handle reconnection
    OnReconnect: func(attempt int) {
        fmt.Printf("Reconnecting (attempt %d)...\n", attempt)
    },
    
    // Resume from last event
    LastEventID: "event-12345",
}

stream, err := client.WithSSEConfig(config).StreamPost("/api/subscribe", subscription)
```

### SSE with Context and Cancellation

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
defer cancel()

stream, err := client.
    Context(ctx).
    WithSSEConfig(&types.SSEConfig{
        EnableReconnect: false, // Don't reconnect on context cancellation
    }).
    StreamGet("/api/live-feed")

if err != nil {
    log.Fatal(err)
}

// Process with timeout
for {
    select {
    case event, ok := <-stream.Events:
        if !ok {
            return // Stream closed
        }
        processEvent(event)
    case <-ctx.Done():
        fmt.Println("Streaming timeout")
        return
    }
}
```

## Rate Limiting Details

### Token Bucket Algorithm

Gorest uses a token bucket algorithm for rate limiting, providing:
- Smooth rate limiting with burst capacity
- Per-client and per-endpoint limits
- Automatic request queuing

### Advanced Configuration

```go
// Global rate limit with custom wait behavior
client := gorest.NewClient(
    gorest.WithRateLimit(rate.Limit(100), 20),
    gorest.WithRateLimitWaitTimeout(5 * time.Second), // Max wait time
)

// Different limits for different operations
client := gorest.NewClient(
    gorest.WithEndpointConfig("/api/read/*", gorest.EndpointConfig{
        RateLimit: &gorest.RateLimitConfig{
            Limit: rate.Limit(1000), // High limit for reads
            Burst: 100,
        },
    }),
    gorest.WithEndpointConfig("/api/write/*", gorest.EndpointConfig{
        RateLimit: &gorest.RateLimitConfig{
            Limit: rate.Limit(10), // Low limit for writes
            Burst: 5,
        },
    }),
)
```

## Circuit Breaker Patterns

### States and Transitions

1. **Closed**: Normal operation, requests pass through
2. **Open**: Circuit open, requests fail immediately
3. **Half-Open**: Testing recovery with limited requests

### Advanced Circuit Breaker

```go
config := gorest.CircuitBreakerConfig{
    // Failure conditions
    MaxFailures:           10,
    SuccessiveFailures:    3,
    FailureRatioThreshold: 0.5,
    SampleSize:           100,
    
    // Recovery settings
    ResetTimeout:     60 * time.Second,
    HalfOpenRequests: 5,
    
    // Custom failure detection
    IsFailure: func(err error, statusCode int) bool {
        // Custom logic to determine failure
        if err != nil {
            return true
        }
        return statusCode >= 500 || statusCode == 429
    },
    
    // State change callbacks
    OnStateChange: func(from, to string) {
        log.Printf("Circuit breaker: %s -> %s\n", from, to)
    },
}

client := gorest.NewClient(
    gorest.WithCircuitBreaker(config),
)
```

## Retry Strategies

### Exponential Backoff with Jitter

```go
retryConfig := gorest.RetryConfig{
    MaxRetries:      5,
    BaseDelay:       100 * time.Millisecond,
    MaxDelay:        30 * time.Second,
    Multiplier:      2.0,
    Jitter:          0.1, // 10% jitter
    RetryableErrors: []int{502, 503, 504},
    
    // Custom retry decision
    RetryIf: func(resp *http.Response, err error) bool {
        if err != nil {
            // Retry on network errors
            return true
        }
        
        // Check Retry-After header
        if resp.StatusCode == 429 {
            retryAfter := resp.Header.Get("Retry-After")
            if retryAfter != "" {
                // Parse and wait if reasonable
                return true
            }
        }
        
        return resp.StatusCode >= 500
    },
    
    // Pre-retry hook
    OnRetry: func(attempt int, delay time.Duration, err error) {
        log.Printf("Retry attempt %d after %v due to: %v\n", 
            attempt, delay, err)
    },
}
```

## Examples Repository

Complete working examples are available in the `/examples` directory:

- `examples/advanced_features.go` - Comprehensive examples of both features
- `examples/health_monitoring.go` - Multi-endpoint health checking
- `examples/user_onboarding.go` - Complete user onboarding workflow
- `examples/e_commerce_workflow.go` - Complex e-commerce order processing

## API Reference

### MultiRequestTarget

```go
type MultiRequestTarget struct {
    Endpoint  string                                    // URL path
    Method    enums.HttpMethods                        // HTTP method
    Transform func(baseBody interface{}) interface{}   // Body transformation
    Headers   map[string]string                        // Endpoint-specific headers
    Params    map[string]string                        // Query parameters
    Response  interface{}                              // Response capture
    Context   context.Context                          // Request context
}
```

### WorkflowStep

```go
type WorkflowStep struct {
    Name           string                                        // Unique step identifier
    Endpoint       string                                        // URL path
    Method         enums.HttpMethods                            // HTTP method
    BodyBuilder    func(ctx *WorkflowContext) interface{}       // Request body builder
    HeadersBuilder func(ctx *WorkflowContext) map[string]string // Headers builder
    ParamsBuilder  func(ctx *WorkflowContext) map[string]string // Parameters builder
    Response       interface{}                                   // Response capture
    Context        context.Context                               // Request context
    StopOnError    bool                                         // Error handling behavior
    Condition      func(ctx *WorkflowContext) bool              // Execution condition
}
```

## Migration Guide

These features are additive and don't break existing code. To start using them:

1. **Update GoRest**: Ensure you're using v2.0 or later
2. **Import Types**: The new types are in the main gorest package
3. **Update Code**: Start with simple examples and gradually adopt advanced features
4. **Test Thoroughly**: Both features involve multiple requests, so test error scenarios

## Support and Contributing

- **Issues**: Report bugs and request features on GitHub
- **Documentation**: Contribute examples and documentation improvements
- **Testing**: Help improve test coverage for edge cases
- **Performance**: Share performance benchmarks and optimization suggestions

These advanced features make GoRest a powerful tool for complex HTTP orchestration while maintaining the simplicity and reliability you expect from the library.
