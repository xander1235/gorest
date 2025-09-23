package main

import (
	"fmt"
	"github.com/xander1235/gorest/v2"
	"github.com/xander1235/gorest/v2/constants/enums"
	"time"
)

// Example models
type HealthCheckResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Uptime  int    `json:"uptime"`
}

type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token     string `json:"token"`
	UserID    int    `json:"user_id"`
	ExpiresIn int    `json:"expires_in"`
}

type UserRequest struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	CreatedBy int    `json:"created_by,omitempty"`
}

type UserResponse struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type NotificationRequest struct {
	UserID   int                    `json:"user_id"`
	Type     string                 `json:"type"`
	Template string                 `json:"template"`
	Data     map[string]interface{} `json:"data"`
}

func main() {
	// Initialize client
	client := gorest.NewClient()

	// Example 1: Multi-endpoint health checks
	fmt.Println("=== Example 1: Multi-endpoint Health Checks ===")
	healthCheckExample(client)

	// Example 2: Multi-endpoint data broadcast
	fmt.Println("\n=== Example 2: Multi-endpoint Data Broadcast ===")
	dataBroadcastExample(client)

	// Example 3: User onboarding using DAG workflow
	fmt.Println("\n=== Example 3: User Onboarding DAG Workflow ===")
	userOnboardingDAGWorkflow(client)

	// Example 4: Advanced DAG workflow with conditionals
	fmt.Println("\n=== Example 4: Advanced Conditional DAG Workflow ===")
	conditionalDAGWorkflowExample(client)

	// Example 5: Error handling in DAG workflows
	fmt.Println("\n=== Example 5: DAG Workflow Error Handling ===")
	dagWorkflowErrorHandlingExample(client)
}

// Example 1: Health check multiple services in parallel
func healthCheckExample(client *gorest.NetworkClient) {
	// Define services to check
	var apiHealth, dbHealth, cacheHealth HealthCheckResponse

	targets := []*gorest.MultiRequestTarget{
		{
			Endpoint: "/api/health",
			Method:   enums.GET,
			Response: &apiHealth,
		},
		{
			Endpoint: "/db/health",
			Method:   enums.GET,
			Response: &dbHealth,
		},
		{
			Endpoint: "/cache/health",
			Method:   enums.GET,
			Response: &cacheHealth,
		},
	}

	// Execute health checks in parallel for faster results
	result := client.Host("https://monitoring.example.com").ExecuteMultiEndpoints(targets, true)

	// Process results
	if result.Success {
		fmt.Printf("All %d services are healthy!\n", result.SuccessCount)
	} else {
		fmt.Printf("Health check results: %d succeeded, %d failed\n",
			result.SuccessCount, result.FailureCount)

		// Handle failed services
		for endpoint, err := range result.GetErrorSummary() {
			fmt.Printf("Service %s failed: %s\n", endpoint, err.Message)
		}
	}

	fmt.Printf("API Health: %+v\n", apiHealth)
	fmt.Printf("DB Health: %+v\n", dbHealth)
	fmt.Printf("Cache Health: %+v\n", cacheHealth)
}

// Example 2: Broadcast data to multiple endpoints with transformations
func dataBroadcastExample(client *gorest.NetworkClient) {
	userData := map[string]interface{}{
		"name":     "John Doe",
		"email":    "john@example.com",
		"source":   "api",
		"priority": "normal",
	}

	targets := []*gorest.MultiRequestTarget{
		{
			Endpoint: "/users",
			Method:   enums.POST,
			Transform: func(body interface{}) interface{} {
				// Add service-specific fields for user service
				data := copyInterfaceMap(body.(map[string]interface{}))
				data["type"] = "user"
				data["validated"] = true
				return data
			},
			Headers: map[string]string{
				"X-Service": "user-service",
			},
		},
		{
			Endpoint: "/profiles",
			Method:   enums.POST,
			Transform: func(body interface{}) interface{} {
				// Transform for profile service
				originalData := body.(map[string]interface{})
				return map[string]interface{}{
					"full_name":    originalData["name"],
					"email_addr":   originalData["email"],
					"profile_type": "standard",
					"source":       originalData["source"],
				}
			},
			Headers: map[string]string{
				"X-Service": "profile-service",
			},
		},
		{
			Endpoint: "/analytics",
			Method:   enums.POST,
			Transform: func(body interface{}) interface{} {
				// Analytics service only needs specific fields
				originalData := body.(map[string]interface{})
				return map[string]interface{}{
					"event":    "user_created",
					"user_ref": originalData["email"],
					"source":   originalData["source"],
					"metadata": map[string]interface{}{
						"priority": originalData["priority"],
					},
				}
			},
			Headers: map[string]string{
				"X-Service": "analytics-service",
			},
		},
	}

	// Execute sequentially to maintain order
	result := client.
		Headers(map[string]string{"Authorization": "Bearer api-key-123"}).
		Body(userData).
		ExecuteMultiEndpoints(targets, false)

	fmt.Printf("Data broadcast results: %d successful, %d failed\n",
		result.SuccessCount, result.FailureCount)

	if result.HasErrors {
		fmt.Println("Failed endpoints:")
		for endpoint, err := range result.GetErrorSummary() {
			fmt.Printf("  %s: %s\n", endpoint, err.Message)
		}
	}
}

