package gorest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/xander1235/gorest/constants/enums"
	"github.com/xander1235/gorest/exceptions/errors"
)

// Test models for workflow tests
type AuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token  string `json:"token"`
	UserID int    `json:"user_id"`
}

type CreateUserRequest struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	CreatedBy int    `json:"created_by"`
}

type UserResponse struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type EmailRequest struct {
	To       string                 `json:"to"`
	Template string                 `json:"template"`
	Data     map[string]interface{} `json:"data"`
}

func TestExecuteWorkflow_SuccessfulFlow(t *testing.T) {
	// Track execution order
	executionOrder := []string{}
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executionOrder = append(executionOrder, r.URL.Path)
		
		switch r.URL.Path {
		case "/auth/login":
			// Simulate authentication
			var authReq AuthRequest
			json.NewDecoder(r.Body).Decode(&authReq)
			
			if authReq.Username == "admin" && authReq.Password == "secret" {
				response := AuthResponse{Token: "jwt-token-123", UserID: 1}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(response)
			} else {
				w.WriteHeader(http.StatusUnauthorized)
			}
		
		case "/users":
			// Verify authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader != "Bearer jwt-token-123" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			
			var userReq CreateUserRequest
			json.NewDecoder(r.Body).Decode(&userReq)
			
			response := UserResponse{
				ID:    123,
				Name:  userReq.Name,
				Email: userReq.Email,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		
		case "/notifications/email":
			var emailReq EmailRequest
			json.NewDecoder(r.Body).Decode(&emailReq)
			
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"sent": true}`))
		
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient()
	
	var authResp AuthResponse
	var userResp UserResponse
	
	steps := []*WorkflowStep{
		{
			Name:     "authenticate",
			Endpoint: "/auth/login",
			Method:   enums.POST,
			BodyBuilder: func(ctx *WorkflowContext) interface{} {
				return AuthRequest{
					Username: "admin",
					Password: "secret",
				}
			},
			Response:    &authResp,
			StopOnError: true,
		},
		{
			Name:     "create_user",
			Endpoint: "/users",
			Method:   enums.POST,
			BodyBuilder: func(ctx *WorkflowContext) interface{} {
				// Use initial body and add authenticated user ID
				userData := ctx.InitialBody.(map[string]interface{})
				authResult := ctx.GetStepResult("authenticate")
				if authResult != nil && authResult.Success {
					auth := authResult.ResponseData.(*AuthResponse)
					return CreateUserRequest{
						Name:      userData["name"].(string),
						Email:     userData["email"].(string),
						CreatedBy: auth.UserID,
					}
				}
				return nil
			},
			HeadersBuilder: func(ctx *WorkflowContext) map[string]string {
				authResult := ctx.GetStepResult("authenticate")
				if authResult != nil && authResult.Success {
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
			Name:     "send_welcome_email",
			Endpoint: "/notifications/email",
			Method:   enums.POST,
			BodyBuilder: func(ctx *WorkflowContext) interface{} {
				userResult := ctx.GetStepResult("create_user")
				if userResult != nil && userResult.Success {
					user := userResult.ResponseData.(*UserResponse)
					return EmailRequest{
						To:       user.Email,
						Template: "welcome",
						Data: map[string]interface{}{
							"name": user.Name,
						},
					}
				}
				return nil
			},
			StopOnError: false, // Don't fail workflow if email fails
		},
	}

	userData := map[string]interface{}{
		"name":  "John Doe",
		"email": "john@example.com",
	}

	result := client.Host(server.URL).ExecuteWorkflow(steps, userData, nil)

	// Verify workflow succeeded
	assert.True(t, result.Success, "Workflow should succeed")
	assert.Equal(t, 3, result.CompletedSteps, "All 3 steps should complete")
	assert.Equal(t, 0, result.SkippedSteps, "No steps should be skipped")
	assert.Nil(t, result.FailedStep, "No step should fail")

	// Verify execution order
	expectedOrder := []string{"/auth/login", "/users", "/notifications/email"}
	assert.Equal(t, expectedOrder, executionOrder, "Steps should execute in order")

	// Verify responses were populated
	assert.Equal(t, "jwt-token-123", authResp.Token)
	assert.Equal(t, 1, authResp.UserID)
	assert.Equal(t, 123, userResp.ID)
	assert.Equal(t, "John Doe", userResp.Name)

	// Verify context has all step results
	assert.Equal(t, 3, len(result.Context.StepResults))
	assert.True(t, result.Context.HasSuccessfulStep("authenticate"))
	assert.True(t, result.Context.HasSuccessfulStep("create_user"))
	assert.True(t, result.Context.HasSuccessfulStep("send_welcome_email"))
}

func TestExecuteWorkflow_StopOnError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/step1":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success": true}`))
		case "/step2":
			// This step fails
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "server error"}`))
		case "/step3":
			// This should not be reached
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success": true}`))
		}
	}))
	defer server.Close()

	client := NewClient()
	
	steps := []*WorkflowStep{
		{
			Name:        "step1",
			Endpoint:    "/step1",
			Method:      enums.GET,
			StopOnError: true,
		},
		{
			Name:        "step2",
			Endpoint:    "/step2",
			Method:      enums.GET,
			StopOnError: true, // This will stop the workflow
		},
		{
			Name:        "step3",
			Endpoint:    "/step3",
			Method:      enums.GET,
			StopOnError: true,
		},
	}

	result := client.Host(server.URL).ExecuteWorkflow(steps, nil, nil)

	// Verify workflow failed
	assert.False(t, result.Success, "Workflow should fail")
	assert.Equal(t, 1, result.CompletedSteps, "Only first step should complete")
	assert.Equal(t, 1, result.SkippedSteps, "Last step should be skipped")
	assert.NotNil(t, result.FailedStep, "Should have failed step")
	assert.Equal(t, "step2", result.FailedStep.Step.Name, "Step2 should be the failed step")

	// Verify step states
	step1Result := result.GetStepByName("step1")
	step2Result := result.GetStepByName("step2")
	step3Result := result.GetStepByName("step3")
	
	assert.True(t, step1Result.Success)
	assert.False(t, step1Result.Skipped)
	
	assert.False(t, step2Result.Success)
	assert.False(t, step2Result.Skipped)
	
	assert.False(t, step3Result.Success)
	assert.True(t, step3Result.Skipped) // Should be marked as skipped
}

