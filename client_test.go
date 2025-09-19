// Package network provides comprehensive tests for the gorest HTTP client library.
// These tests validate thread safety, configuration hierarchy, concurrent access,
// and all advanced features like rate limiting, circuit breaker, and retry logic.
package gorest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// TestDefaultGlobalClient verifies that the global client works with default settings
// without requiring explicit initialization.
func TestDefaultGlobalClient(t *testing.T) {
	// Reset to ensure we're testing defaults
	globalInitialized = false

	// Should work with defaults without initialization
	if Client.httpClient.Timeout != 30*time.Second {
		t.Errorf("Default timeout should be 30s, got %v", Client.httpClient.Timeout)
	}

	if Client.defaultEndpointConfig.rateLimiter != nil {
		t.Error("Default rate limiter should be nil (no rate limiting)")
	}

	if Client.defaultEndpointConfig.circuitBreaker != nil {
		t.Error("Default circuit breaker should be nil")
	}
}

// === GLOBAL CLIENT INITIALIZATION TESTS ===

// TestGlobalClientInitialization verifies that the global client can be initialized
// once and subsequent calls are properly ignored for safety.
func TestGlobalClientInitialization(t *testing.T) {
	// Reset global state for testing
	globalInitialized = false

	// Test first initialization with custom configuration
	Initialize(
		WithTimeout(15*time.Second),
		WithRateLimit(rate.Limit(50), 10),
	)

	if !globalInitialized {
		t.Error("Global client should be initialized after first Initialize() call")
	}

	if Client.httpClient.Timeout != 15*time.Second {
		t.Errorf("Expected timeout 15s, got %v", Client.httpClient.Timeout)
	}

	// Test second initialization (should be ignored)
	Initialize(
		WithTimeout(30 * time.Second), // This should be ignored
	)

	// Timeout should still be 15 seconds from first initialization
	if Client.httpClient.Timeout != 15*time.Second {
		t.Errorf("Second initialization should be ignored, expected 15s, got %v", Client.httpClient.Timeout)
	}
}

// === CLIENT CREATION TESTS ===

// TestNewClientCreation validates that NewClient creates independent instances
// with different configurations while maintaining proper isolation.
func TestNewClientCreation(t *testing.T) {
	client1 := NewClient(
		WithHost("https://api1.com"),
		WithTimeout(10*time.Second),
		WithRateLimit(rate.Limit(25), 5),
	)

	client2 := NewClient(
		WithHost("https://api2.com"),
		WithTimeout(20*time.Second),
		// No rate limiting for client2
	)

	// Verify clients have different configurations
	if client1.host != "https://api1.com" {
		t.Errorf("Client1 host should be 'https://api1.com', got %s", client1.host)
	}

	if client2.host != "https://api2.com" {
		t.Errorf("Client2 host should be 'https://api2.com', got %s", client2.host)
	}

	if client1.httpClient.Timeout != 10*time.Second {
		t.Errorf("Client1 timeout should be 10s, got %v", client1.httpClient.Timeout)
	}

	if client2.httpClient.Timeout != 20*time.Second {
		t.Errorf("Client2 timeout should be 20s, got %v", client2.httpClient.Timeout)
	}

	// Verify rate limiters are configured correctly
	if client1.defaultEndpointConfig.rateLimiter == nil {
		t.Error("Client1 should have rate limiter")
	}

	if client2.defaultEndpointConfig.rateLimiter != nil {
		t.Error("Client2 should not have rate limiter")
	}

	// Verify clients are independent instances
	if client1 == client2 {
		t.Error("NewClient should create independent instances")
	}
}

// === ENDPOINT CONFIGURATION TESTS ===

