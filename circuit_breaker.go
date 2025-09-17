package gorest

import (
	"golang.org/x/time/rate"
	"sync"
	"time"
)

// CircuitBreakerState represents the current state of a circuit breaker.
// The circuit breaker pattern helps prevent cascade failures by monitoring
// the health of downstream services and failing fast when they're struggling.
type CircuitBreakerState string

const (
	// StateClosed indicates normal operation where all requests are allowed to pass through.
	// The circuit breaker monitors failures and transitions to Open when failure threshold is exceeded.
	StateClosed CircuitBreakerState = "closed"

	// StateOpen indicates the circuit is tripped and requests fail immediately without
	// reaching the downstream service. This gives the failing service time to recover
	// while providing fast failure responses to clients.
	StateOpen CircuitBreakerState = "open"

	// StateHalfOpen indicates the circuit is testing if the downstream service has recovered.
	// Limited requests are allowed through to test service health. Success closes the circuit,
	// while failures immediately reopen it.
	StateHalfOpen CircuitBreakerState = "half-open"
)

// CircuitBreakerConfig defines the configuration for circuit breaker fault tolerance.
// Circuit breaker prevents cascade failures by temporarily blocking requests to failing services.
//
// The circuit breaker operates in three states:
//   - CLOSED: Normal operation, requests pass through
//   - OPEN: Circuit is tripped, requests fail immediately without hitting the service
//   - HALF_OPEN: Testing if service has recovered, limited requests allowed
//
// Benefits:
//   - Prevents cascade failures when downstream services are struggling
//   - Reduces load on failing services, giving them time to recover
//   - Provides fast failure response instead of long timeouts
//   - Automatically tests for service recovery
type CircuitBreakerConfig struct {
	// MaxFailures is the number of consecutive failures required to open the circuit.
	// Lower values make the circuit breaker more sensitive to failures.
	// Typical values: 3-10 depending on service criticality.
	//
	// Example: MaxFailures: 5 means circuit opens after 5 consecutive 5xx errors
	MaxFailures int

	// ResetTimeout is how long to wait before attempting to close an open circuit.
	// This gives the failing service time to recover before testing it again.
	// Should be longer than typical service recovery time.
	//
	// Example: ResetTimeout: 30*time.Second waits 30 seconds before testing recovery
	ResetTimeout time.Duration

	// Timeout is the maximum duration for individual requests (currently unused).
	// Reserved for future enhancement to add request-level timeouts.
	Timeout time.Duration
}

// EndpointConfig defines per-endpoint configuration that overrides client defaults.
// This enables fine-grained control over different API endpoints that may have
// different performance characteristics, criticality levels, or rate limits.
//
// Use cases:
//   - Authentication endpoints: Lower rate limits, shorter timeouts
//   - Search endpoints: Higher rate limits, longer timeouts, caching
//   - Admin endpoints: Strict rate limits, circuit breakers
//   - File upload endpoints: No rate limits, longer timeouts
type EndpointConfig struct {
	// rateLimiter overrides the default client rate limiter for this specific endpoint.
	// Useful when different endpoints have different rate limit requirements.
	//
	// Example: Login endpoint might need stricter limits than product listing
	rateLimiter *rate.Limiter

	// circuitBreaker provides endpoint-specific fault tolerance configuration.
	// Critical endpoints might need different failure thresholds than others.
	//
	// Example: Payment endpoints might have lower failure tolerance than read-only APIs
	circuitBreaker *CircuitBreaker

	// timeout overrides the default HTTP client timeout for this endpoint.
	// Long-running operations might need extended timeouts.
	//
	// Example: Search endpoints might need 30s timeout vs 10s for simple CRUD operations
	timeout time.Duration

	// retryConfig customizes retry behavior for this specific endpoint.
	// Critical operations might need more aggressive retry policies.
	//
	// Example: Idempotent operations might retry more than non-idempotent ones
	retryConfig *RetryConfig
}

