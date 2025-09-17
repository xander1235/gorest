package gorest

import (
	"bytes"
	"context"
	"encoding/json"
	inbuiltErr "errors"
	"fmt"
	"go.uber.org/zap"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/xander1235/gorest/constants"
	"github.com/xander1235/gorest/constants/enums"
	"github.com/xander1235/gorest/exceptions"
	"github.com/xander1235/gorest/exceptions/errors"
)

// executeRequest is the main request execution pipeline that coordinates all
// advanced networking features including rate limiting, circuit breaking,
// retry logic, and request/response processing.
//
// Execution pipeline:
//  1. Resolve endpoint-specific configuration
//  2. Apply rate limiting (if configured)
//  3. Check circuit breaker status
//  4. Execute HTTP request with retries
//  5. Record results for circuit breaker
//  6. Process response or error
//
// Parameters:
//   - method: HTTP method to execute (GET, POST, PUT, etc.)
//   - endpoint: URL path to append to configured host
//
// Returns:
//   - *errors.ErrorDetails: Error information if request failed, nil on success
//
// Thread safety:
//
//	This method is safe to call concurrently as it operates on request-specific
//	configuration while coordinating with shared infrastructure safely.
func (nc *NetworkClient) executeRequest(method enums.HttpMethods, endpoint string) *errors.ErrorDetails {
	// Resolve configuration for this specific endpoint
	config := nc.getEndpointConfig(endpoint)

	// Apply rate limiting if configured
	if err := nc.applyRateLimit(config, endpoint); err != nil {
		return err
	}

	// Check circuit breaker status
	if err := nc.checkCircuitBreaker(config, endpoint); err != nil {
		return err
	}

	// Execute request with retry logic
	result := nc.executeWithRetries(method, endpoint, config)

	// Record result for circuit breaker tracking
	nc.recordCircuitBreakerResult(config, result)

	return result
}

// applyRateLimit enforces rate limiting constraints before allowing request execution.
// Uses token bucket algorithm to provide smooth rate limiting with burst capacity.
//
// Rate limiting benefits:
//   - Prevents overwhelming downstream services
//   - Helps stay within API rate limits and quotas
//   - Provides backpressure when services are under load
//   - Enables fair resource sharing across concurrent requests
//
// Parameters:
//   - config: Endpoint configuration containing rate limiter
//   - endpoint: Endpoint path for logging purposes
//
// Returns:
//   - *errors.ErrorDetails: Rate limit error if request should be throttled
//
// Behavior:
//   - If rate limiter is nil, no rate limiting is applied
//   - Blocks until a token is available or context is cancelled
//   - Uses request context for timeout and cancellation
func (nc *NetworkClient) applyRateLimit(config *EndpointConfig, endpoint string) *errors.ErrorDetails {
	if config.rateLimiter == nil {
		return nil // No rate limiting configured
	}

	// Use request context or background if none provided
	ctx := nc.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	// Wait for rate limiter token
	start := time.Now()
	err := config.rateLimiter.Wait(ctx)
	if err != nil {
		// Log rate limit event for monitoring
		if nc.logger != nil {
			nc.logger.Warn("Request rate limited",
				zap.String("endpoint", endpoint),
				zap.String("error", err.Error()),
			)
		}
		return exceptions.GenericException(
			fmt.Sprintf("Rate limit exceeded for endpoint %s: %s", endpoint, err.Error()),
			err,
			429, // Too Many Requests
		)
	}

	// Log rate limiting delay for performance monitoring
	delay := time.Since(start)
	if delay > 10*time.Millisecond && nc.logger != nil {
		nc.logger.Debug("Request delayed by rate limiter",
			zap.String("endpoint", endpoint),
			zap.Duration("delay", delay),
		)
	}

	return nil
}

