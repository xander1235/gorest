package gorest

import (
	"bytes"
	"encoding/json"
	inbuiltErr "errors"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/xander1235/gorest/constants"
	"github.com/xander1235/gorest/constants/enums"
	"github.com/xander1235/gorest/exceptions"
	"github.com/xander1235/gorest/exceptions/errors"
	"io"
)

// === MIDDLEWARE CHAIN EXECUTION ===

// executeMiddlewareChain executes the middleware chain for the request.
// If no middlewares are configured, it falls back to the original request pipeline
// with rate limiting, circuit breaking, and retry logic.
//
// The middleware chain is executed in the order middlewares were added to the client.
// Each middleware can modify the request, execute the next middleware, and process the response.
//
// Parameters:
//   - ctx: MiddlewareContext containing request/response data
//
// Returns:
//   - bool: true if request succeeded, false if it failed or was aborted
func (nc *NetworkClient) executeMiddlewareChain(ctx *MiddlewareContext, isStreaming bool) bool {
	if len(nc.middlewares) == 0 {
		// No middlewares configured - use original pipeline
		return nc.executePipeline(ctx, false, isStreaming)
	}

	// Execute middleware chain
	index := 0
	var next NextFunc
	next = func() bool {
		if index >= len(nc.middlewares) {
			// End of middleware chain - execute the actual HTTP request
			return nc.executePipeline(ctx, true, isStreaming)
		}

		// Get current middleware and increment index
		middleware := nc.middlewares[index]
		index++

		// Execute current middleware
		middleware(ctx, next)

		// Return success status
		return ctx.Error == nil
	}

	// Start the middleware chain
	return next()
}

// executePipeline handles both original and middleware execution paths
// with consistent rate limiting, circuit breaking, and retry logic.
//
// Parameters:
//   - ctx: MiddlewareContext containing request/response data
//   - useMiddleware: if true, uses ctx.Request; if false, builds request on demand
//
// Returns:
//   - bool: true if request succeeded, false if it failed or was aborted
func (nc *NetworkClient) executePipeline(ctx *MiddlewareContext, useMiddleware bool, isStreaming bool) bool {
	// Resolve configuration for this specific endpoint
	config := nc.getEndpointConfig(ctx.Endpoint)

	// Apply rate limiting if configured - once per logical request
	if err := nc.applyRateLimit(config, ctx.Endpoint); err != nil {
		ctx.Error = err
		return false
	}

	// Check circuit breaker status - once per logical request
	if err := nc.checkCircuitBreaker(config, ctx.Endpoint); err != nil {
		ctx.Error = err
		return false
	}

	result := nc.executeWithRetries(ctx, config, useMiddleware, isStreaming)

	// Record result for circuit breaker tracking - once per logical request
	nc.recordCircuitBreakerResult(config, result)

	// Set error in context and return status
	ctx.Error = result
	return result == nil
}

// executeSingleAttempt is a helper method that performs a single HTTP request attempt
// for the original (non-middleware) execution path.
// It would replace the single-attempt logic previously in executeWithRetries.
func (nc *NetworkClient) executeSingleAttempt(ctx *MiddlewareContext, isStreaming bool) *errors.ErrorDetails {
	// Build request for original pipeline
	req, buildErr := nc.buildHTTPRequest(enums.HttpMethods(ctx.Method), ctx.Endpoint)
	if buildErr != nil {
		return buildErr
	}

	// Execute request
	start := time.Now()
	resp, err := nc.httpClient.Do(req)
	duration := time.Since(start)

	ctx.Response = resp

	if err != nil {
		return &errors.ErrorDetails{
			Message:      fmt.Sprintf("HTTP request failed: %s", err.Error()),
			ResponseCode: 0,
		}
	}

	if !isStreaming {
		defer resp.Body.Close()
	}

	// Process response using shared logic
	return nc.processHTTPResponseWithMiddleware(ctx, resp, duration, isStreaming)
}

// executeHTTPRequestWithMiddleware executes the actual HTTP request within the middleware context.
// This is called at the end of the middleware chain to perform the actual network request.
func (nc *NetworkClient) executeHTTPRequestWithMiddleware(ctx *MiddlewareContext, isStreaming bool) bool {
	// Single-attempt execution using helper
	if err := nc.attemptHTTPRequestWithMiddleware(ctx, isStreaming); err != nil {
		ctx.Error = err
		return false
	}
	return true
}

// attemptHTTPRequestWithMiddleware performs a single HTTP attempt using ctx.Request
// and returns an error if the attempt fails or the response indicates an error.
func (nc *NetworkClient) attemptHTTPRequestWithMiddleware(ctx *MiddlewareContext, isStreaming bool) *errors.ErrorDetails {
	start := time.Now()
	resp, err := nc.httpClient.Do(ctx.Request)
	duration := time.Since(start)
	ctx.Duration = duration

	if err != nil {
		return &errors.ErrorDetails{
			Message:      fmt.Sprintf("HTTP request failed: %s", err.Error()),
			ResponseCode: 0,
		}
	}

	// Set response in context for processing
	ctx.Response = resp

	if processErr := nc.processHTTPResponseWithMiddleware(ctx, resp, duration, isStreaming); processErr != nil {
		return processErr
	}
	return nil
}

