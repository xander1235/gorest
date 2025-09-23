# Advanced Features: Multi-Endpoint Requests and DAG Workflows

GoRest v2.0 introduces powerful features that enable sophisticated HTTP request orchestration:

1. **Multi-Endpoint Requests** - Send requests to multiple endpoints with parallel or sequential execution
2. **DAG Workflow System** - Execute complex workflows with dependency management and automatic parallelization

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
