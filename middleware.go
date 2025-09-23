package gorest

import (
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/xander1235/gorest/v2/exceptions/errors"
	"go.uber.org/zap"
)

// === MIDDLEWARE CORE INTERFACES ===

// MiddlewareContext contains all the data that flows through the middleware chain.
// It provides access to the HTTP request, response, and other contextual information
// that middlewares can inspect, modify, or augment during request processing.
//
// This context is passed through the entire middleware chain, allowing each middleware
// to access and modify the request/response data as needed.
type MiddlewareContext struct {
	// Request contains the HTTP request being processed.
	// Middlewares can modify headers, body, URL, etc.
	Request *http.Request

	// Response contains the HTTP response after execution (nil before request execution).
	// Response middlewares can inspect and modify the response.
	Response *http.Response

	// Error contains any error that occurred during request processing.
	// Error middlewares can handle, transform, or suppress errors.
	Error *errors.ErrorDetails

	// StartTime records when the request processing started.
	// Useful for timing and performance measurement middlewares.
	StartTime time.Time

	// Duration contains the total request duration (set after completion).
	// Available in response and error middlewares for performance tracking.
	Duration time.Duration

	// Metadata allows middlewares to store and share arbitrary data.
	// Use this for passing information between middlewares in the chain.
	Metadata map[string]interface{}

	// Endpoint is the endpoint path being requested (e.g., "/users/123").
	// Useful for endpoint-specific middleware behavior.
	Endpoint string

	// Method is the HTTP method being used (GET, POST, etc.).
	// Allows method-specific middleware processing.
	Method string

	// Client provides access to the NetworkClient making the request.
	// Allows middlewares to access client configuration and state.
	Client *NetworkClient
}

// NextFunc represents the function to call the next middleware in the chain.
// Middlewares must call next() to continue processing, or skip it to short-circuit.
// The bool return indicates whether to continue processing (true) or abort (false).
type NextFunc func() bool

// Middleware is the main middleware interface for intercepting and modifying requests/responses.
// Middlewares are executed in the order they were added to the client.
//
// Execution flow:
//  1. Pre-request middlewares execute (modify request, add headers, etc.)
//  2. HTTP request is sent
//  3. Post-response middlewares execute (process response, handle errors, etc.)
//
// Middlewares can:
//   - Modify the request before sending (headers, body, URL)
//   - Process the response after receiving (logging, metrics, error handling)
//   - Short-circuit the chain by not calling next()
//   - Store data in context.Metadata for other middlewares
//   - Handle errors and potentially retry or transform them
//
// Example middleware:
//
//	func LoggingMiddleware(ctx *MiddlewareContext, next NextFunc) {
//	    log.Printf("Request: %s %s", ctx.Method, ctx.Request.URL)
//
//	    if next() { // Continue to next middleware/request
//	        log.Printf("Response: %d in %v", ctx.Response.StatusCode, ctx.Duration)
//	    } else {
//	        log.Printf("Request aborted or failed: %v", ctx.Error)
//	    }
//	}
type Middleware func(ctx *MiddlewareContext, next NextFunc)

// RequestInterceptor is a simpler interface for middlewares that only need to modify requests.
// These are executed before the HTTP request is sent and cannot access the response.
//
// Use RequestInterceptor for:
//   - Adding authentication headers
//   - Modifying request URLs or parameters
//   - Request validation
//   - Adding correlation IDs
//
// Return false to abort the request, true to continue.
//
// Example:
//
//	func AuthInterceptor(ctx *MiddlewareContext) bool {
//	    ctx.Request.Header.Set("Authorization", "Bearer " + getToken())
//	    return true // Continue processing
//	}
type RequestInterceptor func(ctx *MiddlewareContext) bool

// ResponseInterceptor is a simpler interface for middlewares that only need to process responses.
// These are executed after the HTTP response is received and can access both request and response.
//
// Use ResponseInterceptor for:
//   - Response logging
//   - Metrics collection
//   - Response transformation
//   - Error handling
//
// The interceptor can modify ctx.Error to transform error handling.
//
// Example:
//
//	func MetricsInterceptor(ctx *MiddlewareContext) {
//	    metrics.RecordRequestDuration(ctx.Endpoint, ctx.Duration)
//	    if ctx.Response != nil {
//	        metrics.RecordStatusCode(ctx.Response.StatusCode)
//	    }
//	}
type ResponseInterceptor func(ctx *MiddlewareContext) bool

