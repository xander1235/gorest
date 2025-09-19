package gorest

import "go.uber.org/zap"

// === MIDDLEWARE AND INTERCEPTOR CONFIGURATION OPTIONS ===

// WithMiddleware adds a middleware to the client's middleware chain.
// Middlewares are executed in the order they are added and provide powerful
// cross-cutting concerns like authentication, logging, metrics, and error handling.
//
// Middlewares have access to both request and response data and can:
//   - Modify requests before sending (headers, body, URL)
//   - Process responses after receiving (logging, metrics, transformation)
//   - Handle errors and implement retry logic
//   - Short-circuit the request chain
//   - Store data for other middlewares
//
// Parameters:
//   - middleware: A Middleware function that processes requests/responses
//
// Example:
//
//	client := NewClient(
//	    WithMiddleware(func(ctx *MiddlewareContext, next NextFunc) {
//	        // Pre-request processing
//	        ctx.Request.Header.Set("X-Request-ID", generateID())
//
//	        // Execute request
//	        if next() {
//	            // Post-response processing
//	            log.Printf("Request completed in %v", ctx.Duration)
//	        }
//	    }),
//	)
//
// Execution order:
//  1. Middleware 1 pre-request → Middleware 2 pre-request → HTTP Request
//  2. HTTP Response → Middleware 2 post-response → Middleware 1 post-response
func WithMiddleware(middleware Middleware) ClientOption {
	return func(nc *NetworkClient) {
		nc.middlewares = append(nc.middlewares, middleware)
	}
}

// WithMiddlewares adds multiple middlewares to the client's middleware chain.
// This is a convenience function for adding multiple middlewares at once.
//
// Parameters:
//   - middlewares: Variable number of Middleware functions
//
// Example:
//
//	client := NewClient(
//	    WithMiddlewares(
//	        authMiddleware.Execute,
//	        loggingMiddleware.Execute,
//	        metricsMiddleware.Execute,
//	    ),
//	)
func WithMiddlewares(middlewares ...Middleware) ClientOption {
	return func(nc *NetworkClient) {
		nc.middlewares = append(nc.middlewares, middlewares...)
	}
}

// WithRequestInterceptor adds a request interceptor for simple request modifications.
// Request interceptors are simpler than full middlewares and only process requests
// before they are sent. Use them for authentication, header addition, or validation.
//
// Request interceptors execute before middlewares and can abort the request
// by returning false.
//
// Parameters:
//   - interceptor: A RequestInterceptor function that modifies requests
//
// Example:
//
//	client := NewClient(
//	    WithRequestInterceptor(func(ctx *MiddlewareContext) bool {
//	        // Add authentication token
//	        ctx.Request.Header.Set("Authorization", "Bearer "+getToken())
//	        return true // Continue processing
//	    }),
//	)
func WithRequestInterceptor(interceptor RequestInterceptor) ClientOption {
	return func(nc *NetworkClient) {
		nc.requestInterceptors = append(nc.requestInterceptors, interceptor)
	}
}

// WithRequestInterceptors adds multiple request interceptors.
// This is a convenience function for adding multiple request interceptors at once.
//
// Parameters:
//   - interceptors: Variable number of RequestInterceptor functions
//
// Example:
//
//	client := NewClient(
//	    WithRequestInterceptors(
//	        addAuthHeader,
//	        addCorrelationID,
//	        validateRequest,
//	    ),
//	)
func WithRequestInterceptors(interceptors ...RequestInterceptor) ClientOption {
	return func(nc *NetworkClient) {
		nc.requestInterceptors = append(nc.requestInterceptors, interceptors...)
	}
}

// WithResponseInterceptor adds a response interceptor for simple response processing.
// Response interceptors are simpler than full middlewares and only process responses
// after they are received. Use them for logging, metrics, or error transformation.
//
// Response interceptors execute after middlewares and cannot abort the request.
//
// Parameters:
//   - interceptor: A ResponseInterceptor function that processes responses
//
// Example:
//
//	client := NewClient(
//	    WithResponseInterceptor(func(ctx *MiddlewareContext) {
//	        // Log response metrics
//	        log.Printf("Response: %d in %v", ctx.Response.StatusCode, ctx.Duration)
//
//	        // Transform errors
//	        if ctx.Error != nil && ctx.Error.ResponseCode == 503 {
//	            ctx.Error.Message = "Service temporarily unavailable"
//	        }
//	    }),
//	)
func WithResponseInterceptor(interceptor ResponseInterceptor) ClientOption {
	return func(nc *NetworkClient) {
		nc.responseInterceptors = append(nc.responseInterceptors, interceptor)
	}
}

// WithResponseInterceptors adds multiple response interceptors.
// This is a convenience function for adding multiple response interceptors at once.
//
// Parameters:
//   - interceptors: Variable number of ResponseInterceptor functions
//
// Example:
//
//	client := NewClient(
//	    WithResponseInterceptors(
//	        logResponseMetrics,
//	        collectPerformanceData,
//	        transformErrors,
//	    ),
//	)
func WithResponseInterceptors(interceptors ...ResponseInterceptor) ClientOption {
	return func(nc *NetworkClient) {
		nc.responseInterceptors = append(nc.responseInterceptors, interceptors...)
	}
}

// === BUILT-IN MIDDLEWARE CONFIGURATION ===