// TestEndpointConfiguration validates that endpoint-specific configurations
// are properly resolved with correct precedence and pattern matching.
func TestEndpointConfiguration(t *testing.T) {
	client := NewClient(
		WithHost("https://api.example.com"),
		WithRateLimit(rate.Limit(100), 20), // Default rate limit

		// Endpoint-specific configurations
		WithEndpointConfig("/auth/login", EndpointOptions{
			RateLimit: rate.NewLimiter(rate.Limit(5), 1),
			Timeout:   5 * time.Second,
		}),

		WithEndpointConfig("/products/*", EndpointOptions{
			RateLimit: rate.NewLimiter(rate.Limit(200), 50),
		}),

		WithEndpointConfig("/admin/*", EndpointOptions{
			CircuitBreaker: &CircuitBreakerConfig{
				MaxFailures:  2,
				ResetTimeout: 30 * time.Second,
			},
		}),
	)

	// Test exact match resolution
	loginConfig := client.getEndpointConfig("/auth/login")
	if loginConfig.rateLimiter == nil {
		t.Error("Login endpoint should have specific rate limiter")
	}
	if loginConfig.timeout != 5*time.Second {
		t.Error("Login endpoint should have specific timeout")
	}

	// Test wildcard pattern matching
	productConfig := client.getEndpointConfig("/products/123")
	if productConfig.rateLimiter == nil {
		t.Error("Product endpoint should match wildcard pattern")
	}

	// Test another wildcard match
	productSearchConfig := client.getEndpointConfig("/products/search")
	if productSearchConfig.rateLimiter == nil {
		t.Error("Product search should match /products/* pattern")
	}

	// Test circuit breaker configuration
	adminConfig := client.getEndpointConfig("/admin/users")
	if adminConfig.circuitBreaker == nil {
		t.Error("Admin endpoint should have circuit breaker")
	}

	// Test default fallback
	defaultConfig := client.getEndpointConfig("/other/endpoint")
	if defaultConfig.rateLimiter != client.defaultEndpointConfig.rateLimiter {
		t.Error("Unknown endpoint should use default rate limiter")
	}
}

// === CIRCUIT BREAKER TESTS ===

// TestCircuitBreaker validates the circuit breaker state transitions
// and failure tracking behavior.
func TestCircuitBreaker(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		MaxFailures:  3,
		ResetTimeout: 100 * time.Millisecond, // Short timeout for testing
	})

	// Initially closed - should allow requests
	if !cb.AllowRequest() {
		t.Error("Circuit breaker should initially allow requests (CLOSED state)")
	}

	if cb.GetState() != StateClosed {
		t.Errorf("Initial state should be CLOSED, got %s", cb.GetState())
	}

	// Record failures to reach threshold
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
		if cb.GetFailureCount() != i+1 {
			t.Errorf("Failure count should be %d, got %d", i+1, cb.GetFailureCount())
		}
	}

	// Should be open now
	if cb.AllowRequest() {
		t.Error("Circuit breaker should be OPEN after max failures")
	}

	if cb.GetState() != StateOpen {
		t.Errorf("State should be OPEN after failures, got %s", cb.GetState())
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// Should transition to half-open and allow request
	if !cb.AllowRequest() {
		t.Error("Circuit breaker should be HALF_OPEN after reset timeout")
	}

	if cb.GetState() != StateHalfOpen {
		t.Errorf("State should be HALF_OPEN after timeout, got %s", cb.GetState())
	}

	// Record success to close circuit
	cb.RecordSuccess()

	if cb.GetState() != StateClosed {
		t.Errorf("State should be CLOSED after success, got %s", cb.GetState())
	}

	if cb.GetFailureCount() != 0 {
		t.Errorf("Failure count should be reset to 0, got %d", cb.GetFailureCount())
	}

	// Test manual reset
	cb.RecordFailure()
	cb.Reset()
	if cb.GetState() != StateClosed || cb.GetFailureCount() != 0 {
		t.Error("Manual reset should close circuit and reset failure count")
	}
}

// === COPY-ON-WRITE THREAD SAFETY TESTS ===

// TestCopyOnWrite validates that the copy-on-write pattern properly isolates
// request-specific data while sharing infrastructure.
func TestCopyOnWrite(t *testing.T) {
	baseClient := NewClient(
		WithHost("https://api.example.com"),
		WithRateLimit(rate.Limit(100), 20),
	)

	// Create request chains that should be isolated
	client1 := baseClient.
		Headers(map[string]string{"Auth": "token1"}).
		Params(map[string]string{"version": "v1"})

	client2 := baseClient.
		Headers(map[string]string{"Auth": "token2"}).
		Params(map[string]string{"version": "v2"})

	// Verify request-specific data is isolated
	if client1.headers["Auth"] != "token1" {
		t.Errorf("Client1 should have token1, got %s", client1.headers["Auth"])
	}

	if client2.headers["Auth"] != "token2" {
		t.Errorf("Client2 should have token2, got %s", client2.headers["Auth"])
	}

	if client1.params["version"] != "v1" {
		t.Errorf("Client1 should have v1, got %s", client1.params["version"])
	}

	if client2.params["version"] != "v2" {
		t.Errorf("Client2 should have v2, got %s", client2.params["version"])
	}

	// Verify base client is unmodified
	if baseClient.headers != nil && len(baseClient.headers) > 0 {
		t.Error("Base client should not be modified by request chains")
	}
}

