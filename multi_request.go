package gorest

import (
	"context"
	"sync"

	"github.com/xander1235/gorest/v2/constants/enums"
	"github.com/xander1235/gorest/v2/exceptions/errors"
)

// MultiRequestTarget defines a target endpoint for multi-endpoint requests
type MultiRequestTarget struct {
	// Endpoint is the URL path to send the request to
	Endpoint string

	// Method is the HTTP method to use for this endpoint
	Method enums.HttpMethods

	// Host is the optional base URL for this specific endpoint (e.g., "https://api.service1.com")
	// If empty, the client's host will be used as fallback
	Host string

	// Transform is an optional function to transform the base request body
	// for this specific endpoint. If nil, the base body will be used as-is.
	Transform func(baseBody interface{}) interface{}

	// Headers are endpoint-specific headers to add/override for this request
	Headers map[string]string

	// Params are endpoint-specific query parameters for this request
	Params map[string]string

	// Response is a pointer to struct where this endpoint's response should be stored
	Response interface{}

	// Context allows per-endpoint request context (timeout, cancellation)
	Context context.Context
}

// MultiRequestResult holds the result of a single endpoint request
type MultiRequestResult struct {
	// Target is the original target configuration used for this request
	Target *MultiRequestTarget

	// Error contains any error that occurred during the request
	Error *errors.ErrorDetails

	// Success indicates whether the request completed successfully
	Success bool

	// Index is the position of this target in the original targets slice
	Index int
}

// MultiRequestResponse contains results from all endpoint requests
type MultiRequestResponse struct {
	// Results contains the result for each endpoint request in the same order
	// as the original targets slice
	Results []*MultiRequestResult

	// SuccessCount is the number of requests that completed successfully
	SuccessCount int

	// FailureCount is the number of requests that failed
	FailureCount int

	// HasErrors indicates if any requests failed
	HasErrors bool

	// Success indicates if all requests completed successfully (no errors)
	Success bool
}

// ExecuteMultiEndpoints sends the same request body (with optional transformations)
// to multiple endpoints either sequentially or in parallel.
//
// This method is useful for scenarios like:
// - Health checks across multiple services
// - Data aggregation from multiple APIs
// - Fan-out operations to multiple endpoints
// - Broadcasting the same data to multiple services
//
// Parameters:
//   - targets: Slice of endpoint configurations to send requests to
//   - parallel: If true, requests are sent concurrently; if false, sequentially
//
// Returns:
//   - *MultiRequestResponse: Results from all endpoint requests
//
// Examples:
//
//	// Health check multiple services in parallel
//	targets := []*MultiRequestTarget{
//	    {Endpoint: "/health", Method: enums.GET},
//	    {Endpoint: "/status", Method: enums.GET},
//	}
//	result := client.Host("https://api.service1.com").ExecuteMultiEndpoints(targets, true)
//
//	// Send user data to multiple endpoints with transformations
//	userData := map[string]interface{}{"name": "John", "email": "john@example.com"}
//	targets := []*MultiRequestTarget{
//	    {
//	        Endpoint: "/users",
//	        Method: enums.POST,
//	        Transform: func(body interface{}) interface{} {
//	            // Add service-specific field
//	            data := body.(map[string]interface{})
//	            data["source"] = "api"
//	            return data
//	        },
//	    },
//	    {
//	        Endpoint: "/profiles",
//	        Method: enums.POST,
//	        // Use original body without transformation
//	    },
//	}
//	result := client.Body(userData).ExecuteMultiEndpoints(targets, false)
func (nc *NetworkClient) ExecuteMultiEndpoints(targets []*MultiRequestTarget, parallel bool) *MultiRequestResponse {
	copyClient := nc.ensureRequestCopy()

	if parallel {
		return copyClient.executeMultiEndpointsParallel(targets)
	}
	return copyClient.executeMultiEndpointsSequential(targets)
}

// executeMultiEndpointsParallel executes requests to multiple endpoints concurrently
func (nc *NetworkClient) executeMultiEndpointsParallel(targets []*MultiRequestTarget) *MultiRequestResponse {
	var wg sync.WaitGroup
	results := make([]*MultiRequestResult, len(targets))

	// Execute all requests concurrently
	for i, target := range targets {
		wg.Add(1)
		go func(index int, tgt *MultiRequestTarget) {
			defer wg.Done()
			results[index] = nc.executeSingleTarget(tgt, index)
		}(i, target)
	}

	// Wait for all requests to complete
	wg.Wait()

	return nc.buildMultiRequestResponse(results)
}