// Example 3: User onboarding workflow with dependent steps using DAG workflow
func userOnboardingDAGWorkflow(client *gorest.NetworkClient) {
	var authResp AuthResponse
	var userResp UserResponse

	// Create DAG steps
	steps := []*gorest.DAGWorkflowStep{
		{
			Name:     "authenticate",
			Endpoint: "/auth/login",
			Method:   enums.POST,
			BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
				return AuthRequest{
					Username: "admin",
					Password: "admin-secret",
				}
			},
			Response:    &authResp,
			StopOnError: true,
		},
		{
			Name:         "create_user",
			Endpoint:     "/users",
			Method:       enums.POST,
			Dependencies: []string{"authenticate"},
			BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
				// Use initial user data and add creator ID from auth
				userData := ctx.InitialBody.(UserRequest)
				authResult := ctx.GetStepResult("authenticate")
				if authResult.Success {
					auth := authResult.ResponseData.(*AuthResponse)
					userData.CreatedBy = auth.UserID
				}
				return userData
			},
			HeadersBuilder: func(ctx *gorest.WorkflowContext) map[string]string {
				// Add auth token from previous step
				authResult := ctx.GetStepResult("authenticate")
				if authResult.Success {
					auth := authResult.ResponseData.(*AuthResponse)
					return map[string]string{
						"Authorization": "Bearer " + auth.Token,
					}
				}
				return nil
			},
			Response:    &userResp,
			StopOnError: true,
		},
		{
			Name:         "send_welcome_notification",
			Endpoint:     "/notifications",
			Method:       enums.POST,
			Dependencies: []string{"create_user"},
			BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
				userResult := ctx.GetStepResult("create_user")
				if userResult.Success {
					user := userResult.ResponseData.(*UserResponse)
					return NotificationRequest{
						UserID:   user.ID,
						Type:     "email",
						Template: "welcome",
						Data: map[string]interface{}{
							"name":  user.Name,
							"email": user.Email,
						},
					}
				}
				return nil
			},
			HeadersBuilder: func(ctx *gorest.WorkflowContext) map[string]string {
				// Reuse auth token
				authResult := ctx.GetStepResult("authenticate")
				if authResult.Success {
					auth := authResult.ResponseData.(*AuthResponse)
					return map[string]string{
						"Authorization": "Bearer " + auth.Token,
					}
				}
				return nil
			},
			StopOnError: false, // Don't fail workflow if notification fails
		},
		{
			Name:         "log_user_creation",
			Endpoint:     "/audit/log",
			Method:       enums.POST,
			Dependencies: []string{"create_user", "authenticate"},
			BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
				userResult := ctx.GetStepResult("create_user")
				authResult := ctx.GetStepResult("authenticate")

				if userResult.Success && authResult.Success {
					user := userResult.ResponseData.(*UserResponse)
					auth := authResult.ResponseData.(*AuthResponse)

					return map[string]interface{}{
						"action":     "user_created",
						"user_id":    user.ID,
						"created_by": auth.UserID,
						"timestamp":  time.Now().Unix(),
					}
				}
				return nil
			},
			HeadersBuilder: func(ctx *gorest.WorkflowContext) map[string]string {
				authResult := ctx.GetStepResult("authenticate")
				if authResult.Success {
					auth := authResult.ResponseData.(*AuthResponse)
					return map[string]string{
						"Authorization": "Bearer " + auth.Token,
					}
				}
				return nil
			},
			StopOnError: false, // Audit logging failure shouldn't stop workflow
		},
	}

	newUser := UserRequest{
		Name:  "John Doe",
		Email: "john@example.com",
	}

	// Execute the workflow
	response := client.Host("https://api.example.com").ExecuteDAGWorkflow(steps, newUser, nil)

	// Process results
	fmt.Printf("DAG Workflow completed: Success=%t\n", response.Success)
	fmt.Printf("Completed steps: %d\n", response.CompletedSteps)

	if response.Success {
		fmt.Printf("User created successfully: %+v\n", userResp)
		fmt.Printf("Authentication token expires in: %d seconds\n", authResp.ExpiresIn)
	} else {
		fmt.Println("Workflow failed")
		if len(response.FailedSteps) > 0 {
			fmt.Printf("Failed step: %s\n", response.FailedSteps[0].Step.Name)
		}
	}

	// Show detailed step results
	fmt.Println("\nStep-by-step results:")
	for stepName, result := range response.StepResults {
		status := "SUCCESS"
		if result.Skipped {
			status = "SKIPPED"
		} else if !result.Success {
			status = "FAILED"
		}
		fmt.Printf("  %s: %s\n", stepName, status)
	}

	// Show parallelism stats
	fmt.Printf("\nParallelism statistics:\n")
	fmt.Printf("  Execution layers: %d\n", response.ParallelismStats.TotalExecutionLayers)
	fmt.Printf("  Max concurrent steps: %d\n", response.ParallelismStats.MaxConcurrentSteps)
}