// checkCircuitBreaker evaluates circuit breaker state before allowing request execution.
// Circuit breakers provide fast failure when downstream services are unhealthy,
// preventing cascade failures and resource exhaustion.
//
// Circuit breaker states:
//   - CLOSED: Normal operation, requests allowed
//   - OPEN: Service unhealthy, requests blocked with fast failure
//   - HALF_OPEN: Testing service recovery, limited requests allowed
//
// Parameters:
//   - config: Endpoint configuration containing circuit breaker
//   - endpoint: Endpoint path for logging purposes
//
// Returns:
//   - *errors.ErrorDetails: Circuit breaker error if request should be blocked
//
// Benefits:
//   - Prevents wasting resources on requests likely to fail
//   - Gives failing services time to recover
//   - Provides immediate failure response instead of timeout delays
//   - Automatically tests for service recovery
func (nc *NetworkClient) checkCircuitBreaker(config *EndpointConfig, endpoint string) *errors.ErrorDetails {
	if config.circuitBreaker == nil {
		return nil // No circuit breaker configured
	}

	if !config.circuitBreaker.AllowRequest() {
		// Log circuit breaker activation for monitoring
		if nc.logger != nil {
			nc.logger.Warn("Circuit breaker open - blocking request",
				zap.String("endpoint", endpoint),
				zap.String("state", string(config.circuitBreaker.GetState())),
				zap.Int("failures", config.circuitBreaker.GetFailureCount()),
			)
		}
		return exceptions.GenericException(
			fmt.Sprintf("Circuit breaker open for endpoint %s - service may be degraded", endpoint),
			nil,
			503, // Service Unavailable
		)
	}

	return nil
}

// executeWithRetries implements the retry logic with exponential backoff.
// Automatically retries failed requests based on configured retry policies,
// improving reliability for transient failures.
//
// Retry strategy:
//   - Exponential backoff: Delays increase exponentially (100ms, 200ms, 400ms, ...)
//   - Jitter: Random variation to prevent thundering herd
//   - Conditional retries: Only retry on specific error types/status codes
//   - Maximum attempts: Prevent infinite retry loops
//
// Parameters:
//   - method: HTTP method to execute
//   - endpoint: URL path for the request
//   - config: Endpoint configuration with retry settings
//
// Returns:
//   - *errors.ErrorDetails: Final error after all retry attempts, nil on success
//
// Retryable conditions:
//   - Network timeouts and connection failures
//   - HTTP 5xx server errors (internal server error, service unavailable)
//   - HTTP 429 rate limiting responses
//   - DNS resolution failures
func (nc *NetworkClient) executeWithRetries(method enums.HttpMethods, endpoint string, config *EndpointConfig) *errors.ErrorDetails {
	retryConfig := config.retryConfig
	if retryConfig == nil {
		// No retry configuration - execute once
		return nc.executeHTTPRequest(method, endpoint)
	}

	var lastError *errors.ErrorDetails
	maxAttempts := retryConfig.MaxRetries + 1 // +1 for initial attempt

	for attempt := 0; attempt < maxAttempts; attempt++ {
		// Execute the HTTP request
		err := nc.executeHTTPRequest(method, endpoint)
		if err == nil {
			// Success - log retry recovery if this wasn't the first attempt
			if attempt > 0 && nc.logger != nil {
				nc.logger.Info("Request succeeded after retries",
					zap.String("method", method.String()),
					zap.String("endpoint", endpoint),
					zap.Int("attempt", attempt+1),
				)
			}
			return nil
		}

		lastError = err

		// Check if this error type should be retried
		if !nc.shouldRetry(err, retryConfig) {
			// Non-retryable error - fail immediately
			if nc.logger != nil {
				nc.logger.Debug("Error not retryable - failing immediately",
					zap.String("endpoint", endpoint),
					zap.Int("status_code", err.ResponseCode),
					zap.String("error", err.Message),
				)
			}
			break
		}

		// Don't delay after the last attempt
		if attempt < maxAttempts-1 {
			// Calculate retry delay with exponential backoff
			delay := nc.calculateRetryDelay(attempt, retryConfig)

			// Log retry attempt for monitoring
			if nc.logger != nil {
				nc.logger.Warn("Request failed - retrying",
					zap.String("method", method.String()),
					zap.String("endpoint", endpoint),
					zap.Int("attempt", attempt+1),
					zap.Duration("retry_delay", delay),
					zap.Int("status_code", err.ResponseCode),
					zap.String("error", err.Message),
				)
			}

			// Wait before retry (respecting context cancellation)
			ctx := nc.ctx
			if ctx == nil {
				ctx = context.Background()
			}

			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
				// Delay completed - proceed with retry
			case <-ctx.Done():
				// Context cancelled - stop retrying
				timer.Stop()
				return exceptions.GenericException(
					fmt.Sprintf("Request cancelled during retry delay: %s", ctx.Err()),
					ctx.Err(),
					0, // No HTTP status for cancelled requests
				)
			}
		}
	}

	// All retry attempts exhausted - return the last error
	if nc.logger != nil {
		nc.logger.Error("Request failed after all retry attempts",
			zap.String("method", method.String()),
			zap.String("endpoint", endpoint),
			zap.Int("attempts", maxAttempts),
			zap.Int("final_status_code", lastError.ResponseCode),
			zap.String("final_error", lastError.Message),
		)
	}

	return lastError
}