// === BUILT-IN MIDDLEWARES ===

// AuthMiddleware adds authentication headers to requests.
// Supports multiple authentication schemes with flexible token providers.
type AuthMiddleware struct {
	// TokenProvider is a function that returns the current authentication token.
	// This allows for dynamic token retrieval (e.g., refreshing expired tokens).
	TokenProvider func() (string, error)

	// Scheme is the authentication scheme to use ("Bearer", "Basic", "ApiKey", etc.).
	// Default is "Bearer" if not specified.
	Scheme string

	// Header is the HTTP header name to use for authentication.
	// Default is "Authorization" if not specified.
	Header string

	// OnError is called if token retrieval fails. If nil, the request is aborted.
	OnError func(err error) bool // Return true to continue despite error
}

// Execute implements the authentication middleware logic.
func (auth *AuthMiddleware) Execute(ctx *MiddlewareContext, next NextFunc) {
	// Set defaults
	scheme := auth.Scheme
	if scheme == "" {
		scheme = "Bearer"
	}

	header := auth.Header
	if header == "" {
		header = "Authorization"
	}

	// Get authentication token
	token, err := auth.TokenProvider()
	if err != nil {
		if auth.OnError != nil && auth.OnError(err) {
			// Continue despite error
			next()
			return
		}
		// Abort request due to auth failure
		ctx.Error = &errors.ErrorDetails{
			Message:      fmt.Sprintf("Authentication failed: %s", err.Error()),
			ResponseCode: 401,
		}
		return
	}

	// Add authentication header
	if scheme == "Basic" || strings.ToLower(scheme) == "basic" {
		ctx.Request.Header.Set(header, "Basic "+token)
	} else if scheme == "ApiKey" || strings.ToLower(scheme) == "apikey" {
		ctx.Request.Header.Set(header, token) // No prefix for API keys
	} else {
		// Default to Bearer or custom scheme
		ctx.Request.Header.Set(header, scheme+" "+token)
	}

	// Continue to next middleware
	next()
}

// LoggingMiddleware provides comprehensive request/response logging with structured output.
type LoggingMiddleware struct {
	// Logger is the zap logger instance to use for output.
	Logger *zap.Logger

	// LogRequests enables request logging (method, URL, headers).
	LogRequests bool

	// LogResponses enables response logging (status, duration, size).
	LogResponses bool

	// LogRequestBody enables request body logging (be careful with sensitive data).
	LogRequestBody bool

	// LogResponseBody enables response body logging (can be verbose).
	LogResponseBody bool

	// SensitiveHeaders lists headers that should be redacted in logs for security.
	SensitiveHeaders []string

	// MaxBodySize limits the number of bytes logged for request/response bodies.
	MaxBodySize int64
}