// executeMultiEndpointsSequential executes requests to multiple endpoints one by one
func (nc *NetworkClient) executeMultiEndpointsSequential(targets []*MultiRequestTarget) *MultiRequestResponse {
	results := make([]*MultiRequestResult, len(targets))

	// Execute requests sequentially
	for i, target := range targets {
		results[i] = nc.executeSingleTarget(target, i)
	}

	return nc.buildMultiRequestResponse(results)
}

// executeSingleTarget executes a request to a single target endpoint
func (nc *NetworkClient) executeSingleTarget(target *MultiRequestTarget, index int) *MultiRequestResult {
	// Create a copy for this specific target to avoid interference
	targetClient := nc.copyForRequest()

	// Set host for this target - use target-specific host or fallback to client host
	if target.Host != "" {
		targetClient.host = target.Host
	} else if targetClient.host == "" {
		// Fallback to original client's host if neither target nor client has one
		targetClient.host = nc.host
	}

	// Return error immediately if no host is available
	if targetClient.host == "" {
		return &MultiRequestResult{
			Target: target,
			Error: &errors.ErrorDetails{
				Message:      "No host specified for endpoint " + target.Endpoint + ". Provide host via target.Host or client.Host()",
				ResponseCode: 0,
			},
			Success: false,
			Index:   index,
		}
	}

	// Apply target-specific headers
	if target.Headers != nil {
		if targetClient.headers == nil {
			targetClient.headers = make(map[string]string)
		}
		for k, v := range target.Headers {
			targetClient.headers[k] = v
		}
	}

	// Apply target-specific parameters
	if target.Params != nil {
		if targetClient.params == nil {
			targetClient.params = make(map[string]string)
		}
		for k, v := range target.Params {
			targetClient.params[k] = v
		}
	}

	// Apply body transformation if provided
	if target.Transform != nil && targetClient.body != nil {
		targetClient.body = target.Transform(targetClient.body)
	}

	// Set response target if provided
	if target.Response != nil {
		targetClient.response = target.Response
	}

	// Set context if provided
	if target.Context != nil {
		targetClient.ctx = target.Context
	}

	// Execute the request
	err := targetClient.executeRequest(target.Method, target.Endpoint)

	return &MultiRequestResult{
		Target:  target,
		Error:   err,
		Success: err == nil,
		Index:   index,
	}
}

// buildMultiRequestResponse constructs the final response from individual results
func (nc *NetworkClient) buildMultiRequestResponse(results []*MultiRequestResult) *MultiRequestResponse {
	successCount := 0
	failureCount := 0
	hasErrors := false

	for _, result := range results {
		if result.Success {
			successCount++
		} else {
			failureCount++
			hasErrors = true
		}
	}

	return &MultiRequestResponse{
		Results:      results,
		SuccessCount: successCount,
		FailureCount: failureCount,
		HasErrors:    hasErrors,
		Success:      !hasErrors,
	}
}

// GetSuccessfulResults returns only the results that completed successfully
func (mr *MultiRequestResponse) GetSuccessfulResults() []*MultiRequestResult {
	var successful []*MultiRequestResult
	for _, result := range mr.Results {
		if result.Success {
			successful = append(successful, result)
		}
	}
	return successful
}

// GetFailedResults returns only the results that failed
func (mr *MultiRequestResponse) GetFailedResults() []*MultiRequestResult {
	var failed []*MultiRequestResult
	for _, result := range mr.Results {
		if !result.Success {
			failed = append(failed, result)
		}
	}
	return failed
}

// GetErrorSummary returns a summary of all errors that occurred
func (mr *MultiRequestResponse) GetErrorSummary() map[string]*errors.ErrorDetails {
	errorMap := make(map[string]*errors.ErrorDetails)
	for _, result := range mr.Results {
		if result.Error != nil {
			errorMap[result.Target.Endpoint] = result.Error
		}
	}
	return errorMap
}