// === HTTP REQUEST BUILDING ===

// buildHTTPRequest builds the HTTP request from the client configuration.
// This is extracted from the original request building logic to work with middlewares.
func (nc *NetworkClient) buildHTTPRequest(method enums.HttpMethods, endpoint string) (*http.Request, *errors.ErrorDetails) {
	// Delegate to content-type-specific request builders
	switch nc.requestType {
	case enums.Json.ToString():
		return nc.buildJSONRequest(method, endpoint)
	case enums.Multipart.ToString():
		return nc.buildMultipartRequest(method, endpoint)
	case enums.FormUrlEncoded.ToString():
		return nc.buildFormRequest(method, endpoint)
	case "text/event-stream":
		return nc.buildSSERequest(method, endpoint)
	default:
		return nil, &errors.ErrorDetails{
			Message:      fmt.Sprintf("Unsupported request type: %s", nc.requestType),
			ResponseCode: 500,
		}
	}
}

// buildJSONRequest builds an HTTP request with JSON body encoding.
func (nc *NetworkClient) buildJSONRequest(method enums.HttpMethods, endpoint string) (*http.Request, *errors.ErrorDetails) {
	var bodyBuffer *bytes.Buffer

	if nc.body != nil {
		// Encode request body as JSON
		jsonBytes, err := json.Marshal(nc.body)
		if err != nil {
			return nil, &errors.ErrorDetails{
				Message:      fmt.Sprintf("Failed to encode request body as JSON: %s", err.Error()),
				ResponseCode: 500,
			}
		}
		bodyBuffer = bytes.NewBuffer(jsonBytes)
	} else {
		bodyBuffer = bytes.NewBuffer(nil)
	}

	// Create HTTP request
	req, err := http.NewRequest(method.String(), nc.host+endpoint, bodyBuffer)
	if err != nil {
		return nil, &errors.ErrorDetails{
			Message:      fmt.Sprintf("Failed to create HTTP request: %s", err.Error()),
			ResponseCode: 500,
		}
	}

	// Add headers and finalize request
	req = nc.finalizeHTTPRequest(req)
	return req, nil
}

// buildMultipartRequest builds an HTTP request with multipart body encoding.
func (nc *NetworkClient) buildMultipartRequest(method enums.HttpMethods, endpoint string) (*http.Request, *errors.ErrorDetails) {
	var bodyBuffer *bytes.Buffer
	var contentType string

	if nc.multipart != nil {
		// Create multipart body
		buffer, ct, err := nc.multipart.CreateBuffer()
		if err != nil {
			return nil, &errors.ErrorDetails{
				Message:      fmt.Sprintf("Failed to create multipart body: %s", err.Error()),
				ResponseCode: 500,
			}
		}
		bodyBuffer = buffer
		contentType = ct
	} else {
		bodyBuffer = bytes.NewBuffer(nil)
		contentType = enums.Multipart.ToString()
	}

	// Create HTTP request
	req, err := http.NewRequest(method.String(), nc.host+endpoint, bodyBuffer)
	if err != nil {
		return nil, &errors.ErrorDetails{
			Message:      fmt.Sprintf("Failed to create HTTP request: %s", err.Error()),
			ResponseCode: 500,
		}
	}

	// Add headers and finalize request (this sets Content-Type from nc.requestType)
	req = nc.finalizeHTTPRequest(req)
	
	// Override Content-Type header with actual multipart content type (includes boundary)
	// Do this AFTER finalization to ensure the correct multipart content type is used
	req.Header.Set("Content-Type", contentType)
	
	return req, nil
}

// buildFormRequest builds an HTTP request with form URL-encoded body.
func (nc *NetworkClient) buildFormRequest(method enums.HttpMethods, endpoint string) (*http.Request, *errors.ErrorDetails) {
	var bodyBuffer *bytes.Buffer

	if nc.body != nil {
		// Convert body to form values
		form := url.Values{}
		if formData, ok := nc.body.(map[string]string); ok {
			for key, value := range formData {
				form.Set(key, value)
			}
		} else {
			return nil, &errors.ErrorDetails{
				Message:      "Form URL-encoded body must be map[string]string",
				ResponseCode: 400,
			}
		}

		// Encode form data
		encodedData := form.Encode()
		bodyBuffer = bytes.NewBufferString(encodedData)
	} else {
		bodyBuffer = bytes.NewBuffer(nil)
	}

	// Create HTTP request
	req, err := http.NewRequest(method.String(), nc.host+endpoint, bodyBuffer)
	if err != nil {
		return nil, &errors.ErrorDetails{
			Message:      fmt.Sprintf("Failed to create HTTP request: %s", err.Error()),
			ResponseCode: 500,
		}
	}

	// Add headers and finalize request
	req = nc.finalizeHTTPRequest(req)
	return req, nil
}