// RetryConfig defines retry behavior for handling transient failures.
// Implements exponential backoff with jitter to prevent thundering herd effects.
//
// Retry logic helps handle:
//   - Network hiccups and temporary connectivity issues
//   - Rate limit responses (429 Too Many Requests)
//   - Server overload conditions (503 Service Unavailable)
//   - Timeout errors from slow responses
//
// Benefits:
//   - Improves reliability by automatically handling transient failures
//   - Reduces manual error handling in application code
//   - Implements best practices like exponential backoff
//   - Prevents overwhelming failing services with immediate retries
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts for a failed request.
	// Higher values increase reliability but also increase latency for persistent failures.
	//
	// Example: MaxRetries: 3 means total of 4 attempts (1 original + 3 retries)
	MaxRetries int

	// BaseDelay is the initial delay before the first retry attempt.
	// This delay is exponentially increased for subsequent retries.
	//
	// Example: BaseDelay: 100*time.Millisecond starts with 100ms, then 200ms, 400ms, etc.
	BaseDelay time.Duration

	// MaxDelay caps the maximum delay between retry attempts.
	// Prevents excessively long delays that could impact user experience.
	//
	// Example: MaxDelay: 5*time.Second ensures no retry waits longer than 5 seconds
	MaxDelay time.Duration

	// RetryableStatusCodes defines which HTTP status codes should trigger retries.
	// Typically includes server errors (5xx) and rate limiting (429).
	//
	// Example: []int{429, 500, 502, 503, 504} retries on rate limits and server errors
	RetryableStatusCodes []int

	// RetryableErrors defines which error types should trigger retries.
	// Useful for handling network-level errors like timeouts and connection failures.
	RetryableErrors []error
}

// CircuitBreaker implements the circuit breaker pattern for fault tolerance.
// It monitors request failures and automatically stops sending requests to failing
// services, giving them time to recover while providing fast failure responses.
//
// The circuit breaker prevents:
//   - Cascade failures when downstream services are struggling
//   - Resource exhaustion from retrying against failing services
//   - Long response times waiting for timeouts on failed requests
//   - Overloading already struggling downstream services
//
// Implementation details:
//   - Thread-safe using mutex for concurrent access
//   - Tracks consecutive failures and last failure time
//   - Automatically transitions between states based on configuration
//   - Provides fast-fail mechanism during Open state
type CircuitBreaker struct {
	// config contains the circuit breaker configuration parameters
	config CircuitBreakerConfig

	// state tracks the current circuit breaker state (Closed/Open/Half-Open)
	state CircuitBreakerState

	// failures counts consecutive failures in the current state
	failures int

	// lastFailTime records when the most recent failure occurred
	// Used to determine when to transition from Open to Half-Open state
	lastFailTime time.Time

	// mutex protects concurrent access to circuit breaker state
	// Essential for thread-safety when multiple goroutines use the same client
	mutex sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker with the specified configuration.
// The circuit breaker starts in Closed state, allowing all requests through initially.
//
// Parameters:
//   - config: CircuitBreakerConfig defining failure thresholds and timing behavior
//
// Returns:
//   - *CircuitBreaker: A new circuit breaker instance ready for use
//
// Example:
//
//	cb := NewCircuitBreaker(CircuitBreakerConfig{
//	    MaxFailures:  5,                // Open circuit after 5 consecutive failures
//	    ResetTimeout: 30 * time.Second, // Wait 30 seconds before testing recovery
//	})
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		config: config,
		state:  StateClosed, // Start in closed state (normal operation)
	}
}

// AllowRequest determines whether a request should be allowed through the circuit breaker.
// This is the primary method used before making HTTP requests to check if the circuit
// allows the request or if it should fail fast.
//
// State-based behavior:
//   - CLOSED: Always allows requests (normal operation)
//   - OPEN: Denies requests until reset timeout expires, then transitions to HALF_OPEN
//   - HALF_OPEN: Allows requests to test service recovery
//
// Returns:
//   - bool: true if request should proceed, false if it should fail fast
//
// Thread Safety:
//
//	Uses read lock for performance when circuit is closed, write lock for state changes
//
// Example:
//
//	if !circuitBreaker.AllowRequest() {
//	    return errors.New("circuit breaker open - failing fast")
//	}
//	// Proceed with actual HTTP request
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	now := time.Now()

	switch cb.state {
	case StateClosed:
		// Normal operation - allow all requests
		return true

	case StateOpen:
		// Check if enough time has passed to test recovery
		if now.Sub(cb.lastFailTime) > cb.config.ResetTimeout {
			cb.state = StateHalfOpen
			cb.failures = 0 // Reset failure count for half-open testing
			return true
		}
		// Circuit still open - deny request
		return false

	case StateHalfOpen:
		// Allow request to test service recovery
		return true

	default:
		// Unknown state - default to allowing request
		return true
	}
}

