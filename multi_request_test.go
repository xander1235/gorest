package gorest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/xander1235/gorest/constants/enums"
	"github.com/xander1235/gorest/exceptions/errors"
)

// Test models for multi-endpoint tests
type TestUser struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type TestStatus struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

func TestExecuteMultiEndpoints_ParallelRequests(t *testing.T) {
	// Create test server with multiple endpoints
	requestCount := 0
	var mu sync.Mutex
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		
		switch r.URL.Path {
		case "/users":
			response := TestUser{ID: 1, Name: "John Doe", Email: "john@example.com"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		case "/status":
			response := TestStatus{Status: "ok", Message: "Service is healthy"}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
		case "/health":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient()
	
	var user TestUser
	var status TestStatus
	
	targets := []*MultiRequestTarget{
		{
			Endpoint: "/users",
			Method:   enums.GET,
			Response: &user,
		},
		{
			Endpoint: "/status",
			Method:   enums.GET,
			Response: &status,
		},
		{
			Endpoint: "/health",
			Method:   enums.GET,
		},
	}

	// Execute requests in parallel
	start := time.Now()
	result := client.Host(server.URL).ExecuteMultiEndpoints(targets, true)
	duration := time.Since(start)

	// Verify results
	assert.True(t, result.Success, "All requests should succeed")
	assert.Equal(t, 3, result.SuccessCount, "All 3 requests should succeed")
	assert.Equal(t, 0, result.FailureCount, "No requests should fail")
	assert.False(t, result.HasErrors, "Should have no errors")

	// Verify responses were populated
	assert.Equal(t, "John Doe", user.Name)
	assert.Equal(t, "ok", status.Status)

	// Verify requests were actually made
	assert.Equal(t, 3, requestCount)
	
	// Parallel requests should be faster than sequential (rough check)
	assert.Less(t, duration, 1*time.Second, "Parallel requests should complete quickly")
}

func TestExecuteMultiEndpoints_SequentialRequests(t *testing.T) {
	requestOrder := []string{}
	var mu sync.Mutex
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestOrder = append(requestOrder, r.URL.Path)
		mu.Unlock()
		
		// Add small delay to verify sequential execution
		time.Sleep(10 * time.Millisecond)
		
		switch r.URL.Path {
		case "/step1":
			w.Write([]byte(`{"step": 1}`))
		case "/step2":
			w.Write([]byte(`{"step": 2}`))
		case "/step3":
			w.Write([]byte(`{"step": 3}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewClient()
	
	targets := []*MultiRequestTarget{
		{Endpoint: "/step1", Method: enums.GET},
		{Endpoint: "/step2", Method: enums.GET},
		{Endpoint: "/step3", Method: enums.GET},
	}

	// Execute requests sequentially
	result := client.Host(server.URL).ExecuteMultiEndpoints(targets, false)

	// Verify results
	assert.True(t, result.Success)
	assert.Equal(t, 3, result.SuccessCount)

	// Verify sequential order
	mu.Lock()
	expectedOrder := []string{"/step1", "/step2", "/step3"}
	assert.Equal(t, expectedOrder, requestOrder, "Requests should be executed in order")
	mu.Unlock()
}

func TestExecuteMultiEndpoints_WithTransformations(t *testing.T) {
	receivedBodies := []string{}
	var mu sync.Mutex
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(readJSONBody(r))
		mu.Lock()
		receivedBodies = append(receivedBodies, string(body))
		mu.Unlock()
		
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	client := NewClient()
	
	baseData := map[string]interface{}{
		"name":  "John",
		"email": "john@example.com",
	}
	
	targets := []*MultiRequestTarget{
		{
			Endpoint: "/users",
			Method:   enums.POST,
			Transform: func(body interface{}) interface{} {
				data := copyMap(body.(map[string]interface{}))
				data["source"] = "api"
				data["type"] = "user"
				return data
			},
		},
		{
			Endpoint: "/profiles",
			Method:   enums.POST,
			Transform: func(body interface{}) interface{} {
				data := copyMap(body.(map[string]interface{}))
				data["source"] = "profile"
				data["verified"] = true
				return data
			},
		},
		{
			Endpoint: "/contacts",
			Method:   enums.POST,
			// No transformation - use original body
		},
	}

	result := client.Body(baseData).Host(server.URL).ExecuteMultiEndpoints(targets, false)

	// Verify all requests succeeded
	assert.True(t, result.Success)
	assert.Equal(t, 3, result.SuccessCount)

	// Verify transformations were applied
	mu.Lock()
	assert.Len(t, receivedBodies, 3)
	
	// Check first transformation (users endpoint)
	var userData map[string]interface{}
	json.Unmarshal([]byte(receivedBodies[0]), &userData)
	assert.Equal(t, "api", userData["source"])
	assert.Equal(t, "user", userData["type"])
	assert.Equal(t, "John", userData["name"])
	
	// Check second transformation (profiles endpoint)
	var profileData map[string]interface{}
	json.Unmarshal([]byte(receivedBodies[1]), &profileData)
	assert.Equal(t, "profile", profileData["source"])
	assert.Equal(t, true, profileData["verified"])
	assert.Equal(t, "John", profileData["name"])
	
	// Check third request (no transformation)
	var contactData map[string]interface{}
	json.Unmarshal([]byte(receivedBodies[2]), &contactData)
	assert.Equal(t, "John", contactData["name"])
	assert.Nil(t, contactData["source"]) // Should not have transformation fields
	mu.Unlock()
}

func TestExecuteMultiEndpoints_WithFailures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/success":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok"}`))
		case "/failure":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "internal error"}`))
		case "/notfound":
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "not found"}`))
		}
	}))
	defer server.Close()

	client := NewClient()
	
	targets := []*MultiRequestTarget{
		{Endpoint: "/success", Method: enums.GET},
		{Endpoint: "/failure", Method: enums.GET},
		{Endpoint: "/notfound", Method: enums.GET},
	}

	result := client.Host(server.URL).ExecuteMultiEndpoints(targets, true)

	// Verify mixed results
	assert.False(t, result.Success, "Should fail due to errors")
	assert.Equal(t, 1, result.SuccessCount, "One request should succeed")
	assert.Equal(t, 2, result.FailureCount, "Two requests should fail")
	assert.True(t, result.HasErrors, "Should have errors")

	// Check individual results
	successResults := result.GetSuccessfulResults()
	failedResults := result.GetFailedResults()
	
	assert.Len(t, successResults, 1)
	assert.Equal(t, "/success", successResults[0].Target.Endpoint)
	
	assert.Len(t, failedResults, 2)
	
	errorSummary := result.GetErrorSummary()
	assert.Len(t, errorSummary, 2)
	assert.Contains(t, errorSummary, "/failure")
	assert.Contains(t, errorSummary, "/notfound")
}

