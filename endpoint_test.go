package gorest

import (
	"testing"
	"golang.org/x/time/rate"
	"time"
)

func TestEndpointLevelConfiguration(t *testing.T) {
	// Test that different endpoints get different rate limiters and circuit breakers
	client := NewClient(
		WithHost("https://api.example.com"),
		
		// Default configuration for all endpoints
		WithRateLimit(rate.Limit(100), 20),
		WithCircuitBreaker(CircuitBreakerConfig{
			MaxFailures:  5,
			ResetTimeout: 30 * time.Second,
		}),
		
		// Endpoint-specific overrides
		WithEndpointConfig("/auth/login", EndpointOptions{
			RateLimit: rate.NewLimiter(rate.Limit(5), 1), // Stricter for login
		}),
		
		WithEndpointConfig("/products/*", EndpointOptions{
			RateLimit: rate.NewLimiter(rate.Limit(200), 50), // Higher for products
		}),
	)
	
	// Test that different endpoints get different configurations
	loginConfig := client.getEndpointConfig("/auth/login")
	productsConfig := client.getEndpointConfig("/products/123")
	defaultConfig := client.getEndpointConfig("/users/profile")
	
	// Each endpoint should have its own rate limiter instance
	if loginConfig.rateLimiter == productsConfig.rateLimiter {
		t.Error("Login and products endpoints should have different rate limiters")
	}
	
	if loginConfig.rateLimiter == defaultConfig.rateLimiter {
		t.Error("Login and default endpoints should have different rate limiters")
	}
	
	// Default config should be used for endpoints without specific config
	if defaultConfig != client.defaultEndpointConfig {
		t.Error("Unspecified endpoints should use default endpoint config")
	}
	
	// Verify default endpoint config has the global settings
	if client.defaultEndpointConfig.rateLimiter == nil {
		t.Error("Default endpoint config should have rate limiter from WithRateLimit")
	}
	
	if client.defaultEndpointConfig.circuitBreaker == nil {
		t.Error("Default endpoint config should have circuit breaker from WithCircuitBreaker")
	}
}

func TestEndpointIsolation(t *testing.T) {
	// Test that circuit breaker state is isolated per endpoint
	client := NewClient(
		WithHost("https://api.example.com"),
		WithEndpointConfig("/payment/*", EndpointOptions{
			CircuitBreaker: &CircuitBreakerConfig{
				MaxFailures:  2, // Very sensitive
				ResetTimeout: 60 * time.Second,
			},
		}),
		WithEndpointConfig("/logs/*", EndpointOptions{
			CircuitBreaker: &CircuitBreakerConfig{
				MaxFailures:  10, // Less sensitive
				ResetTimeout: 30 * time.Second,
			},
		}),
	)
	
	paymentConfig := client.getEndpointConfig("/payment/charge")
	logsConfig := client.getEndpointConfig("/logs/upload")
	
	// Different endpoints should have different circuit breaker instances
	if paymentConfig.circuitBreaker == logsConfig.circuitBreaker {
		t.Error("Payment and logs endpoints should have separate circuit breakers")
	}
	
	// Simulate failures on payment endpoint
	paymentConfig.circuitBreaker.RecordFailure()
	paymentConfig.circuitBreaker.RecordFailure()
	
	// Payment circuit should be open, logs circuit should still be closed
	if paymentConfig.circuitBreaker.GetState() != StateOpen {
		t.Error("Payment circuit breaker should be open after failures")
	}
	
	if logsConfig.circuitBreaker.GetState() != StateClosed {
		t.Error("Logs circuit breaker should still be closed")
	}
}
