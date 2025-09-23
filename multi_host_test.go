package gorest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xander1235/gorest/v2/constants/enums"
)

// Test models for multi-host tests
type ServiceResponse struct {
	Service string `json:"service"`
	Status  string `json:"status"`
	Host    string `json:"host"`
}

func TestMultiEndpoints_DifferentHosts(t *testing.T) {
	// Create multiple test servers to simulate different services
	service1Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ServiceResponse{
			Service: "service1",
			Status:  "ok",
			Host:    r.Host,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer service1Server.Close()

	service2Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ServiceResponse{
			Service: "service2",
			Status:  "healthy",
			Host:    r.Host,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer service2Server.Close()

	service3Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ServiceResponse{
			Service: "service3",
			Status:  "running",
			Host:    r.Host,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer service3Server.Close()

	client := NewClient()

	var service1Resp, service2Resp, service3Resp ServiceResponse

	targets := []*MultiRequestTarget{
		{
			Endpoint: "/status",
			Method:   enums.GET,
			Host:     service1Server.URL, // Different host for each target
			Response: &service1Resp,
		},
		{
			Endpoint: "/health",
			Method:   enums.GET,
			Host:     service2Server.URL,
			Response: &service2Resp,
		},
		{
			Endpoint: "/ping",
			Method:   enums.GET,
			Host:     service3Server.URL,
			Response: &service3Resp,
		},
	}

	// Execute requests to different hosts in parallel
	result := client.ExecuteMultiEndpoints(targets, true)

	// Verify all requests succeeded
	assert.True(t, result.Success, "All requests should succeed")
	assert.Equal(t, 3, result.SuccessCount, "All 3 requests should succeed")
	assert.Equal(t, 0, result.FailureCount, "No requests should fail")

	// Verify each service responded correctly
	assert.Equal(t, "service1", service1Resp.Service)
	assert.Equal(t, "ok", service1Resp.Status)

	assert.Equal(t, "service2", service2Resp.Service)
	assert.Equal(t, "healthy", service2Resp.Status)

	assert.Equal(t, "service3", service3Resp.Service)
	assert.Equal(t, "running", service3Resp.Status)
}

func TestMultiEndpoints_MixedHosts_ClientFallback(t *testing.T) {
	// Create servers for different services
	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ServiceResponse{
			Service: "primary",
			Status:  "ok",
			Host:    r.Host,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer primaryServer.Close()

	secondaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ServiceResponse{
			Service: "secondary",
			Status:  "healthy",
			Host:    r.Host,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer secondaryServer.Close()

	client := NewClient()

	var primaryResp, secondaryResp, fallbackResp ServiceResponse

	targets := []*MultiRequestTarget{
		{
			Endpoint: "/endpoint1",
			Method:   enums.GET,
			Host:     primaryServer.URL, // Specific host
			Response: &primaryResp,
		},
		{
			Endpoint: "/endpoint2",
			Method:   enums.GET,
			Host:     secondaryServer.URL, // Different specific host
			Response: &secondaryResp,
		},
		{
			Endpoint: "/endpoint3",
			Method:   enums.GET,
			// No Host specified - should fallback to client host
			Response: &fallbackResp,
		},
	}

	// Set client host as fallback and execute
	result := client.Host(primaryServer.URL).ExecuteMultiEndpoints(targets, false)

	// Verify all requests succeeded
	assert.True(t, result.Success, "All requests should succeed")
	assert.Equal(t, 3, result.SuccessCount, "All 3 requests should succeed")

	// Verify responses
	assert.Equal(t, "primary", primaryResp.Service)
	assert.Equal(t, "secondary", secondaryResp.Service)
	assert.Equal(t, "primary", fallbackResp.Service) // Should use client host (primary)
}

func TestMultiEndpoints_NoHostError(t *testing.T) {
	client := NewClient()

	targets := []*MultiRequestTarget{
		{
			Endpoint: "/test",
			Method:   enums.GET,
			// No Host specified and no client host
		},
	}

	// Execute without setting any host
	result := client.ExecuteMultiEndpoints(targets, false)

	// Should fail with clear error message
	assert.False(t, result.Success, "Should fail due to missing host")
	assert.Equal(t, 0, result.SuccessCount, "No requests should succeed")
	assert.Equal(t, 1, result.FailureCount, "One request should fail")
	assert.True(t, result.HasErrors, "Should have errors")

	// Check error message
	failedResult := result.Results[0]
	assert.False(t, failedResult.Success)
	assert.NotNil(t, failedResult.Error)
	assert.Contains(t, failedResult.Error.Message, "No host specified for endpoint /test")
	assert.Contains(t, failedResult.Error.Message, "Provide host via target.Host or client.Host()")
}

func TestWorkflow_DifferentHosts(t *testing.T) {
	// Create multiple servers for different workflow steps
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var authReq map[string]interface{}
		json.NewDecoder(r.Body).Decode(&authReq)

		if authReq["username"] == "admin" && authReq["password"] == "secret" {
			response := map[string]interface{}{
				"token":   "jwt-token-123",
				"user_id": 1,
				"service": "auth-service",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		} else {
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer authServer.Close()

	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer jwt-token-123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var userReq map[string]interface{}
		json.NewDecoder(r.Body).Decode(&userReq)

		response := map[string]interface{}{
			"id":      123,
			"name":    userReq["name"],
			"email":   userReq["email"],
			"service": "user-service",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer userServer.Close()

	notificationServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var notifReq map[string]interface{}
		json.NewDecoder(r.Body).Decode(&notifReq)

		response := map[string]interface{}{
			"sent":    true,
			"to":      notifReq["to"],
			"service": "notification-service",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer notificationServer.Close()

	client := NewClient()

	var authResp, userResp, notifResp map[string]interface{}

	steps := []*WorkflowStep{
		{
			Name:     "authenticate",
			Endpoint: "/login",
			Method:   enums.POST,
			Host:     authServer.URL, // Auth service host
			BodyBuilder: func(ctx *WorkflowContext) interface{} {
				return map[string]string{
					"username": "admin",
					"password": "secret",
				}
			},
			Response:    &authResp,
			StopOnError: true,
		},
		{
			Name:     "create_user",
			Endpoint: "/users",
			Method:   enums.POST,
			Host:     userServer.URL, // User service host
			BodyBuilder: func(ctx *WorkflowContext) interface{} {
				userData := ctx.InitialBody.(map[string]interface{})
				authResult := ctx.GetStepResult("authenticate")
				if authResult.Success {
					auth := authResult.ResponseData.(*map[string]interface{})
					userData["created_by"] = (*auth)["user_id"]
				}
				return userData
			},
			HeadersBuilder: func(ctx *WorkflowContext) map[string]string {
				authResult := ctx.GetStepResult("authenticate")
				if authResult.Success {
					auth := authResult.ResponseData.(*map[string]interface{})
					return map[string]string{
						"Authorization": "Bearer " + (*auth)["token"].(string),
					}
				}
				return nil
			},
			Response:    &userResp,
			StopOnError: true,
		},
		{
			Name:     "send_notification",
			Endpoint: "/notify",
			Method:   enums.POST,
			Host:     notificationServer.URL, // Notification service host
			BodyBuilder: func(ctx *WorkflowContext) interface{} {
				userResult := ctx.GetStepResult("create_user")
				if userResult.Success {
					user := userResult.ResponseData.(*map[string]interface{})
					return map[string]interface{}{
						"to":       (*user)["email"],
						"template": "welcome",
						"data": map[string]interface{}{
							"name": (*user)["name"],
						},
					}
				}
				return nil
			},
			Response:    &notifResp,
			StopOnError: false,
		},
	}

	userData := map[string]interface{}{
		"name":  "John Doe",
		"email": "john@example.com",
	}

	// Execute workflow hitting different hosts for each step
	result := client.ExecuteWorkflow(steps, userData, nil)

	// Verify workflow succeeded
	assert.True(t, result.Success, "Workflow should succeed")
	assert.Equal(t, 3, result.CompletedSteps, "All 3 steps should complete")
	assert.Equal(t, 0, result.SkippedSteps, "No steps should be skipped")

	// Verify each service was hit correctly
	assert.Equal(t, "jwt-token-123", authResp["token"])
	assert.Equal(t, "auth-service", authResp["service"])

	assert.Equal(t, float64(123), userResp["id"]) // JSON numbers become float64
	assert.Equal(t, "John Doe", userResp["name"])
	assert.Equal(t, "user-service", userResp["service"])

	assert.Equal(t, true, notifResp["sent"])
	assert.Equal(t, "john@example.com", notifResp["to"])
	assert.Equal(t, "notification-service", notifResp["service"])
}

func TestWorkflow_MixedHosts_ClientFallback(t *testing.T) {
	// Create one server that will be used as the client fallback and specific host
	primaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"service": "primary-service",
			"path":    r.URL.Path,
			"success": true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer primaryServer.Close()

	secondaryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"service": "secondary-service",
			"path":    r.URL.Path,
			"success": true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer secondaryServer.Close()

	client := NewClient()

	var step1Resp, step2Resp, step3Resp map[string]interface{}

	steps := []*WorkflowStep{
		{
			Name:     "step1",
			Endpoint: "/endpoint1",
			Method:   enums.GET,
			Host:     secondaryServer.URL, // Specific different host
			Response: &step1Resp,
		},
		{
			Name:     "step2",
			Endpoint: "/endpoint2",
			Method:   enums.GET,
			// No Host - should use client host fallback
			Response: &step2Resp,
		},
		{
			Name:     "step3",
			Endpoint: "/endpoint3",
			Method:   enums.GET,
			Host:     primaryServer.URL, // Explicit host same as client
			Response: &step3Resp,
		},
	}

	// Execute workflow with client host as fallback
	result := client.Host(primaryServer.URL).ExecuteWorkflow(steps, nil, nil)

	// Verify workflow succeeded
	assert.True(t, result.Success, "Workflow should succeed")
	assert.Equal(t, 3, result.CompletedSteps, "All 3 steps should complete")

	// Verify each step hit the correct service
	assert.Equal(t, "secondary-service", step1Resp["service"]) // Used step-specific host
	assert.Equal(t, "primary-service", step2Resp["service"])   // Used client fallback host
	assert.Equal(t, "primary-service", step3Resp["service"])   // Used step-specific host (same as client)
}

func TestWorkflow_NoHostError(t *testing.T) {
	client := NewClient()

	steps := []*WorkflowStep{
		{
			Name:     "failing_step",
			Endpoint: "/test",
			Method:   enums.GET,
			// No Host specified and no client host
			StopOnError: true,
		},
	}

	// Execute without setting any host
	result := client.ExecuteWorkflow(steps, nil, nil)

	// Should fail with clear error message
	assert.False(t, result.Success, "Workflow should fail due to missing host")
	assert.Equal(t, 0, result.CompletedSteps, "No steps should complete")
	assert.NotNil(t, result.FailedStep, "Should have failed step")

	// Check error message
	assert.Equal(t, "failing_step", result.FailedStep.Step.Name)
	assert.NotNil(t, result.FailedStep.Error)
	assert.Contains(t, result.FailedStep.Error.Message, "No host specified for step 'failing_step'")
	assert.Contains(t, result.FailedStep.Error.Message, "Provide host via step.Host or client.Host()")
}

func TestMultiEndpoints_PriorityOrder(t *testing.T) {
	// Test that target-specific host takes priority over client host
	targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ServiceResponse{Service: "target-server", Status: "ok", Host: r.Host}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer targetServer.Close()

	clientServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ServiceResponse{Service: "client-server", Status: "ok", Host: r.Host}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer clientServer.Close()

	client := NewClient()

	var resp ServiceResponse

	targets := []*MultiRequestTarget{
		{
			Endpoint: "/test",
			Method:   enums.GET,
			Host:     targetServer.URL, // Target-specific host should win
			Response: &resp,
		},
	}

	// Execute with both client host and target host specified
	result := client.Host(clientServer.URL).ExecuteMultiEndpoints(targets, false)

	// Verify target-specific host was used (not client host)
	assert.True(t, result.Success)
	assert.Equal(t, "target-server", resp.Service, "Target-specific host should take priority")
}

func TestWorkflow_PriorityOrder(t *testing.T) {
	// Test that step-specific host takes priority over client host
	stepServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{"service": "step-server", "success": true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer stepServer.Close()

	clientServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{"service": "client-server", "success": true}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer clientServer.Close()

	client := NewClient()

	var resp map[string]interface{}

	steps := []*WorkflowStep{
		{
			Name:     "test_step",
			Endpoint: "/test",
			Method:   enums.GET,
			Host:     stepServer.URL, // Step-specific host should win
			Response: &resp,
		},
	}

	// Execute with both client host and step host specified
	result := client.Host(clientServer.URL).ExecuteWorkflow(steps, nil, nil)

	// Verify step-specific host was used (not client host)
	assert.True(t, result.Success)
	assert.Equal(t, "step-server", resp["service"], "Step-specific host should take priority")
}