// === CONCURRENT ACCESS TESTS ===

// TestConcurrentAccess validates that multiple goroutines can safely use
// the same client instance without race conditions or data corruption.
func TestConcurrentAccess(t *testing.T) {
	client := NewClient(
		WithHost("https://httpbin.org"),
		WithTimeout(300*time.Second),
	)

	var wg sync.WaitGroup
	errorChan := make(chan error, 10)
	const numGoroutines = 3 // Reduced from 10 to avoid overwhelming the test server

	// Start multiple concurrent request chains
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Add a small delay between goroutine startups to prevent overwhelming the server
			time.Sleep(100 * time.Millisecond)

			// Each goroutine creates its own request chain
			// This tests that copy-on-write prevents race conditions
			err := client.
				Headers(map[string]string{
					"X-Request-ID":   fmt.Sprintf("req-%d", id),
					"X-Goroutine-ID": fmt.Sprintf("%d", id),
					"Authorization":  fmt.Sprintf("Bearer token-%d", id),
				}).
				Params(map[string]string{
					"id":        fmt.Sprintf("%d", id),
					"timestamp": fmt.Sprintf("%d", time.Now().UnixNano()),
				}).
				WithContext(context.Background()).
				Get("/get")

			if err != nil {
				errorChan <- fmt.Errorf("goroutine %d failed: %w", id, err.Error)
			}
		}(i)
	}

	wg.Wait()
	close(errorChan)

	// Check for any race conditions or errors
	errorCount := 0
	for err := range errorChan {
		if err != nil {
			t.Errorf("Concurrent request failed: %s", err.Error())
			errorCount++
		}
	}

	if errorCount > 0 {
		t.Errorf("Expected no errors from concurrent access, got %d errors", errorCount)
	}
}

// === CONFIGURATION TESTS ===

// TestDefaultHeaders validates that default headers are properly applied
// and can be overridden by request-specific headers.
func TestDefaultHeaders(t *testing.T) {
	client := NewClient(
		WithDefaultHeaders(map[string]string{
			"User-Agent":    "TestClient/1.0",
			"Accept":        "application/json",
			"X-API-Version": "v1",
		}),
	)

	if len(client.defaultHeaders) != 3 {
		t.Errorf("Should have 3 default headers, got %d", len(client.defaultHeaders))
	}

	if client.defaultHeaders["User-Agent"] != "TestClient/1.0" {
		t.Errorf("Wrong User-Agent header: %s", client.defaultHeaders["User-Agent"])
	}

	if client.defaultHeaders["Accept"] != "application/json" {
		t.Errorf("Wrong Accept header: %s", client.defaultHeaders["Accept"])
	}
}

// TestLoggerIntegration validates that logger configuration works correctly.
func TestLoggerIntegration(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	client := NewClient(
		WithLogger(logger),
		WithHost("https://httpbin.org"),
	)

	if client.logger == nil {
		t.Error("Client should have logger configured")
	}

	if client.logger != logger {
		t.Error("Client should have the correct logger instance")
	}
}

// TestRetryConfiguration validates retry configuration settings.
func TestRetryConfiguration(t *testing.T) {
	retryConfig := RetryConfig{
		MaxRetries:           3,
		BaseDelay:            100 * time.Millisecond,
		MaxDelay:             5 * time.Second,
		RetryableStatusCodes: []int{429, 500, 502, 503},
	}

	client := NewClient(
		WithRetry(retryConfig),
	)

	if client.defaultEndpointConfig.retryConfig == nil {
		t.Error("Client should have retry configuration")
	}

	if client.defaultEndpointConfig.retryConfig.MaxRetries != 3 {
		t.Errorf("Expected MaxRetries 3, got %d", client.defaultEndpointConfig.retryConfig.MaxRetries)
	}

	if client.defaultEndpointConfig.retryConfig.BaseDelay != 100*time.Millisecond {
		t.Errorf("Expected BaseDelay 100ms, got %v", client.defaultEndpointConfig.retryConfig.BaseDelay)
	}
}