// Example 4: Advanced DAG workflow with conditional execution
func conditionalDAGWorkflowExample(client *gorest.NetworkClient) {
	var userCheck map[string]interface{}
	var premiumResp map[string]interface{}
	var basicResp map[string]interface{}

	// Create DAG steps
	steps := []*gorest.DAGWorkflowStep{
		{
			Name:        "check_user_tier",
			Endpoint:    "/users/123/tier",
			Method:      enums.GET,
			Response:    &userCheck,
			StopOnError: true,
		},
		{
			Name:         "activate_premium_features",
			Endpoint:     "/features/premium",
			Method:       enums.POST,
			Dependencies: []string{"check_user_tier"},
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
					"user_id": 123,
					"features": []string{
						"advanced_analytics",
						"priority_support",
						"custom_integrations",
					},
				}
			},
			Response: &premiumResp,
		},
		{
			Name:         "activate_basic_features",
			Endpoint:     "/features/basic",
			Method:       enums.POST,
			Dependencies: []string{"check_user_tier"},
			Condition: func(ctx *gorest.WorkflowContext) bool {
				// Only execute if user is not premium
				tierResult := ctx.GetStepResult("check_user_tier")
				if tierResult != nil && tierResult.Success {
					tierData := tierResult.ResponseData.(*map[string]interface{})
					if tier, exists := (*tierData)["tier"]; exists {
						return tier != "premium"
					}
				}
				return true
			},
			BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
				return map[string]interface{}{
					"user_id": 123,
					"features": []string{
						"basic_analytics",
						"standard_support",
					},
				}
			},
			Response: &basicResp,
		},
		{
			Name:     "send_activation_email",
			Endpoint: "/notifications/activation",
			Method:   enums.POST,
			Dependencies: []string{
				"check_user_tier",
				"activate_premium_features",
				"activate_basic_features",
			},
			BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
				tierResult := ctx.GetStepResult("check_user_tier")

				emailData := map[string]interface{}{
					"user_id":  123,
					"template": "feature_activation",
				}

				if tierResult.Success {
					tierData := tierResult.ResponseData.(*map[string]interface{})
					emailData["tier"] = (*tierData)["tier"]
				}

				return emailData
			},
		},
	}

	// Execute the workflow
	response := client.
		Host("https://features.example.com").
		Headers(map[string]string{"Authorization": "Bearer user-token-456"}).
		ExecuteDAGWorkflow(steps, nil, nil)

	fmt.Printf("Feature activation workflow completed: Success=%t\n", response.Success)
	fmt.Printf("Completed steps: %d\n", response.CompletedSteps)

	// Check which path was taken
	premiumStep := response.StepResults["activate_premium_features"]
	basicStep := response.StepResults["activate_basic_features"]

	if premiumStep != nil && premiumStep.Success && !premiumStep.Skipped {
		fmt.Println("Premium features activated!")
		fmt.Printf("Features: %+v\n", premiumResp)
	} else if basicStep != nil && basicStep.Success && !basicStep.Skipped {
		fmt.Println("Basic features activated!")
		fmt.Printf("Features: %+v\n", basicResp)
	}

	skippedSteps := []string{}
	for stepName, result := range response.StepResults {
		if result.Skipped {
			skippedSteps = append(skippedSteps, stepName)
		}
	}

	if len(skippedSteps) > 0 {
		fmt.Printf("Skipped %d steps based on conditions: %v\n", len(skippedSteps), skippedSteps)
	}
}