// shouldRetry determines whether a failed request should be retried based on
// the error type, HTTP status code, and retry configuration.
//
// Default retryable conditions:
//   - HTTP 429 (Too Many Requests) - rate limiting
//   - HTTP 5xx (Server Error) - internal server error, service unavailable, etc.
//   - Network errors - timeouts, connection failures, DNS errors
//
// Non-retryable conditions:
//   - HTTP 4xx (Client Error) except 429 - bad request, unauthorized, not found
//   - HTTP 2xx/3xx (Success/Redirect) - shouldn't be errors
//   - Application-specific errors based on configuration
//
// Parameters:
//   - err: The error details from the failed request
//   - config: Retry configuration with specific retry conditions
//
// Returns:
//   - bool: true if the request should be retried, false otherwise
func (nc *NetworkClient) shouldRetry(err *errors.ErrorDetails, config *RetryConfig) bool {
	if err == nil {
		return false // No error - shouldn't be retrying
	}

	// Check configured retryable status codes
	if len(config.RetryableStatusCodes) > 0 {
		for _, code := range config.RetryableStatusCodes {
			if err.ResponseCode == code {
				return true
			}
		}
		return false // Status code not in retryable list
	}

	// Default retryable status codes if none configured
	switch err.ResponseCode {
	case 429: // Too Many Requests - rate limiting
		return true
	case 500, 502, 503, 504: // Server errors
		return true
	case 0: // Network errors (no HTTP status)
		return true
	default:
		return false
	}
}

// calculateRetryDelay computes the delay before the next retry attempt using
// exponential backoff with optional jitter to prevent thundering herd effects.
//
// Exponential backoff formula:
//
//	delay = min(BaseDelay * (2 ^ attempt), MaxDelay)
//
// Benefits:
//   - Starts with short delays for quick recovery from brief issues
//   - Increases delays to avoid overwhelming struggling services
//   - Caps maximum delay to prevent excessive wait times
//   - Jitter prevents multiple clients from retrying simultaneously
//
// Parameters:
//   - attempt: Current retry attempt number (0-based)
//   - config: Retry configuration with delay parameters
//
// Returns:
//   - time.Duration: Calculated delay before next retry attempt
func (nc *NetworkClient) calculateRetryDelay(attempt int, config *RetryConfig) time.Duration {
	if config.BaseDelay <= 0 {
		return time.Second // Fallback to 1 second if no base delay configured
	}

	// Exponential backoff: baseDelay * (2 ^ attempt)
	// Use float64 to handle large exponents without overflow
	multiplier := math.Pow(2, float64(attempt))
	delay := time.Duration(float64(config.BaseDelay) * multiplier)

	// Cap at maximum delay if configured
	if config.MaxDelay > 0 && delay > config.MaxDelay {
		delay = config.MaxDelay
	}

	// TODO: Add jitter in future enhancement to prevent thundering herd
	// jitter := time.Duration(rand.Int63n(int64(delay * 0.1))) // 10% jitter
	// delay += jitter

	return delay
}

