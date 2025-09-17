package gorest

import (
	"net/http"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// === TRANSPORT AND CONNECTION OPTIONS ===

// ClientOption is a function type used to configure Client instances.
// This pattern provides a clean, extensible way to configure clients with
// optional parameters while maintaining backward compatibility.
//
// The options pattern benefits:
//   - Clean API without parameter explosion
//   - Backward compatible when adding new options
//   - Self-documenting configuration
//   - Composable and reusable configuration
//
// Example usage:
//
//	client := NewClient(
//	    WithHost("https://api.example.com"),
//	    WithTimeout(30 * time.Second),
//	    WithRateLimit(rate.Limit(100), 20),
//	)
type ClientOption func(*NetworkClient)

// EndpointOptions defines configuration options for specific API endpoints.
// Used with WithEndpointConfig to customize behavior for individual endpoints
// or endpoint patterns (using wildcards).
//
// This granular control enables:
//   - Different rate limits per endpoint based on criticality
//   - Endpoint-specific timeouts based on expected response times
//   - Custom retry policies based on operation idempotency
//   - Circuit breaker configuration based on service dependencies
type EndpointOptions struct {
	// RateLimit sets endpoint-specific rate limiting that overrides client defaults.
	// Use nil to inherit client default or disable rate limiting.
	//
	// Example: rate.NewLimiter(rate.Limit(10), 2) allows 10 req/sec with burst of 2
	RateLimit *rate.Limiter

	// CircuitBreaker configures fault tolerance for this specific endpoint.
	// Use nil to inherit client default or disable circuit breaking.
	//
	// Example: More sensitive for critical endpoints, less sensitive for optional features
	CircuitBreaker *CircuitBreakerConfig

	// Timeout overrides the default HTTP timeout for this endpoint.
	// Use 0 to inherit client default timeout.
	//
	// Example: Longer timeouts for search/reporting, shorter for simple CRUD
	Timeout time.Duration

	// Retry customizes retry behavior for this endpoint.
	// Use nil to inherit client default retry configuration.
	//
	// Example: More retries for idempotent operations, fewer for critical writes
	Retry *RetryConfig

	// CacheConfig defines caching behavior for this endpoint (future enhancement).
	// Reserved for implementing response caching capabilities.
	CacheConfig interface{} // Reserved for future caching implementation
}

// WithTransport configures a custom HTTP transport for the client.
// The transport controls low-level HTTP behavior including connection pooling,
// timeouts, TLS configuration, and proxy settings.
//
// Benefits of custom transport:
//   - Connection pooling: Reuse TCP connections for better performance
//   - Resource limits: Control memory usage with connection limits
//   - Network optimization: Tune timeouts for your network conditions
//   - Security: Configure TLS versions and certificate validation
//   - Proxy support: Route requests through corporate proxies
//
// Parameters:
//   - transport: Pre-configured http.Transport with desired settings
//
// Example:
//
//	transport := &http.Transport{
//	    MaxIdleConns:        200,              // Total connection pool size
//	    MaxIdleConnsPerHost: 20,               // Connections per host
//	    IdleConnTimeout:     90 * time.Second, // Keep-alive duration
//	    TLSHandshakeTimeout: 10 * time.Second, // TLS setup timeout
//	}
//	client := NewClient(WithTransport(transport))
//
// Production recommendations:
//   - MaxIdleConns: 100-500 depending on traffic volume
//   - MaxIdleConnsPerHost: 10-50 per upstream service
//   - IdleConnTimeout: 90s (matches server keep-alive)
//   - TLSHandshakeTimeout: 10s for reliable TLS setup
func WithTransport(transport *http.Transport) ClientOption {
	return func(nc *NetworkClient) {
		nc.httpClient.Transport = transport
	}
}

// WithTimeout sets the overall HTTP client timeout for all requests.
// This includes connection establishment, request sending, and response reading.
// Individual endpoint configurations can override this default.
//
// Timeout considerations:
//   - Too short: Legitimate slow requests may fail unnecessarily
//   - Too long: Failed requests consume resources and impact user experience
//   - Network conditions: Consider latency to your services
//   - Operation types: Read operations typically faster than writes
//
// Parameters:
//   - timeout: Maximum duration for HTTP requests
//
// Example:
//
//	client := NewClient(
//	    WithTimeout(30 * time.Second), // 30 second default timeout
//	)
//
// Recommended timeouts by use case:
//   - Internal services: 10-30 seconds
//   - External APIs: 30-60 seconds
//   - File uploads: 5-15 minutes
//   - Search/reports: 60-120 seconds
func WithTimeout(timeout time.Duration) ClientOption {
	return func(nc *NetworkClient) {
		nc.httpClient.Timeout = timeout
	}
}

// === RATE LIMITING OPTIONS ===

// WithRateLimit configures default rate limiting for all endpoints using the token bucket algorithm.
// This sets the default rate limiter for endpoints without specific configuration.
// Individual endpoints can override this default with WithEndpointConfig.
//
// Token bucket algorithm benefits:
//   - Smooth traffic: Allows steady request rates with burst capacity
//   - Burst handling: Accommodates short bursts of traffic
//   - Fair queuing: Requests wait fairly for available tokens
//   - Precise control: Exact rate and burst size configuration
//
// Parameters:
//   - rps: Requests per second (rate.Limit) - steady-state rate
//   - burst: Maximum burst size - tokens available for immediate use
//
// Example:
//
//	client := NewClient(
//	    WithRateLimit(rate.Limit(50), 10), // 50 req/sec with burst of 10
//	)
//
// Rate limiting strategies by service type:
//   - Critical services (payment): rate.Limit(5), burst 2
//   - User-facing APIs: rate.Limit(100), burst 20
//   - Internal services: rate.Limit(500), burst 50
//   - Background jobs: rate.Limit(10), burst 1
//
// Special values:
//   - rate.Inf: No rate limiting (unlimited)
//   - rate.Limit(0): Block all requests
func WithRateLimit(rps rate.Limit, burst int) ClientOption {
	return func(nc *NetworkClient) {
		nc.defaultEndpointConfig.rateLimiter = rate.NewLimiter(rps, burst)
	}
}

// === FAULT TOLERANCE OPTIONS ===

// WithCircuitBreaker configures client-wide circuit breaker for fault tolerance.
// Circuit breakers prevent cascade failures by monitoring downstream service health
// and failing fast when services are struggling, giving them time to recover.
//
// Benefits of circuit breaking:
//   - Prevents cascade failures in microservice architectures
//   - Reduces resource consumption on failing requests
//   - Provides fast failure responses instead of long timeouts
//   - Automatically tests for service recovery
//   - Protects upstream services from overloading downstream
//
// Parameters:
//   - config: CircuitBreakerConfig with failure thresholds and timings
//
// Example:
//
//	client := NewClient(
//	    WithCircuitBreaker(CircuitBreakerConfig{
//	        MaxFailures:  5,                // Open after 5 consecutive failures
//	        ResetTimeout: 30 * time.Second, // Test recovery every 30 seconds
//	    }),
//	)
//
// Configuration guidelines by service criticality:
//   - Critical services: MaxFailures 2-3, ResetTimeout 60s
//   - Standard services: MaxFailures 5-10, ResetTimeout 30s
//   - Non-critical services: MaxFailures 10-20, ResetTimeout 15s
//   - Development/testing: Disable circuit breaker
func WithCircuitBreaker(config CircuitBreakerConfig) ClientOption {
	return func(nc *NetworkClient) {
		nc.defaultEndpointConfig.circuitBreaker = NewCircuitBreaker(config)
	}
}

// WithRetry configures client-wide retry behavior for handling transient failures.
// Automatic retries improve reliability by handling temporary network issues,
// rate limiting responses, and brief service outages without manual intervention.
//
// Retry benefits:
//   - Handles transient network failures automatically
//   - Improves overall request success rates
//   - Reduces need for application-level retry logic
//   - Implements exponential backoff to avoid overwhelming services
//
// Parameters:
//   - config: RetryConfig defining retry attempts, delays, and conditions
//
// Example:
//
//	client := NewClient(
//	    WithRetry(RetryConfig{
//	        MaxRetries: 3,                        // Retry up to 3 times
//	        BaseDelay:  100 * time.Millisecond,   // Start with 100ms delay
//	        MaxDelay:   5 * time.Second,          // Cap delays at 5 seconds
//	        RetryableStatusCodes: []int{429, 500, 502, 503, 504},
//	    }),
//	)
//
// Retry strategies by operation type:
//   - Idempotent reads: Aggressive retries (5-10 attempts)
//   - Idempotent writes: Moderate retries (3-5 attempts)
//   - Non-idempotent writes: Conservative retries (1-2 attempts)
//   - Critical operations: Longer delays, more attempts
func WithRetry(config RetryConfig) ClientOption {
	return func(nc *NetworkClient) {
		nc.defaultEndpointConfig.retryConfig = &config
	}
}

// === HOST AND ENDPOINT OPTIONS ===

// WithHost sets the default base URL for all requests from this client.
// The host should include the protocol (https://) and can include a port number.
// Individual requests can override this by calling Host() in the request chain.
//
// Parameters:
//   - host: Base URL including protocol (e.g., "https://api.example.com")
//
// Example:
//
//	client := NewClient(
//	    WithHost("https://api.stripe.com/v1"), // Stripe API base URL
//	)
//	// All requests will be sent to https://api.stripe.com/v1/endpoint
//
// Best practices:
//   - Always include protocol (https:// for production)
//   - Use environment variables for different deployment environments
//   - Include API version in base URL if applicable
//   - Avoid trailing slashes to prevent double-slash issues
func WithHost(host string) ClientOption {
	return func(nc *NetworkClient) {
		nc.host = host
	}
}

// WithEndpointConfig configures endpoint-specific behavior that overrides client defaults.
// This enables fine-grained control over individual API endpoints based on their
// unique characteristics, performance requirements, and criticality levels.
//
// Endpoint patterns supported:
//   - Exact match: "/auth/login" matches only that specific endpoint
//   - Wildcard patterns: "/products/*" matches /products/123, /products/search, etc.
//   - Admin patterns: "/admin/*" matches all admin endpoints
//
// Parameters:
//   - endpoint: Endpoint path or pattern (supports wildcards with *)
//   - options: EndpointOptions with rate limiting, circuit breaker, timeout, retry config
//
// Example:
//
//	client := NewClient(
//	    WithHost("https://api.example.com"),
//	    WithRateLimit(rate.Limit(100), 20), // Default rate limit
//
//	    // Authentication endpoint - strict limits
//	    WithEndpointConfig("/auth/login", EndpointOptions{
//	        RateLimit: rate.NewLimiter(rate.Limit(5), 1), // Very strict
//	        Timeout:   10 * time.Second,
//	        CircuitBreaker: &CircuitBreakerConfig{
//	            MaxFailures: 2, ResetTimeout: 60 * time.Second,
//	        },
//	    }),
//
//	    // Product endpoints - high volume
//	    WithEndpointConfig("/products/*", EndpointOptions{
//	        RateLimit: rate.NewLimiter(rate.Limit(200), 50), // Higher limit
//	        Timeout:   30 * time.Second, // Longer timeout for complex queries
//	    }),
//
//	    // Admin endpoints - very restricted
//	    WithEndpointConfig("/admin/*", EndpointOptions{
//	        RateLimit: rate.NewLimiter(rate.Limit(10), 2), // Low limit
//	        CircuitBreaker: &CircuitBreakerConfig{
//	            MaxFailures: 1, ResetTimeout: 30 * time.Second,
//	        },
//	    }),
//	)
//
// Configuration hierarchy (most specific wins):
//  1. Endpoint-specific config (highest priority)
//  2. Client default config
//  3. Global default config
//  4. Built-in defaults (lowest priority)
func WithEndpointConfig(endpoint string, options EndpointOptions) ClientOption {
	return func(nc *NetworkClient) {
		if nc.endpointConfigs == nil {
			nc.endpointConfigs = make(map[string]*EndpointConfig)
		}

		config := &EndpointConfig{
			rateLimiter: options.RateLimit,
			timeout:     options.Timeout,
			retryConfig: options.Retry,
		}

		// Create circuit breaker if configuration provided
		if options.CircuitBreaker != nil {
			config.circuitBreaker = NewCircuitBreaker(*options.CircuitBreaker)
		}

		nc.endpointConfigs[endpoint] = config
	}
}

// === OBSERVABILITY OPTIONS ===

// WithLogger configures structured logging for request/response tracing and debugging.
// Logging provides visibility into client behavior, performance metrics, and error patterns.
// Essential for production troubleshooting and performance monitoring.
//
// Logged information includes:
//   - Request details: Method, URL, headers (sanitized), request size
//   - Response details: Status code, response time, response size
//   - Rate limiting events: When requests are throttled
//   - Circuit breaker events: State changes and failure patterns
//   - Retry attempts: When and why requests are retried
//   - Error details: Network failures, timeouts, server errors
//
// Parameters:
//   - logger: Configured zap.Logger instance with desired output format and level
//
// Example:
//
//	logger, _ := zap.NewProduction() // JSON format for production
//	// OR
//	logger, _ := zap.NewDevelopment() // Human-readable for development
//
//	client := NewClient(
//	    WithLogger(logger),
//	)
//
// Production logging best practices:
//   - Use JSON format for machine parsing
//   - Configure appropriate log levels (INFO for requests, ERROR for failures)
//   - Include correlation IDs for request tracing
//   - Sanitize sensitive headers (Authorization, API keys)
//   - Consider log volume impact on performance and storage
func WithLogger(logger *zap.Logger) ClientOption {
	return func(nc *NetworkClient) {
		nc.logger = logger
	}
}

// WithDefaultHeaders configures headers that are automatically added to all requests.
// Default headers establish organization-wide standards and reduce repetitive header
// configuration across different parts of your application.
//
// Common default headers:
//   - User-Agent: Identify your application and version
//   - Accept: Specify preferred response content types
//   - Content-Type: Default content type for request bodies
//   - Authorization: Service-to-service authentication (if applicable)
//   - API-Version: Specify API version for versioned services
//   - Custom headers: Organization-specific tracking or routing headers
//
// Parameters:
//   - headers: Map of header names to values applied to all requests
//
// Example:
//
//	client := NewClient(
//	    WithDefaultHeaders(map[string]string{
//	        "User-Agent":    "MyApp/2.1.0",
//	        "Accept":        "application/json",
//	        "Content-Type":  "application/json",
//	        "API-Version":   "v1",
//	        "X-Client-Name": "payment-service",
//	    }),
//	)
//
// Header precedence (request-specific headers override defaults):
//  1. Request-specific headers (highest priority)
//  2. Default headers configured here
//  3. Library-generated headers (Content-Type, X-Request-ID)
//
// Security considerations:
//   - Avoid putting secrets in default headers
//   - Use request-specific headers for authentication tokens
//   - Consider header size impact on request overhead
func WithDefaultHeaders(headers map[string]string) ClientOption {
	return func(nc *NetworkClient) {
		if nc.defaultHeaders == nil {
			nc.defaultHeaders = make(map[string]string)
		}
		for k, v := range headers {
			nc.defaultHeaders[k] = v
		}
	}
}