// Execute implements the logging middleware logic.
func (log *LoggingMiddleware) Execute(ctx *MiddlewareContext, next NextFunc) {
	if log.Logger == nil {
		next()
		return
	}

	// Log request if enabled
	if log.LogRequests {
		fields := []zap.Field{
			zap.String("method", ctx.Method),
			zap.String("url", ctx.Request.URL.String()),
			zap.String("endpoint", ctx.Endpoint),
			zap.Time("start_time", ctx.StartTime),
		}

		// Add headers (with sensitive data redaction)
		headers := make(map[string]string)
		for name, values := range ctx.Request.Header {
			if log.isSensitiveHeader(name) {
				headers[name] = "[REDACTED]"
			} else {
				headers[name] = strings.Join(values, ", ")
			}
		}
		fields = append(fields, zap.Any("headers", headers))

		// Add request body if enabled
		if log.LogRequestBody && ctx.Request.Body != nil {
			// Note: This would need body reading/restoration logic in production
			fields = append(fields, zap.String("request_body", "[BODY_LOGGING_NOT_IMPLEMENTED]"))
		}

		log.Logger.Info("HTTP Request Started", fields...)
	}

	// Execute next middleware and measure duration
	start := time.Now()
	continued := next()
	duration := time.Since(start)
	ctx.Duration = duration

	// Log response if enabled
	if log.LogResponses {
		fields := []zap.Field{
			zap.String("method", ctx.Method),
			zap.String("endpoint", ctx.Endpoint),
			zap.Duration("duration", duration),
			zap.Bool("success", continued && ctx.Error == nil),
		}

		if ctx.Response != nil {
			fields = append(fields,
				zap.Int("status_code", ctx.Response.StatusCode),
				zap.Int64("content_length", ctx.Response.ContentLength),
			)
		}

		if ctx.Error != nil {
			fields = append(fields,
				zap.String("error_message", ctx.Error.Message),
				zap.Int("error_code", ctx.Error.ResponseCode),
			)
		}

		if continued && ctx.Error == nil {
			log.Logger.Info("HTTP Request Completed", fields...)
		} else {
			log.Logger.Error("HTTP Request Failed", fields...)
		}
	}
}

// isSensitiveHeader checks if a header contains sensitive information that should be redacted.
func (log *LoggingMiddleware) isSensitiveHeader(headerName string) bool {
	defaultSensitive := []string{
		"authorization", "x-api-key", "x-auth-token", "cookie", "set-cookie",
	}

	headerLower := strings.ToLower(headerName)

	// Check custom sensitive headers
	for _, sensitive := range log.SensitiveHeaders {
		if strings.ToLower(sensitive) == headerLower {
			return true
		}
	}

	// Check default sensitive headers
	for _, sensitive := range defaultSensitive {
		if headerLower == sensitive {
			return true
		}
	}

	return false
}

// MetricsMiddleware collects performance and usage metrics.
type MetricsMiddleware struct {
	// MetricsCollector receives the collected metrics.
	Collector MetricsCollector
}

// MetricsCollector defines the interface for metrics collection backends.
type MetricsCollector interface {
	// RecordRequestDuration records the time taken for a request.
	RecordRequestDuration(endpoint, method string, duration time.Duration, success bool)

	// RecordRequestCount increments the request counter for an endpoint.
	RecordRequestCount(endpoint, method string, statusCode int)

	// RecordRequestSize records the size of request bodies.
	RecordRequestSize(endpoint, method string, size int64)

	// RecordResponseSize records the size of response bodies.
	RecordResponseSize(endpoint, method string, size int64)
}

// Execute implements the metrics collection middleware logic.
func (metrics *MetricsMiddleware) Execute(ctx *MiddlewareContext, next NextFunc) {
	if metrics.Collector == nil {
		next()
		return
	}

	// Record request size if available
	if ctx.Request.ContentLength > 0 {
		metrics.Collector.RecordRequestSize(ctx.Endpoint, ctx.Method, ctx.Request.ContentLength)
	}

	// Execute request and measure duration
	start := time.Now()
	continued := next()
	duration := time.Since(start)

	success := continued && ctx.Error == nil
	statusCode := 0
	if ctx.Response != nil {
		statusCode = ctx.Response.StatusCode
	} else if ctx.Error != nil {
		statusCode = ctx.Error.ResponseCode
	}

	// Record metrics
	metrics.Collector.RecordRequestDuration(ctx.Endpoint, ctx.Method, duration, success)
	metrics.Collector.RecordRequestCount(ctx.Endpoint, ctx.Method, statusCode)

	// Record response size if available
	if ctx.Response != nil && ctx.Response.ContentLength > 0 {
		metrics.Collector.RecordResponseSize(ctx.Endpoint, ctx.Method, ctx.Response.ContentLength)
	}
}