// recordCircuitBreakerResult updates the circuit breaker with request outcome
// to track service health and trigger state transitions when necessary.
//
// Result classification:
//   - Success: HTTP 2xx status codes and no network errors
//   - Failure: HTTP 5xx status codes, timeouts, connection errors
//   - Neutral: HTTP 4xx client errors (don't indicate service health issues)
//
// Parameters:
//   - config: Endpoint configuration containing circuit breaker
//   - result: Request result to record (nil for success, error details for failure)
//
// Circuit breaker state transitions:
//   - Success in any state → CLOSED (service healthy)
//   - Failure in CLOSED → may trigger OPEN if threshold exceeded
//   - Failure in HALF_OPEN → immediately OPEN (service still unhealthy)
func (nc *NetworkClient) recordCircuitBreakerResult(config *EndpointConfig, result *errors.ErrorDetails) {
	if config.circuitBreaker == nil {
		return // No circuit breaker configured
	}

	if result == nil {
		// Success - record positive result
		config.circuitBreaker.RecordSuccess()
		return
	}

	// Determine if this error should count as a circuit breaker failure
	if nc.isCircuitBreakerFailure(result) {
		config.circuitBreaker.RecordFailure()
		if nc.logger != nil {
			nc.logger.Debug("Circuit breaker failure recorded",
				zap.Int("status_code", result.ResponseCode),
				zap.String("state", string(config.circuitBreaker.GetState())),
				zap.Int("failure_count", config.circuitBreaker.GetFailureCount()),
			)
		}
	} else {
		// Client error or other non-service-health-related error
		// Don't record as failure since it doesn't indicate service degradation
		if nc.logger != nil {
			nc.logger.Debug("Error not counted as circuit breaker failure",
				zap.Int("status_code", result.ResponseCode),
				zap.String("error", result.Message),
			)
		}
	}
}

// isCircuitBreakerFailure determines whether an error should count as a failure
// for circuit breaker purposes. Only errors that indicate service health issues
// should contribute to circuit breaker state changes.
//
// Circuit breaker failures (indicate service issues):
//   - HTTP 5xx server errors (internal server error, service unavailable)
//   - Network errors (timeouts, connection failures, DNS errors)
//   - Request cancellations due to timeouts
//
// Not circuit breaker failures (client issues):
//   - HTTP 4xx client errors (bad request, unauthorized, not found)
//   - Validation errors and malformed requests
//   - Authentication and authorization failures
//
// Parameters:
//   - err: Error details from the failed request
//
// Returns:
//   - bool: true if error should count as circuit breaker failure
func (nc *NetworkClient) isCircuitBreakerFailure(err *errors.ErrorDetails) bool {
	if err == nil {
		return false
	}

	// HTTP 5xx errors indicate server/service issues
	if err.ResponseCode >= 500 && err.ResponseCode < 600 {
		return true
	}

	// Network errors (no HTTP status code)
	if err.ResponseCode == 0 {
		return true
	}

	// HTTP 429 (Too Many Requests) could indicate service overload
	if err.ResponseCode == 429 {
		return true
	}

	// All other errors (4xx client errors, etc.) don't indicate service issues
	return false
}