// Example 5: DAG Workflow with error handling and recovery
func dagWorkflowErrorHandlingExample(client *gorest.NetworkClient) {
	// Create DAG steps
	steps := []*gorest.DAGWorkflowStep{
		{
			Name:     "critical_step",
			Endpoint: "/critical/operation",
			Method:   enums.POST,
			BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
				return map[string]interface{}{
					"operation": "critical_task",
					"data":      "important_data",
				}
			},
			StopOnError: true, // This must succeed
		},
		{
			Name:         "optional_step_1",
			Endpoint:     "/optional/operation1",
			Method:       enums.POST,
			Dependencies: []string{"critical_step"},
			BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
				return map[string]interface{}{
					"operation": "optional_task_1",
				}
			},
			StopOnError: false, // Continue even if this fails
		},
		{
			Name:         "optional_step_2",
			Endpoint:     "/optional/operation2",
			Method:       enums.POST,
			Dependencies: []string{"critical_step"},
			BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
				return map[string]interface{}{
					"operation": "optional_task_2",
				}
			},
			StopOnError: false, // Continue even if this fails
		},
		{
			Name:         "cleanup",
			Endpoint:     "/cleanup",
			Method:       enums.POST,
			Dependencies: []string{"critical_step", "optional_step_1", "optional_step_2"},
			BodyBuilder: func(ctx *gorest.WorkflowContext) interface{} {
				// Always run cleanup, collect any failed steps
				failedSteps := []string{}
				for stepName, result := range ctx.StepResults {
					if !result.Success && !result.Skipped {
						failedSteps = append(failedSteps, stepName)
					}
				}

				return map[string]interface{}{
					"cleanup_type": "workflow",
					"failed_steps": failedSteps,
				}
			},
			StopOnError: false, // Don't fail workflow if cleanup fails
		},
	}

	// Add metadata for tracking
	metadata := map[string]interface{}{
		"workflow_id": "error-handling-example",
		"started_at":  time.Now().Unix(),
	}

	// Execute the workflow with options
	response := client.
		Host("https://resilient-api.example.com").
		Headers(map[string]string{"X-Workflow-ID": "error-handling-example"}).
		ExecuteDAGWorkflowWithOptions(steps, nil, metadata, 2, false)

	// Comprehensive error analysis
	fmt.Printf("DAG workflow execution completed: Success=%t\n", response.Success)
	fmt.Printf("Completed steps: %d\n", response.CompletedSteps)

	if response.Success {
		fmt.Println("All steps completed successfully!")
	} else {
		fmt.Println("Workflow had errors but continued execution of non-critical steps.")
		// Detailed error analysis
		errorSteps := []string{}
		for stepName, result := range response.StepResults {
			if !result.Success && !result.Skipped {
				errorSteps = append(errorSteps, stepName)
				fmt.Printf("Step %s failed: %v\n", stepName, result.Error)
			}
		}
	}

	// Show what completed successfully
	successfulSteps := []string{}
	for stepName, result := range response.StepResults {
		if result.Success {
			successfulSteps = append(successfulSteps, stepName)
		}
	}

	if len(successfulSteps) > 0 {
		fmt.Printf("\nSuccessfully completed %d steps: %v\n", len(successfulSteps), successfulSteps)
	}

	// Show final metadata
	fmt.Printf("\nWorkflow ID: %s\n", metadata["workflow_id"])

	// Show parallelism statistics
	fmt.Printf("\nParallelism Statistics:\n")
	fmt.Printf("  Execution layers: %d\n", response.ParallelismStats.TotalExecutionLayers)
	fmt.Printf("  Max concurrent steps: %d\n", response.ParallelismStats.MaxConcurrentSteps)
}

// Utility function to safely copy interface map
func copyInterfaceMap(original map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{})
	for key, value := range original {
		copy[key] = value
	}
	return copy
}