func TestExecuteWorkflow_ContinueOnError(t *testing.T) {
	executionOrder := []string{}
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executionOrder = append(executionOrder, r.URL.Path)
		
		switch r.URL.Path {
		case "/step1":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success": true}`))
		case "/step2":
			// This step fails but workflow continues
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "server error"}`))
		case "/step3":
			// This should still execute
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"success": true}`))
		}
	}))
	defer server.Close()

	client := NewClient()
	
	steps := []*WorkflowStep{
		{
			Name:        "step1",
			Endpoint:    "/step1",
			Method:      enums.GET,
			StopOnError: true,
		},
		{
			Name:        "step2",
			Endpoint:    "/step2",
			Method:      enums.GET,
			StopOnError: false, // Continue even if this fails
		},
		{
			Name:        "step3",
			Endpoint:    "/step3",
			Method:      enums.GET,
			StopOnError: true,
		},
	}

	result := client.Host(server.URL).ExecuteWorkflow(steps, nil, nil)

	// Verify workflow had errors but continued
	assert.False(t, result.Success, "Workflow should fail due to step2 error")
	assert.Equal(t, 2, result.CompletedSteps, "Two steps should complete successfully")
	assert.Equal(t, 0, result.SkippedSteps, "No steps should be skipped")
	assert.NotNil(t, result.FailedStep, "Should have failed step")

	// Verify all steps were executed
	expectedOrder := []string{"/step1", "/step2", "/step3"}
	assert.Equal(t, expectedOrder, executionOrder, "All steps should execute")

	// Verify individual step results
	failedSteps := result.GetFailedSteps()
	successfulSteps := result.GetSuccessfulSteps()
	
	assert.Len(t, failedSteps, 1)
	assert.Equal(t, "step2", failedSteps[0].Step.Name)
	
	assert.Len(t, successfulSteps, 2)
}

