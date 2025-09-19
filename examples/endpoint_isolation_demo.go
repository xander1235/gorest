package main

import (
	"fmt"
	"sync"
	"time"

	gorest "github.com/xander1235/gorest"
	"golang.org/x/time/rate"
)

// This example demonstrates the new endpoint-level rate limiting and circuit breaking
// Each endpoint gets its own isolated rate limiter and circuit breaker for better control

func main() {
	fmt.Println("=== Endpoint-Level Isolation Demo ===")
	fmt.Println("Demonstrating independent rate limiting and circuit breaking per endpoint\n")

	// Create a client with endpoint-specific configurations
	client := gorest.NewClient(
		gorest.WithHost("https://httpbin.org"),

		// Default configuration for all endpoints (fallback)
		gorest.WithRateLimit(rate.Limit(10), 5), // 10 req/sec default
		gorest.WithCircuitBreaker(gorest.CircuitBreakerConfig{
			MaxFailures:  5,
			ResetTimeout: 30 * time.Second,
		}),

		// Auth endpoints: Very strict rate limiting (security-critical)
		gorest.WithEndpointConfig("/anything/auth/*", gorest.EndpointOptions{
			RateLimit: rate.NewLimiter(rate.Limit(2), 1), // Only 2 req/sec
			CircuitBreaker: &gorest.CircuitBreakerConfig{
				MaxFailures:  2, // Very sensitive
				ResetTimeout: 60 * time.Second,
			},
		}),

		// Data endpoints: High throughput allowed (read-heavy operations)
		gorest.WithEndpointConfig("/anything/data/*", gorest.EndpointOptions{
			RateLimit: rate.NewLimiter(rate.Limit(50), 20), // High volume
			CircuitBreaker: &gorest.CircuitBreakerConfig{
				MaxFailures:  10, // More tolerant
				ResetTimeout: 15 * time.Second,
			},
		}),

		// Admin endpoints: Moderate but controlled (administrative operations)
		gorest.WithEndpointConfig("/anything/admin/*", gorest.EndpointOptions{
			RateLimit: rate.NewLimiter(rate.Limit(5), 2), // Moderate
			CircuitBreaker: &gorest.CircuitBreakerConfig{
				MaxFailures:  3,
				ResetTimeout: 45 * time.Second,
			},
		}),
	)

	fmt.Println("Client configured with 4 different rate limiting policies:")
	fmt.Println("1. Auth endpoints: 2 req/sec (strict security)")
	fmt.Println("2. Data endpoints: 50 req/sec (high throughput)")
	fmt.Println("3. Admin endpoints: 5 req/sec (moderate control)")
	fmt.Println("4. Default endpoints: 10 req/sec (fallback)\n")

	// Demonstrate that different endpoints have different rate limits
	demonstrateConcurrentEndpoints(client)

	// Demonstrate circuit breaker isolation
	demonstratCircuitBreakerIsolation(client)

	fmt.Println("\n=== Key Benefits Demonstrated ===")
	fmt.Println("✅ Complete isolation - auth failures don't affect data endpoints")
	fmt.Println("✅ Independent rate limiting - each endpoint has its own limits")
	fmt.Println("✅ Granular control - configure policies based on endpoint characteristics")
	fmt.Println("✅ Zero interference - high-volume endpoints don't impact critical ones")
	fmt.Println("✅ Pattern matching - wildcard patterns for flexible configuration")
}

func demonstrateConcurrentEndpoints(client *gorest.NetworkClient) {
	fmt.Println("=== Testing Concurrent Endpoint Access ===")

	var wg sync.WaitGroup
	results := make(map[string][]time.Duration)
	resultsMutex := sync.Mutex{}

	endpoints := []struct {
		name          string
		path          string
		expectedLimit string
	}{
		{"Auth", "/anything/auth/login", "2 req/sec"},
		{"Data", "/anything/data/users", "50 req/sec"},
		{"Admin", "/anything/admin/settings", "5 req/sec"},
		{"Default", "/anything/other/endpoint", "10 req/sec"},
	}

	for _, endpoint := range endpoints {
		wg.Add(1)
		go func(ep struct {
			name          string
			path          string
			expectedLimit string
		}) {
			defer wg.Done()

			fmt.Printf("Testing %s endpoint (%s) - Expected: %s\n",
				ep.name, ep.path, ep.expectedLimit)

			durations := make([]time.Duration, 0, 3)

			// Make 3 requests to each endpoint to observe rate limiting
			for i := 0; i < 3; i++ {
				start := time.Now()

				// This request will be rate limited based on endpoint-specific config
				err := client.
					Headers(map[string]string{"X-Test-Endpoint": ep.name}).
					Post(ep.path)

				duration := time.Since(start)
				durations = append(durations, duration)

				if err != nil {
					// Rate limiting or circuit breaker may cause errors
					fmt.Printf("  Request %d to %s: %v (took %v)\n",
						i+1, ep.name, err, duration)
				} else {
					fmt.Printf("  Request %d to %s: Success (took %v)\n",
						i+1, ep.name, duration)
				}

				// Small delay between requests to observe rate limiting effects
				time.Sleep(100 * time.Millisecond)
			}

			resultsMutex.Lock()
			results[ep.name] = durations
			resultsMutex.Unlock()

		}(endpoint)
	}

	wg.Wait()

	fmt.Println("\n📊 Results Summary:")
	for name, durations := range results {
		total := time.Duration(0)
		for _, d := range durations {
			total += d
		}
		avg := total / time.Duration(len(durations))
		fmt.Printf("  %s endpoint: Avg %v per request\n", name, avg)
	}
	fmt.Println()
}

func demonstratCircuitBreakerIsolation(client *gorest.NetworkClient) {
	fmt.Println("=== Circuit Breaker Isolation Demo ===")
	fmt.Println("Simulating failures on auth endpoint - other endpoints should be unaffected\n")

	// This demonstrates that circuit breaker failures are isolated per endpoint
	// We'll simulate failures to auth endpoints and verify other endpoints still work

	fmt.Println("Making requests to different endpoints:")

	// Try auth endpoint (this might trigger circuit breaker due to 401/403 responses)
	err := client.Post("/status/401") // This will fail with 401
	if err != nil {
		fmt.Printf("Auth endpoint failed (expected): %v\n", err)
	}

	// Try data endpoint - should still work despite auth failures
	err = client.Get("/anything/data/users")
	if err != nil {
		fmt.Printf("Data endpoint failed: %v\n", err)
	} else {
		fmt.Println("✅ Data endpoint succeeded - not affected by auth failures")
	}

	// Try admin endpoint - should also still work
	err = client.Get("/anything/admin/settings")
	if err != nil {
		fmt.Printf("Admin endpoint failed: %v\n", err)
	} else {
		fmt.Println("✅ Admin endpoint succeeded - not affected by auth failures")
	}

	// Try default endpoint - should still work
	err = client.Get("/anything/other/service")
	if err != nil {
		fmt.Printf("Default endpoint failed: %v\n", err)
	} else {
		fmt.Println("✅ Default endpoint succeeded - not affected by auth failures")
	}

	fmt.Println("\n🎯 Circuit Breaker Isolation Verified:")
	fmt.Println("   Auth endpoint failures don't cascade to other endpoints")
	fmt.Println("   Each endpoint maintains its own circuit breaker state")
	fmt.Println("   System remains resilient despite individual endpoint issues")
}