// WithAuthMiddleware adds authentication middleware with a token provider.
// This is a convenience function for the common case of Bearer token authentication.
//
// Parameters:
//   - tokenProvider: Function that returns the current authentication token
//
// Example:
//
//	client := NewClient(
//	    WithAuthMiddleware(func() (string, error) {
//	        return os.Getenv("API_TOKEN"), nil
//	    }),
//	)
func WithAuthMiddleware(tokenProvider func() (string, error)) ClientOption {
	auth := NewAuthMiddleware(tokenProvider)
	return WithMiddleware(auth.Execute)
}

// WithCustomAuthMiddleware adds authentication middleware with custom configuration.
// Use this for non-Bearer token authentication or custom headers.
//
// Parameters:
//   - auth: Configured AuthMiddleware with custom settings
//
// Example:
//
//	client := NewClient(
//	    WithCustomAuthMiddleware(&AuthMiddleware{
//	        TokenProvider: getAPIKey,
//	        Scheme:        "ApiKey",
//	        Header:        "X-API-Key",
//	    }),
//	)
func WithCustomAuthMiddleware(auth *AuthMiddleware) ClientOption {
	return WithMiddleware(auth.Execute)
}

// WithLoggingMiddleware adds comprehensive request/response logging.
// This provides structured logging of all HTTP requests and responses.
//
// Parameters:
//   - logger: Configured zap.Logger for output
//
// Example:
//
//	logger, _ := zap.NewProduction()
//	client := NewClient(
//	    WithLoggingMiddleware(logger),
//	)
func WithLoggingMiddleware(logger *zap.Logger) ClientOption {
	logging := NewLoggingMiddleware(logger)
	return WithMiddleware(logging.Execute)
}

// WithCustomLoggingMiddleware adds logging middleware with custom configuration.
// Use this to control what gets logged and how it's formatted.
//
// Parameters:
//   - logging: Configured LoggingMiddleware with custom settings
//
// Example:
//
//	client := NewClient(
//	    WithCustomLoggingMiddleware(&LoggingMiddleware{
//	        Logger:           logger,
//	        LogRequests:      true,
//	        LogResponses:     true,
//	        LogRequestBody:   false, // Security: don't log sensitive data
//	        LogResponseBody:  false, // Performance: don't log large responses
//	        SensitiveHeaders: []string{"X-Internal-Token"},
//	    }),
//	)
func WithCustomLoggingMiddleware(logging *LoggingMiddleware) ClientOption {
	return WithMiddleware(logging.Execute)
}

// WithMetricsMiddleware adds performance and usage metrics collection.
// This tracks request duration, status codes, and request/response sizes.
//
// Parameters:
//   - collector: MetricsCollector implementation for metrics storage
//
// Example:
//
//	client := NewClient(
//	    WithMetricsMiddleware(prometheusCollector),
//	)
func WithMetricsMiddleware(collector MetricsCollector) ClientOption {
	metrics := NewMetricsMiddleware(collector)
	return WithMiddleware(metrics.Execute)
}

// WithRetryMiddleware adds automatic retry logic with exponential backoff.
// This middleware handles transient failures automatically.
//
// Parameters:
//   - maxRetries: Maximum number of retry attempts
//
// Example:
//
//	client := NewClient(
//	    WithRetryMiddleware(3), // Retry up to 3 times
//	)
func WithRetryMiddleware(maxRetries int) ClientOption {
	retry := NewRetryMiddleware(maxRetries)
	return WithMiddleware(retry.Execute)
}

// WithCustomRetryMiddleware adds retry middleware with custom configuration.
// Use this to control retry conditions, delays, and callbacks.
//
// Parameters:
//   - retry: Configured RetryMiddleware with custom settings
//
// Example:
//
//	client := NewClient(
//	    WithCustomRetryMiddleware(&RetryMiddleware{
//	        MaxRetries: 5,
//	        BaseDelay:  200 * time.Millisecond,
//	        MaxDelay:   10 * time.Second,
//	        RetryCondition: func(ctx *MiddlewareContext) bool {
//	            // Custom retry logic
//	            return ctx.Error != nil && ctx.Error.ResponseCode >= 500
//	        },
//	        OnRetry: func(attempt int, ctx *MiddlewareContext) {
//	            log.Printf("Retrying request (attempt %d): %s", attempt, ctx.Error.Message)
//	        },
//	    }),
//	)
func WithCustomRetryMiddleware(retry *RetryMiddleware) ClientOption {
	return WithMiddleware(retry.Execute)
}

// === MIDDLEWARE CHAIN HELPERS ===

// WithMiddlewareChain is a convenience function for adding multiple middlewares
// in a specific order. This makes it easy to create reusable middleware configurations.
//
// Parameters:
//   - middlewares: Slice of Middleware functions to add in order
//
// Example:
//
//	standardChain := []Middleware{
//	    authMiddleware.Execute,
//	    loggingMiddleware.Execute,
//	    metricsMiddleware.Execute,
//	    retryMiddleware.Execute,
//	}
//
//	client := NewClient(
//	    WithMiddlewareChain(standardChain),
//	)
func WithMiddlewareChain(middlewares []Middleware) ClientOption {
	return func(nc *NetworkClient) {
		nc.middlewares = append(nc.middlewares, middlewares...)
	}
}

// ClearMiddlewares removes all middlewares from the client.
// This is useful for testing or when you want to start with a clean slate.
//
// Example:
//
//	client := NewClient(
//	    WithAuthMiddleware(tokenProvider),
//	    WithLoggingMiddleware(logger),
//	    ClearMiddlewares(), // Remove all middlewares
//	    WithMetricsMiddleware(collector), // Add only metrics
//	)
func ClearMiddlewares() ClientOption {
	return func(nc *NetworkClient) {
		nc.middlewares = nil
		nc.requestInterceptors = nil
		nc.responseInterceptors = nil
	}
}