// RecordSuccess records a successful request outcome.
// Success in any state moves the circuit back to Closed state and resets failure counters.
// This enables automatic recovery when the downstream service becomes healthy again.
//
// State transitions triggered by success:
//   - CLOSED → CLOSED: Reset failure counter (normal operation)
//   - HALF_OPEN → CLOSED: Service has recovered, resume normal operation
//   - OPEN → CLOSED: Should not occur (no requests allowed in Open state)
//
// Thread Safety:
//
//	Uses write lock to safely update circuit breaker state
//
// Example:
//
//	resp, err := http.Get(url)
//	if err == nil && resp.StatusCode < 500 {
//	    circuitBreaker.RecordSuccess()
//	}
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	// Success always resets to closed state with zero failures
	cb.failures = 0
	cb.state = StateClosed
}

// RecordFailure records a failed request outcome.
// Failures accumulate and can trigger state transitions when thresholds are exceeded.
//
// State transitions triggered by failure:
//   - CLOSED → OPEN: When failures exceed MaxFailures threshold
//   - HALF_OPEN → OPEN: Any failure immediately reopens the circuit
//   - OPEN → OPEN: Continues tracking failures (though requests shouldn't reach here)
//
// Failure criteria (typically):
//   - HTTP 5xx server errors (internal server error, service unavailable, etc.)
//   - Network timeouts and connection failures
//   - DNS resolution failures
//   - SSL/TLS handshake failures
//
// Thread Safety:
//
//	Uses write lock to safely update circuit breaker state and counters
//
// Example:
//
//	resp, err := http.Get(url)
//	if err != nil || resp.StatusCode >= 500 {
//	    circuitBreaker.RecordFailure()
//	}
func (cb *CircuitBreaker) RecordFailure() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.failures++
	cb.lastFailTime = time.Now()

	// State-specific failure handling
	switch cb.state {
	case StateClosed:
		// Check if we've exceeded the failure threshold
		if cb.failures >= cb.config.MaxFailures {
			cb.state = StateOpen
		}

	case StateHalfOpen:
		// Any failure in half-open immediately opens the circuit
		cb.state = StateOpen

	case StateOpen:
		// Already open - continue tracking failures but no state change needed
	}
}

// GetState returns the current state of the circuit breaker.
// Useful for monitoring, debugging, and metrics collection.
//
// Returns:
//   - CircuitBreakerState: Current state (Closed/Open/Half-Open)
//
// Thread Safety:
//
//	Uses read lock for safe concurrent access without blocking other readers
//
// Example:
//
//	state := circuitBreaker.GetState()
//	if state == StateOpen {
//	    log.Warn("Circuit breaker is open - service may be degraded")
//	}
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

// GetFailureCount returns the current number of consecutive failures.
// Useful for monitoring circuit breaker health and failure trends.
//
// Returns:
//   - int: Number of consecutive failures since last success
//
// Thread Safety:
//
//	Uses read lock for safe concurrent access
//
// Example:
//
//	failures := circuitBreaker.GetFailureCount()
//	if failures > cb.config.MaxFailures/2 {
//	    log.Warn("Circuit breaker approaching failure threshold")
//	}
func (cb *CircuitBreaker) GetFailureCount() int {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.failures
}

// Reset manually resets the circuit breaker to Closed state.
// This is useful for administrative recovery or testing scenarios.
// In production, circuits should recover automatically through normal success recording.
//
// Use cases:
//   - Manual recovery after fixing downstream service issues
//   - Testing circuit breaker behavior in development
//   - Administrative override during maintenance windows
//
// Thread Safety:
//
//	Uses write lock to safely reset circuit breaker state
//
// Example:
//
//	// After fixing the downstream service issue
//	circuitBreaker.Reset()
//	log.Info("Circuit breaker manually reset to closed state")
func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.state = StateClosed
	cb.failures = 0
	cb.lastFailTime = time.Time{} // Reset to zero time
}