func TestExecuteMultiEndpoints_WithCustomHeaders(t *testing.T) {
	receivedHeaders := make(map[string]map[string]string)
	var mu sync.Mutex
	
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers := make(map[string]string)
		for name, values := range r.Header {
			if len(values) > 0 {
				headers[name] = values[0]
			}
		}
		
		mu.Lock()
		receivedHeaders[r.URL.Path] = headers
		mu.Unlock()
		
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	client := NewClient()
	
	targets := []*MultiRequestTarget{
		{
			Endpoint: "/service1",
			Method:   enums.GET,
			Headers: map[string]string{
				"X-Service": "service1",
				"X-Version": "v1",
			},
		},
		{
			Endpoint: "/service2",
			Method:   enums.GET,
			Headers: map[string]string{
				"X-Service": "service2",
				"X-Version": "v2",
			},
		},
	}

	result := client.
		Headers(map[string]string{"X-Global": "global"}).
		Host(server.URL).
		ExecuteMultiEndpoints(targets, true)

	assert.True(t, result.Success)
	
	// Verify headers were applied correctly
	mu.Lock()
	service1Headers := receivedHeaders["/service1"]
	service2Headers := receivedHeaders["/service2"]
	
	// Check service1 headers
	assert.Equal(t, "service1", service1Headers["X-Service"])
	assert.Equal(t, "v1", service1Headers["X-Version"])
	assert.Equal(t, "global", service1Headers["X-Global"])
	
	// Check service2 headers
	assert.Equal(t, "service2", service2Headers["X-Service"])
	assert.Equal(t, "v2", service2Headers["X-Version"])
	assert.Equal(t, "global", service2Headers["X-Global"])
	mu.Unlock()
}