func TestExecuteWorkflow_ConditionalSteps(t *testing.T) {
	executionOrder := []string{}
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		executionOrder = append(executionOrder, r.URL.Path)
		
		switch r.URL.Path {
		case "/check_premium":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"is_premium": true}`))
		case "/premium_feature":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"enabled": true}`))
		case "/basic_feature":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"enabled": true}`))
		}
	}))
	defer server.Close()

	client := NewClient()
	
	var premiumCheck map[string]interface{}
	
	steps := []*WorkflowStep{
		{
			Name:     "check_premium",
			Endpoint: "/check_premium",
			Method:   enums.GET,
			Response: &premiumCheck,
		},
		{
			Name:     "premium_feature",
			Endpoint: "/premium_feature",
			Method:   enums.GET,
			Condition: func(ctx *WorkflowContext) bool {
				// Only execute if user is premium
				checkResult := ctx.GetStepResult("check_premium")
				if checkResult != nil && checkResult.Success {
					premium := checkResult.ResponseData.(*map[string]interface{})
					if isPremium, exists := (*premium)["is_premium"]; exists {
						return isPremium.(bool)
					}
				}
				return false
			},
		},
		{
			Name:     "basic_feature",
			Endpoint: "/basic_feature",
			Method:   enums.GET,
			Condition: func(ctx *WorkflowContext) bool {
				// Only execute if user is not premium
				checkResult := ctx.GetStepResult("check_premium")
				if checkResult != nil && checkResult.Success {
					premium := checkResult.ResponseData.(*map[string]interface{})
					if isPremium, exists := (*premium)["is_premium"]; exists {
						return !isPremium.(bool)
					}
				}
				return true
			},
		},
	}

	result := client.Host(server.URL).ExecuteWorkflow(steps, nil, nil)

	// Verify workflow results
	assert.True(t, result.Success, "Workflow should succeed")
	assert.Equal(t, 2, result.CompletedSteps, "Two steps should complete")
	assert.Equal(t, 1, result.SkippedSteps, "One step should be skipped")

	// Verify execution order (basic_feature should be skipped)
	expectedOrder := []string{"/check_premium", "/premium_feature"}
	assert.Equal(t, expectedOrder, executionOrder, "Only premium feature should execute")

	// Verify step states
	checkResult := result.GetStepByName("check_premium")
	premiumResult := result.GetStepByName("premium_feature")
	basicResult := result.GetStepByName("basic_feature")
	
	assert.True(t, checkResult.Success)
	assert.False(t, checkResult.Skipped)
	
	assert.True(t, premiumResult.Success)
	assert.False(t, premiumResult.Skipped)
	
	assert.True(t, basicResult.Success) // Skipped steps are marked as successful
	assert.True(t, basicResult.Skipped)
}

func TestExecuteWorkflow_WithMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	client := NewClient()
	
	initialMetadata := map[string]interface{}{
		"workflow_id": "test-123",
		"version":     "1.0",
	}
	
	steps := []*WorkflowStep{
		{
			Name:     "step1",
			Endpoint: "/step1",
			Method:   enums.GET,
			BodyBuilder: func(ctx *WorkflowContext) interface{} {
				// Add metadata to step execution
				ctx.SetMetadata("step1_executed", true)
				ctx.SetMetadata("execution_time", time.Now().Unix())
				
				return map[string]interface{}{
					"workflow_id": ctx.GetMetadata("workflow_id"),
					"version":     ctx.GetMetadata("version"),
				}
			},
		},
		{
			Name:     "step2",
			Endpoint: "/step2",
			Method:   enums.GET,
			BodyBuilder: func(ctx *WorkflowContext) interface{} {
				// Check if step1 was executed
				step1Executed := ctx.GetMetadata("step1_executed")
				
				return map[string]interface{}{
					"previous_step_executed": step1Executed,
					"workflow_id":            ctx.GetMetadata("workflow_id"),
				}
			},
		},
	}

	result := client.Host(server.URL).ExecuteWorkflow(steps, nil, initialMetadata)

	// Verify workflow succeeded
	assert.True(t, result.Success)
	assert.Equal(t, 2, result.CompletedSteps)

	// Verify metadata was preserved and updated
	finalContext := result.Context
	assert.Equal(t, "test-123", finalContext.GetMetadata("workflow_id"))
	assert.Equal(t, "1.0", finalContext.GetMetadata("version"))
	assert.Equal(t, true, finalContext.GetMetadata("step1_executed"))
	assert.NotNil(t, finalContext.GetMetadata("execution_time"))
}

func TestExecuteWorkflow_WithStepTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			// Simulate slow endpoint
			time.Sleep(200 * time.Millisecond)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	client := NewClient()
	
	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	
	steps := []*WorkflowStep{
		{
			Name:     "fast_step",
			Endpoint: "/fast",
			Method:   enums.GET,
		},
		{
			Name:        "slow_step",
			Endpoint:    "/slow",
			Method:      enums.GET,
			Context:     ctx, // This should timeout
			StopOnError: true,
		},
		{
			Name:     "final_step",
			Endpoint: "/final",
			Method:   enums.GET,
		},
	}

	result := client.Host(server.URL).ExecuteWorkflow(steps, nil, nil)

	// Verify workflow failed due to timeout
	assert.False(t, result.Success)
	assert.Equal(t, 1, result.CompletedSteps, "Only first step should complete")
	assert.Equal(t, 1, result.SkippedSteps, "Last step should be skipped due to timeout")
	assert.NotNil(t, result.FailedStep)
	assert.Equal(t, "slow_step", result.FailedStep.Step.Name)
}

func TestWorkflowContext_HelperMethods(t *testing.T) {
	// Create mock workflow context
	ctx := &WorkflowContext{
		StepResults: make(map[string]*WorkflowStepResult),
		Metadata:    make(map[string]interface{}),
		CurrentStep: 1,
	}

	// Add some step results
	successResult := &WorkflowStepResult{
		Step:         &WorkflowStep{Name: "success_step"},
		Success:      true,
		ResponseData: &TestUser{ID: 1, Name: "John"},
	}
	
	failureResult := &WorkflowStepResult{
		Step:    &WorkflowStep{Name: "failure_step"},
		Success: false,
		Error:   &errors.ErrorDetails{Message: "Failed"},
	}
	
	ctx.StepResults["success_step"] = successResult
	ctx.StepResults["failure_step"] = failureResult

	// Test GetStepResult
	result := ctx.GetStepResult("success_step")
	assert.NotNil(t, result)
	assert.True(t, result.Success)

	// Test GetStepResponse
	response := ctx.GetStepResponse("success_step")
	assert.NotNil(t, response)
	user := response.(*TestUser)
	assert.Equal(t, "John", user.Name)

	// Test GetStepResponse for failed step
	failedResponse := ctx.GetStepResponse("failure_step")
	assert.Nil(t, failedResponse, "Failed step should not return response")

	// Test HasSuccessfulStep
	assert.True(t, ctx.HasSuccessfulStep("success_step"))
	assert.False(t, ctx.HasSuccessfulStep("failure_step"))
	assert.False(t, ctx.HasSuccessfulStep("nonexistent_step"))

	// Test metadata methods
	ctx.SetMetadata("test_key", "test_value")
	assert.Equal(t, "test_value", ctx.GetMetadata("test_key"))
	assert.Nil(t, ctx.GetMetadata("nonexistent_key"))
}

func TestWorkflowResponse_HelperMethods(t *testing.T) {
	// Create mock workflow response
	successStep := &WorkflowStepResult{
		Step:    &WorkflowStep{Name: "success"},
		Success: true,
		Skipped: false,
	}
	
	failedStep := &WorkflowStepResult{
		Step:    &WorkflowStep{Name: "failed"},
		Success: false,
		Skipped: false,
		Error:   &errors.ErrorDetails{Message: "Failed"},
	}
	
	skippedStep := &WorkflowStepResult{
		Step:    &WorkflowStep{Name: "skipped"},
		Success: true,
		Skipped: true,
	}

	response := &WorkflowResponse{
		StepResults:    []*WorkflowStepResult{successStep, failedStep, skippedStep},
		Success:        false,
		CompletedSteps: 1,
		SkippedSteps:   1,
		FailedStep:     failedStep,
	}

	// Test GetSuccessfulSteps (should exclude skipped ones)
	successfulSteps := response.GetSuccessfulSteps()
	assert.Len(t, successfulSteps, 1)
	assert.Equal(t, "success", successfulSteps[0].Step.Name)

	// Test GetFailedSteps
	failedSteps := response.GetFailedSteps()
	assert.Len(t, failedSteps, 1)
	assert.Equal(t, "failed", failedSteps[0].Step.Name)

	// Test GetSkippedSteps
	skippedSteps := response.GetSkippedSteps()
	assert.Len(t, skippedSteps, 1)
	assert.Equal(t, "skipped", skippedSteps[0].Step.Name)

	// Test GetStepByName
	step := response.GetStepByName("success")
	assert.NotNil(t, step)
	assert.Equal(t, "success", step.Step.Name)
	
	nonExistentStep := response.GetStepByName("nonexistent")
	assert.Nil(t, nonExistentStep)

	// Test GetErrorSummary
	errorSummary := response.GetErrorSummary()
	assert.Len(t, errorSummary, 1)
	assert.Contains(t, errorSummary, "failed")
	assert.Equal(t, "Failed", errorSummary["failed"].Message)

	// Test String method
	summary := response.String()
	expected := "Workflow: 3 total steps, 1 completed, 1 skipped, 1 failed"
	assert.Equal(t, expected, summary)
}

// Benchmark tests for workflow execution
func BenchmarkExecuteWorkflow_SimpleFlow(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	client := NewClient()
	
	steps := []*WorkflowStep{
		{
			Name:     "step1",
			Endpoint: "/step1",
			Method:   enums.GET,
		},
		{
			Name:     "step2",
			Endpoint: "/step2",
			Method:   enums.GET,
		},
		{
			Name:     "step3",
			Endpoint: "/step3",
			Method:   enums.GET,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := client.Host(server.URL).ExecuteWorkflow(steps, nil, nil)
		if !result.Success {
			b.Fatal("Workflow failed")
		}
	}
}

func BenchmarkExecuteWorkflow_WithDataTransformation(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id": 123, "name": "John"}`))
	}))
	defer server.Close()

	client := NewClient()
	
	var response1, response2, response3 map[string]interface{}
	
	steps := []*WorkflowStep{
		{
			Name:     "step1",
			Endpoint: "/step1",
			Method:   enums.GET,
			Response: &response1,
		},
		{
			Name:     "step2",
			Endpoint: "/step2",
			Method:   enums.POST,
			BodyBuilder: func(ctx *WorkflowContext) interface{} {
				step1Result := ctx.GetStepResponse("step1")
				if data, ok := step1Result.(*map[string]interface{}); ok {
					return map[string]interface{}{
						"previous_id":   (*data)["id"],
						"previous_name": (*data)["name"],
						"step":          2,
					}
				}
				return nil
			},
			Response: &response2,
		},
		{
			Name:     "step3",
			Endpoint: "/step3",
			Method:   enums.POST,
			BodyBuilder: func(ctx *WorkflowContext) interface{} {
				step2Result := ctx.GetStepResponse("step2")
				if data, ok := step2Result.(*map[string]interface{}); ok {
					return map[string]interface{}{
						"final_data": data,
						"step":       3,
					}
				}
				return nil
			},
			Response: &response3,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := client.Host(server.URL).ExecuteWorkflow(steps, nil, nil)
		if !result.Success {
			b.Fatal("Workflow failed")
		}
	}
}
