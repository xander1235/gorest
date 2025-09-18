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
func (nc *NetworkClient) executeMiddlewareChain(ctx *MiddlewareContext) bool {
	if len(nc.middlewares) == 0 {
		// No middlewares configured - use original pipeline
		return nc.executeOriginalPipeline(ctx)
	}

	// Execute middleware chain
	index := 0
	var next NextFunc
	next = func() bool {
		if index >= len(nc.middlewares) {
			// End of middleware chain - execute the actual HTTP request
			return nc.executeHTTPRequestWithMiddleware(ctx)
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

// executeOriginalPipeline executes the original request pipeline without middlewares.
// This maintains backward compatibility and provides the same functionality as before
// middlewares were added, including rate limiting, circuit breaking, and retry logic.
func (nc *NetworkClient) executeOriginalPipeline(ctx *MiddlewareContext) bool {
	// Resolve configuration for this specific endpoint
	config := nc.getEndpointConfig(ctx.Endpoint)

	// Apply rate limiting if configured
	if err := nc.applyRateLimit(config, ctx.Endpoint); err != nil {
		ctx.Error = err
		return false
	}

	// Check circuit breaker status
	if err := nc.checkCircuitBreaker(config, ctx.Endpoint); err != nil {
		ctx.Error = err
		return false
	}

	// Execute request with retry logic
	method := enums.HttpMethods(ctx.Method)
	result := nc.executeWithRetries(method, ctx.Endpoint, config)

	// Record result for circuit breaker tracking
	nc.recordCircuitBreakerResult(config, result)

	// Set error in context
	ctx.Error = result
	return result == nil
}

// executeHTTPRequestWithMiddleware executes the actual HTTP request within the middleware context.
// This is called at the end of the middleware chain to perform the actual network request.
func (nc *NetworkClient) executeHTTPRequestWithMiddleware(ctx *MiddlewareContext) bool {
	// Record start time for duration calculation
	start := time.Now()

	// Execute HTTP request
	resp, err := nc.httpClient.Do(ctx.Request)
	duration := time.Since(start)
	ctx.Duration = duration

	if err != nil {
		// Network error or timeout
		ctx.Error = &errors.ErrorDetails{
			Message:      fmt.Sprintf("HTTP request failed: %s", err.Error()),
			ResponseCode: 0,
		}
		nc.logRequestError(ctx.Request, err, duration)
		return false
	}

	// Set response in context
	ctx.Response = resp

	// Process HTTP response
	processErr := nc.processHTTPResponseWithMiddleware(ctx, resp, duration)
	if processErr != nil {
		ctx.Error = processErr
		return false
	}

	return true
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
	nc.finalizeHTTPRequest(req)
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

	// Override request type with actual multipart content type (includes boundary)
	nc.requestType = contentType

	// Add headers and finalize request
	nc.finalizeHTTPRequest(req)
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
	nc.finalizeHTTPRequest(req)
	return req, nil
}

// finalizeHTTPRequest adds headers, query parameters, and context to the HTTP request.
func (nc *NetworkClient) finalizeHTTPRequest(req *http.Request) {
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
}

// === RESPONSE PROCESSING ===

// processHTTPResponseWithMiddleware handles HTTP response processing within the middleware context.
func (nc *NetworkClient) processHTTPResponseWithMiddleware(ctx *MiddlewareContext, resp *http.Response, duration time.Duration) *errors.ErrorDetails {
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		nc.logRequestError(ctx.Request, err, duration)
		return &errors.ErrorDetails{
			Message:      fmt.Sprintf("Failed to read response body: %s", err.Error()),
			ResponseCode: resp.StatusCode,
		}
	}

	bodyString := string(bodyBytes)

	// Log response details
	nc.logResponse(ctx.Request, resp, duration, len(bodyBytes))

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