// executeHTTPRequest performs the actual HTTP request execution with proper
// content type handling, header management, and response processing.
//
// This method handles:
//   - Request body encoding based on content type (JSON, multipart, form-urlencoded)
//   - HTTP header management (defaults + request-specific + generated headers)
//   - Query parameter encoding and URL construction
//   - Response body reading and parsing
//   - Error response handling and classification
//
// Parameters:
//   - method: HTTP method to execute
//   - endpoint: URL path to append to configured host
//
// Returns:
//   - *errors.ErrorDetails: Error information if request failed, nil on success
func (nc *NetworkClient) executeHTTPRequest(method enums.HttpMethods, endpoint string) *errors.ErrorDetails {
	// Delegate to content-type-specific request builders
	switch nc.requestType {
	case enums.Json.ToString():
		return nc.sendJSONRequest(method, endpoint)
	case enums.Multipart.ToString():
		return nc.sendMultipartRequest(method, endpoint)
	case enums.FormUrlEncoded.ToString():
		return nc.sendFormRequest(method, endpoint)
	default:
		return exceptions.GenericException(
			fmt.Sprintf("Unsupported request type: %s", nc.requestType),
			inbuiltErr.New(fmt.Sprintf("Unsupported request type: %s", nc.requestType)),
			500,
		)
	}
}

// sendJSONRequest builds and executes an HTTP request with JSON body encoding.
// This is the most common request type for REST APIs.
func (nc *NetworkClient) sendJSONRequest(method enums.HttpMethods, endpoint string) *errors.ErrorDetails {
	var bodyBuffer *bytes.Buffer

	if nc.body != nil {
		// Encode request body as JSON
		jsonBytes, err := json.Marshal(nc.body)
		if err != nil {
			return exceptions.GenericException(
				fmt.Sprintf("Failed to encode request body as JSON: %s", err.Error()),
				err,
				500,
			)
		}
		bodyBuffer = bytes.NewBuffer(jsonBytes)
	} else {
		bodyBuffer = bytes.NewBuffer(nil)
	}

	// Create HTTP request
	req, err := http.NewRequest(method.String(), nc.host+endpoint, bodyBuffer)
	if err != nil {
		return exceptions.GenericException(
			fmt.Sprintf("Failed to create HTTP request: %s", err.Error()),
			err,
			500,
		)
	}

	return nc.executeHTTPRequestWithHeaders(req)
}