// buildSSERequest builds an HTTP request for Server-Sent Events streaming.
func (nc *NetworkClient) buildSSERequest(method enums.HttpMethods, endpoint string) (*http.Request, *errors.ErrorDetails) {
	var bodyBuffer *bytes.Buffer

	if nc.body != nil {
		// Serialize body as JSON for SSE POST requests
		jsonBody, err := json.Marshal(nc.body)
		if err != nil {
			return nil, &errors.ErrorDetails{
				Message:      fmt.Sprintf("Failed to serialize SSE request body: %s", err.Error()),
				ResponseCode: 500,
			}
		}
		bodyBuffer = bytes.NewBuffer(jsonBody)
	} else {
		bodyBuffer = bytes.NewBuffer(nil)
	}

	// Create HTTP request
	req, err := http.NewRequest(method.String(), nc.host+endpoint, bodyBuffer)
	if err != nil {
		return nil, &errors.ErrorDetails{
			Message:      fmt.Sprintf("Failed to create SSE HTTP request: %s", err.Error()),
			ResponseCode: 500,
		}
	}

	// Add headers and finalize request
	req = nc.finalizeHTTPRequest(req)
	// Set SSE-specific headers
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	// If using HTTP/1.1, ensure keep-alive
	if req.ProtoMajor == 1 && req.ProtoMinor == 1 {
		req.Header.Set("Connection", "keep-alive")
	}

	// Override Content-Type for SSE requests AFTER finalization to avoid being overwritten
	if nc.body != nil {
		// For POST streaming with body, use JSON content type
		req.Header.Set("Content-Type", "application/json")
	} else {
		// For GET streaming, remove content type
		req.Header.Del("Content-Type")
	}
	return req, nil
}

// finalizeHTTPRequest adds headers, query parameters, and context to the HTTP request.
func (nc *NetworkClient) finalizeHTTPRequest(req *http.Request) *http.Request {
	// Set Content-Type header
	req.Header.Set(constants.ContentType, nc.requestType)

	// Set unique request ID for tracing
	req.Header.Set(constants.XRequestId, uuid.New().String())

	// Add default headers first
	for key, value := range nc.defaultHeaders {
		req.Header.Set(key, value)
	}

	// Add request-specific headers (override defaults)
	for key, value := range nc.headers {
		req.Header.Set(key, value)
	}

	// Add query parameters
	if len(nc.params) > 0 {
		q := req.URL.Query()
		for key, value := range nc.params {
			q.Add(key, value)
		}
		req.URL.RawQuery = q.Encode()
	}

	// Set request context if provided
	if nc.ctx != nil {
		req = req.WithContext(nc.ctx)
	}
	return req
}

// === RESPONSE PROCESSING ===

// processHTTPResponseWithMiddleware handles HTTP response processing within the middleware context.
func (nc *NetworkClient) processHTTPResponseWithMiddleware(ctx *MiddlewareContext, resp *http.Response, duration time.Duration, isStreaming bool) *errors.ErrorDetails {
	if isStreaming {
		return nil
	}

	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		// TODO: Add proper request error logging
		return &errors.ErrorDetails{
			Message:      fmt.Sprintf("Failed to read response body: %s", err.Error()),
			ResponseCode: resp.StatusCode,
		}
	}

	bodyString := string(bodyBytes)

	// Log response details
	// TODO: Add proper response logging

	// Process response based on status code
	switch enums.HttpStatus(resp.StatusCode).SeriesType() {
	case enums.Successful:
		// 2xx Success - parse response into provided struct
		if nc.response != nil {
			if parseErr := nc.parser(bodyString, nc.response); parseErr != nil {
				return parseErr
			}
		}
		return nil

	case enums.ClientError:
		// 4xx Client Error - parse error details
		errorDetails := nc.errorParser(bodyString)
		return exceptions.GenericException(
			errorDetails.Message,
			errorDetails.Error,
			resp.StatusCode,
		)

	case enums.ServerError:
		// 5xx Server Error - generic server error handling
		return exceptions.GenericException(
			constants.SomethingWentWrong,
			inbuiltErr.New(bodyString),
			resp.StatusCode,
		)

	default:
		// Unexpected status code
		return exceptions.GenericException(
			fmt.Sprintf("Unexpected HTTP status code: %d", resp.StatusCode),
			inbuiltErr.New(bodyString),
			resp.StatusCode,
		)
	}
}

// === HELPER FUNCTIONS ===

// logRequestError logs HTTP request errors for debugging and monitoring.
//func (nc *NetworkClient) logRequestError(req *http.Request, err error, duration time.Duration) {
//	if nc.logger == nil {
//		return
//	}
//
//	nc.logger.Error("HTTP request failed",
//		zap.String("method", req.Method),
//		zap.String("url", req.URL.String()),
//		zap.Error(err),
//		zap.Duration("duration", duration),
//		zap.String("request_id", req.Header.Get(constants.XRequestId)),
//	)
//}