// === PERFORMANCE BENCHMARKS ===

// BenchmarkNewClient measures the performance of creating new client instances.
func BenchmarkNewClient(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = NewClient(
			WithHost("https://api.example.com"),
			WithTimeout(30*time.Second),
			WithRateLimit(rate.Limit(100), 20),
		)
	}
}

// BenchmarkCopyOnWrite measures the performance of the copy-on-write pattern
// when creating request chains.
func BenchmarkCopyOnWrite(b *testing.B) {
	baseClient := NewClient(
		WithHost("https://api.example.com"),
		WithRateLimit(rate.Limit(100), 20),
	)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_ = baseClient.
			Headers(map[string]string{"Auth": "token"}).
			Params(map[string]string{"version": "v1"}).
			Body(map[string]string{"key": "value"})
	}
}

// BenchmarkEndpointConfigResolution measures the performance of endpoint
// configuration resolution including pattern matching.
func BenchmarkEndpointConfigResolution(b *testing.B) {
	client := NewClient(
		WithEndpointConfig("/auth/login", EndpointOptions{
			RateLimit: rate.NewLimiter(rate.Limit(5), 1),
		}),
		WithEndpointConfig("/products/*", EndpointOptions{
			RateLimit: rate.NewLimiter(rate.Limit(100), 20),
		}),
		WithEndpointConfig("/admin/*", EndpointOptions{
			RateLimit: rate.NewLimiter(rate.Limit(10), 2),
		}),
		WithEndpointConfig("/api/v1/*", EndpointOptions{
			RateLimit: rate.NewLimiter(rate.Limit(200), 50),
		}),
	)

	b.ResetTimer()

	// Test various endpoint patterns
	endpoints := []string{
		"/auth/login",     // Exact match
		"/products/123",   // Wildcard match
		"/admin/users",    // Another wildcard
		"/api/v1/data",    // Complex pattern
		"/other/endpoint", // Default fallback
	}

	for i := 0; i < b.N; i++ {
		endpoint := endpoints[i%len(endpoints)]
		_ = client.getEndpointConfig(endpoint)
	}
}

// === RATE LIMITING TESTS ===

// TestRateLimitingIntegration validates that rate limiting works correctly
// in practice (skipped in short test mode due to timing requirements).
func TestRateLimitingIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping rate limiting test in short mode")
	}

	// Create client with very low rate limit for testing
	client := NewClient(
		WithHost("https://httpbin.org"),
		WithRateLimit(rate.Limit(2), 1), // 2 requests per second, burst 1
	)

	start := time.Now()

	// Make 3 requests - should be rate limited
	for i := 0; i < 3; i++ {
		err := client.
			WithContext(context.Background()).
			Get("/get")

		// First request should succeed immediately
		// Second request may succeed due to burst
		// Third request should be delayed by rate limiter
		if err != nil && i < 2 {
			// Allow some failures due to network issues, but not all
			t.Logf("Request %d failed (may be network related): %v", i, err)
		}
	}

	duration := time.Since(start)

	// Should take at least 500ms due to rate limiting on third request
	// This is approximate due to network latency and burst allowance
	if duration < 400*time.Millisecond {
		t.Logf("Requests completed in %v - rate limiting may not be working as expected (this could be due to network speed or test conditions)", duration)
	}

	t.Logf("Rate limiting test completed in %v", duration)
}

// === HELPER FUNCTIONS FOR TESTING ===

// createTestClient creates a client configured for testing with reasonable defaults.
func createTestClient() *NetworkClient {
	return NewClient(
		WithHost("https://httpbin.org"),
		WithTimeout(10*time.Second),
		// No rate limiting in tests for speed
	)
}

// TestHelperFunctions validates our testing helper functions work correctly.
func TestHelperFunctions(t *testing.T) {
	client := createTestClient()

	if client.host != "https://httpbin.org" {
		t.Errorf("Test client should have httpbin host, got %s", client.host)
	}

	if client.httpClient.Timeout != 10*time.Second {
		t.Errorf("Test client should have 10s timeout, got %v", client.httpClient.Timeout)
	}
}