func TestExecuteMultiEndpoints_WithContextTimeout(t *testing.T) {
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
	
	targets := []*MultiRequestTarget{
		{
			Endpoint: "/fast",
			Method:   enums.GET,
		},
		{
			Endpoint: "/slow",
			Method:   enums.GET,
			Context:  ctx, // This should timeout
		},
	}

	result := client.Host(server.URL).ExecuteMultiEndpoints(targets, true)

	// Should have mixed results - one success, one timeout
	assert.False(t, result.Success)
	assert.Equal(t, 1, result.SuccessCount)
	assert.Equal(t, 1, result.FailureCount)
	
	failedResults := result.GetFailedResults()
	assert.Len(t, failedResults, 1)
	assert.Equal(t, "/slow", failedResults[0].Target.Endpoint)
}

// Helper functions
func readJSONBody(r *http.Request) interface{} {
	var body interface{}
	if r.Body != nil {
		decoder := json.NewDecoder(r.Body)
		decoder.Decode(&body)
	}
	return body
}

func copyMap(original map[string]interface{}) map[string]interface{} {
	copy := make(map[string]interface{})
	for key, value := range original {
		copy[key] = value
	}
	return copy
}

func TestMultiRequestResponse_HelperMethods(t *testing.T) {
	// Create mock results
	successResult := &MultiRequestResult{
		Target:  &MultiRequestTarget{Endpoint: "/success"},
		Success: true,
		Error:   nil,
	}
	
	failureResult := &MultiRequestResult{
		Target:  &MultiRequestTarget{Endpoint: "/failure"},
		Success: false,
		Error:   &errors.ErrorDetails{Message: "Failed"},
	}
	
	response := &MultiRequestResponse{
		Results:      []*MultiRequestResult{successResult, failureResult},
		SuccessCount: 1,
		FailureCount: 1,
		HasErrors:    true,
	}
	
	// Test GetSuccessfulResults
	successResults := response.GetSuccessfulResults()
	assert.Len(t, successResults, 1)
	assert.Equal(t, "/success", successResults[0].Target.Endpoint)
	
	// Test GetFailedResults
	failedResults := response.GetFailedResults()
	assert.Len(t, failedResults, 1)
	assert.Equal(t, "/failure", failedResults[0].Target.Endpoint)
	
	// Test GetErrorSummary
	errorSummary := response.GetErrorSummary()
	assert.Len(t, errorSummary, 1)
	assert.Contains(t, errorSummary, "/failure")
	assert.Equal(t, "Failed", errorSummary["/failure"].Message)
}

// Benchmark tests for multi-endpoint requests
func BenchmarkExecuteMultiEndpoints_Parallel(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	client := NewClient()
	
	targets := []*MultiRequestTarget{
		{Endpoint: "/endpoint1", Method: enums.GET},
		{Endpoint: "/endpoint2", Method: enums.GET},
		{Endpoint: "/endpoint3", Method: enums.GET},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := client.Host(server.URL).ExecuteMultiEndpoints(targets, true)
		if !result.Success {
			b.Fatal("Request failed")
		}
	}
}

func BenchmarkExecuteMultiEndpoints_Sequential(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}))
	defer server.Close()

	client := NewClient()
	
	targets := []*MultiRequestTarget{
		{Endpoint: "/endpoint1", Method: enums.GET},
		{Endpoint: "/endpoint2", Method: enums.GET},
		{Endpoint: "/endpoint3", Method: enums.GET},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		result := client.Host(server.URL).ExecuteMultiEndpoints(targets, false)
		if !result.Success {
			b.Fatal("Request failed")
		}
	}
}