// RetryMiddleware implements automatic retry logic with exponential backoff.
type RetryMiddleware struct {
	// MaxRetries is the maximum number of retry attempts.
	MaxRetries int

	// BaseDelay is the initial delay between retry attempts.
	BaseDelay time.Duration

	// MaxDelay caps the maximum delay between retries.
	MaxDelay time.Duration

	// RetryCondition determines whether a request should be retried based on the response.
	// If nil, default retry logic is used (5xx errors, timeouts, etc.).
	RetryCondition func(ctx *MiddlewareContext) bool

	// OnRetry is called before each retry attempt (for logging, metrics, etc.).
	OnRetry func(attempt int, ctx *MiddlewareContext)
}

// Execute implements the retry middleware logic.
func (retry *RetryMiddleware) Execute(ctx *MiddlewareContext, next NextFunc) {
	if retry.MaxRetries <= 0 {
		next()
		return
	}

	for attempt := 0; attempt <= retry.MaxRetries; attempt++ {
		// Clear previous error
		ctx.Error = nil
		ctx.Response = nil

		// Execute request
		continued := next()

		// If successful or non-retryable, return
		if continued && ctx.Error == nil {
			return
		}

		// Check if we should retry
		if !retry.shouldRetry(ctx) || attempt >= retry.MaxRetries {
			return
		}

		// Calculate delay for next attempt
		delay := retry.calculateDelay(attempt)

		// Notify about retry
		if retry.OnRetry != nil {
			retry.OnRetry(attempt+1, ctx)
		}

		// Wait before next attempt
		if ctx.Request.Context() != nil {
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
				// Continue with retry
			case <-ctx.Request.Context().Done():
				// Context cancelled
				timer.Stop()
				ctx.Error = &errors.ErrorDetails{
					Message:      "Request cancelled during retry delay",
					ResponseCode: 0,
				}
				return
			}
		} else {
			time.Sleep(delay)
		}
	}
}

// shouldRetry determines if a request should be retried based on the error/response.
func (retry *RetryMiddleware) shouldRetry(ctx *MiddlewareContext) bool {
	if retry.RetryCondition != nil {
		return retry.RetryCondition(ctx)
	}

	// Default retry logic
	if ctx.Error == nil {
		return false
	}

	// Retry on server errors (5xx) and rate limiting (429)
	switch ctx.Error.ResponseCode {
	case 429, 500, 502, 503, 504:
		return true
	case 0: // Network errors
		return true
	default:
		return false
	}
}

// calculateDelay computes the delay before the next retry using exponential backoff.
func (retry *RetryMiddleware) calculateDelay(attempt int) time.Duration {
	baseDelay := retry.BaseDelay
	if baseDelay <= 0 {
		baseDelay = 100 * time.Millisecond
	}

	// Exponential backoff: base * (2 ^ attempt)
	delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))

	// Cap at maximum delay if specified
	if retry.MaxDelay > 0 && delay > retry.MaxDelay {
		delay = retry.MaxDelay
	}

	return delay
}

// === CONVENIENCE FUNCTIONS ===

// NewAuthMiddleware creates an authentication middleware with a token provider.
func NewAuthMiddleware(tokenProvider func() (string, error)) *AuthMiddleware {
	return &AuthMiddleware{
		TokenProvider: tokenProvider,
		Scheme:        "Bearer",
		Header:        "Authorization",
	}
}

// NewLoggingMiddleware creates a logging middleware with the specified logger.
func NewLoggingMiddleware(logger *zap.Logger) *LoggingMiddleware {
	return &LoggingMiddleware{
		Logger:           logger,
		LogRequests:      true,
		LogResponses:     true,
		LogRequestBody:   false, // Disabled by default for security
		LogResponseBody:  false, // Disabled by default for performance
		SensitiveHeaders: []string{},
		MaxBodySize:      1024, // 1KB limit
	}
}

// NewMetricsMiddleware creates a metrics middleware with the specified collector.
func NewMetricsMiddleware(collector MetricsCollector) *MetricsMiddleware {
	return &MetricsMiddleware{
		Collector: collector,
	}
}

// NewRetryMiddleware creates a retry middleware with sensible defaults.
func NewRetryMiddleware(maxRetries int) *RetryMiddleware {
	return &RetryMiddleware{
		MaxRetries: maxRetries,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   5 * time.Second,
	}
}