// sendMultipartRequest builds and executes an HTTP request with multipart body encoding.
// Used for file uploads and complex forms with mixed content types.
func (nc *NetworkClient) sendMultipartRequest(method enums.HttpMethods, endpoint string) *errors.ErrorDetails {
	var bodyBuffer *bytes.Buffer
	var contentType string

	if nc.multipart != nil {
		// Create multipart body
		buffer, ct, err := nc.multipart.CreateBuffer()
		if err != nil {
			return exceptions.GenericException(
				fmt.Sprintf("Failed to create multipart body: %s", err.Error()),
				err,
				500,
			)
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
		return exceptions.GenericException(
			fmt.Sprintf("Failed to create HTTP request: %s", err.Error()),
			err,
			500,
		)
	}

	// Override request type with actual multipart content type (includes boundary)
	nc.requestType = contentType

	return nc.executeHTTPRequestWithHeaders(req)
}

// sendFormRequest builds and executes an HTTP request with form URL-encoded body.
// Used for traditional HTML form submissions and simple key-value data.
func (nc *NetworkClient) sendFormRequest(method enums.HttpMethods, endpoint string) *errors.ErrorDetails {
	var bodyBuffer *bytes.Buffer

	if nc.body != nil {
		// Convert body to form values
		form := url.Values{}
		if formData, ok := nc.body.(map[string]string); ok {
			for key, value := range formData {
				form.Set(key, value)
			}
		} else {
			return exceptions.GenericException(
				"Form URL-encoded body must be map[string]string",
				nil,
				400,
			)
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
		return exceptions.GenericException(
			fmt.Sprintf("Failed to create HTTP request: %s", err.Error()),
			err,
			500,
		)
	}

	return nc.executeHTTPRequestWithHeaders(req)
}

// executeHTTPRequestWithHeaders finalizes the HTTP request by setting headers,
// query parameters, context, and executing the request with response processing.
func (nc *NetworkClient) executeHTTPRequestWithHeaders(req *http.Request) *errors.ErrorDetails {
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

	// Log request details if logger is configured
	nc.logRequest(req)

	// Execute HTTP request
	start := time.Now()
	resp, err := nc.httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		// Network error or timeout
		nc.logRequestError(req, err, duration)
		return exceptions.GenericException(
			fmt.Sprintf("HTTP request failed: %s", err.Error()),
			err,
			0, // No HTTP status for network errors
		)
	}

	// Process HTTP response
	return nc.processHTTPResponse(req, resp, duration)
}

// processHTTPResponse handles HTTP response processing including body reading,
// status code evaluation, success/error parsing, and logging.
func (nc *NetworkClient) processHTTPResponse(req *http.Request, resp *http.Response, duration time.Duration) *errors.ErrorDetails {
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		nc.logRequestError(req, err, duration)
		return exceptions.GenericException(
			fmt.Sprintf("Failed to read response body: %s", err.Error()),
			err,
			resp.StatusCode,
		)
	}

	bodyString := string(bodyBytes)

	// Log response details
	nc.logResponse(req, resp, duration, len(bodyBytes))

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
			inbuiltErr.New(bodyString),
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

// === LOGGING HELPERS ===

// logRequest logs HTTP request details for observability and debugging.
func (nc *NetworkClient) logRequest(req *http.Request) {
	if nc.logger == nil {
		return
	}

	// Create sanitized headers (remove sensitive information)
	sanitizedHeaders := make(map[string]string)
	for key, values := range req.Header {
		if nc.isSensitiveHeader(key) {
			sanitizedHeaders[key] = "[REDACTED]"
		} else {
			sanitizedHeaders[key] = strings.Join(values, ", ")
		}
	}

	nc.logger.Info("HTTP request started",
		zap.String("method", req.Method),
		zap.String("url", req.URL.String()),
		zap.Any("headers", sanitizedHeaders),
		zap.String("request_id", req.Header.Get(constants.XRequestId)),
	)
}

// logResponse logs HTTP response details for observability and performance monitoring.
func (nc *NetworkClient) logResponse(req *http.Request, resp *http.Response, duration time.Duration, bodySize int) {
	if nc.logger == nil {
		return
	}

	nc.logger.Info("HTTP request completed",
		zap.String("method", req.Method),
		zap.String("url", req.URL.String()),
		zap.Int("status_code", resp.StatusCode),
		zap.Duration("duration", duration),
		zap.Int("response_size", bodySize),
		zap.String("request_id", req.Header.Get(constants.XRequestId)),
	)
}

// logRequestError logs HTTP request errors for debugging and monitoring.
func (nc *NetworkClient) logRequestError(req *http.Request, err error, duration time.Duration) {
	if nc.logger == nil {
		return
	}

	nc.logger.Error("HTTP request failed",
		zap.String("method", req.Method),
		zap.String("url", req.URL.String()),
		zap.Error(err),
		zap.Duration("duration", duration),
		zap.String("request_id", req.Header.Get(constants.XRequestId)),
	)
}

// isSensitiveHeader determines if a header contains sensitive information
// that should be redacted in logs for security purposes.
func (nc *NetworkClient) isSensitiveHeader(headerName string) bool {
	sensitiveHeaders := []string{
		"authorization",
		"x-api-key",
		"x-auth-token",
		"cookie",
		"set-cookie",
	}

	headerLower := strings.ToLower(headerName)
	for _, sensitive := range sensitiveHeaders {
		if headerLower == sensitive {
			return true
		}
	}
	return false
}
