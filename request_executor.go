package gorest

import (
	"context"
	inbuiltErr "errors"
	"fmt"
	"go.uber.org/zap"
	"io"
	"math"
	"time"

	"github.com/xander1235/gorest/v2/constants"
	"github.com/xander1235/gorest/v2/constants/enums"
	"github.com/xander1235/gorest/v2/exceptions"
	"github.com/xander1235/gorest/v2/exceptions/errors"
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
	// Build the HTTP request
	req, err := nc.buildHTTPRequest(method, endpoint)
	if err != nil {
		return err
	}

	// Create middleware context for the request pipeline
	ctx := &MiddlewareContext{
		Request:   req,
		Response:  nil,
		Error:     nil,
		StartTime: time.Now(),
		Metadata:  make(map[string]interface{}),
		Endpoint:  endpoint,
		Method:    method.String(),
		Client:    nc,
	}

	// Execute request interceptors (simple pre-request processing)
	for _, interceptor := range nc.requestInterceptors {
		if !interceptor(ctx) {
			// Request interceptor aborted the request
			return &errors.ErrorDetails{
				Message:      "Request aborted by request interceptor",
				ResponseCode: 0,
			}
		}
	}

	// Execute middleware chain (comprehensive processing)
	_ = nc.executeMiddlewareChain(ctx, false)

	// Execute response interceptors (simple post-response processing)
	for _, interceptor := range nc.responseInterceptors {
		if !interceptor(ctx) {
			// Response interceptor modified the response context
			break
		}
	}

	// Return error from context (set by middleware chain or interceptors)
	return ctx.Error
}

// Execute middleware chain (comprehensive processing)
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
func (nc *NetworkClient) executeWithRetries(ctx *MiddlewareContext, config *EndpointConfig, useMiddleware bool, isStreaming bool) *errors.ErrorDetails {
	var result *errors.ErrorDetails
	retryConfig := config.retryConfig

	if retryConfig == nil {
		// Single attempt - choose appropriate execution path
		if useMiddleware {
			result = nc.attemptHTTPRequestWithMiddleware(ctx, isStreaming)
		} else {
			// Equivalent of single attempt in original pipeline
			// This would call the underlying execution method without retries
			// We don't have its implementation in the provided code, but it would look like:
			result = nc.executeSingleAttempt(ctx, isStreaming)
		}
	} else {
		// Multi-attempt with retries - logic is shared regardless of path
		maxAttempts := retryConfig.MaxRetries + 1

		for attempt := 0; attempt < maxAttempts; attempt++ {
			// Execute attempt based on mode
			var err *errors.ErrorDetails

			if useMiddleware {
				// Reset request body for middleware retries if possible
				if attempt > 0 && ctx.Request != nil && ctx.Request.GetBody != nil {
					if newBody, gerr := ctx.Request.GetBody(); gerr == nil {
						ctx.Request.Body = newBody
					}
				}
				err = nc.attemptHTTPRequestWithMiddleware(ctx, isStreaming)
			} else {
				// For original pipeline, use the appropriate single attempt function
				err = nc.executeSingleAttempt(ctx, isStreaming)
			}

			// Check for success
			if err == nil {
				result = nil
				break
			} else {
				result = err
			}

			// Decide whether to retry
			if !nc.shouldRetry(result, retryConfig) {
				break
			}

			// Don't delay after the last attempt
			if attempt < maxAttempts-1 {
				delay := nc.calculateRetryDelay(attempt, retryConfig)

				// Respect request context cancellation
				var reqCtx context.Context
				if useMiddleware && ctx.Request != nil {
					reqCtx = ctx.Request.Context()
				} else {
					// Use client context or background for non-middleware path
					reqCtx = nc.ctx
					if reqCtx == nil {
						reqCtx = context.Background()
					}
				}

				timer := time.NewTimer(delay)
				select {
				case <-timer.C:
					// proceed
				case <-reqCtx.Done():
					timer.Stop()
					result = &errors.ErrorDetails{
						Message:      fmt.Sprintf("Request cancelled during retry delay: %s", reqCtx.Err()),
						ResponseCode: 0,
					}
					attempt = maxAttempts // break outer loop
				}
			}
			// Optionally log retries if nc.logger != nil (omitted for brevity)
		}
	}

	return result
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
	// Build the HTTP request based on current requestType/body
	req, buildErr := nc.buildHTTPRequest(method, endpoint)
	if buildErr != nil {
		return buildErr
	}

	// Ensure context is applied (defensive in case finalize didn't attach it)
	if nc.ctx != nil {
		req = req.WithContext(nc.ctx)
	}

	// Execute request
	start := time.Now()
	resp, err := nc.httpClient.Do(req)
	_ = time.Since(start) // duration available for future logging/metrics
	if err != nil {
		return exceptions.GenericException(
			fmt.Sprintf("HTTP request failed: %s", err.Error()),
			err,
			0,
		)
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return exceptions.GenericException(
			fmt.Sprintf("Failed to read response body: %s", readErr.Error()),
			readErr,
			resp.StatusCode,
		)
	}
	bodyString := string(bodyBytes)

	// Process response
	switch enums.HttpStatus(resp.StatusCode).SeriesType() {
	case enums.Successful:
		if nc.response != nil {
			if parseErr := nc.parser(bodyString, nc.response); parseErr != nil {
				return parseErr
			}
		}
		return nil
	case enums.ClientError:
		errDetails := nc.errorParser(bodyString)
		return exceptions.GenericException(
			errDetails.Message,
			errDetails.Error,
			resp.StatusCode,
		)
	case enums.ServerError:
		return exceptions.GenericException(
			constants.SomethingWentWrong,
			inbuiltErr.New(bodyString),
			resp.StatusCode,
		)
	default:
		return exceptions.GenericException(
			fmt.Sprintf("Unexpected HTTP status code: %d", resp.StatusCode),
			inbuiltErr.New(bodyString),
			resp.StatusCode,
		)
	}
}

// Removed legacy send*Request helpers that caused recursion. Request building is now
// centralized in buildHTTPRequest() and execution/processing happens here.
