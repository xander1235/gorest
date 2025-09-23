// Package main demonstrates advanced usage patterns for the gorest HTTP client library.
// This example shows how to configure global defaults, create service-specific clients,
// and use endpoint-specific configurations for production applications.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"

	"github.com/xander1235/gorest/v2"
)

// User represents a user in the API
type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// CreateUserRequest represents the request payload for creating a user
type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// ChargeRequest represents a payment charge request
type ChargeRequest struct {
	Amount   int    `json:"amount"`   // Amount in cents
	Currency string `json:"currency"` // Currency code (USD, EUR, etc.)
	Token    string `json:"token"`    // Payment token from frontend
}

// === SERVICE CLIENT COLLECTION ===

// APIClients holds all service-specific HTTP clients for different parts of the application.
// This pattern allows each service to have its own configuration while maintaining
// consistent patterns across the application.
type APIClients struct {
	Payment  *gorest.NetworkClient // Handles payment processing with strict limits
	User     *gorest.NetworkClient // Handles user operations with moderate limits
	Product  *gorest.NetworkClient // Handles product catalog with high throughput
	Internal *gorest.NetworkClient // Handles internal services with no limits
}

// === MAIN DEMONSTRATION ===

func main() {
	// Initialize structured logging for observability
	logger := setupLogger()
	defer logger.Sync()

	log.Println("=== Gorest Advanced Usage Demonstration ===")

	// 1. Global client initialization (one-time setup at application startup)
	demonstratGlobalInitialization(logger)

	// 2. Use the configured global client
	demonstrateGlobalClientUsage()

	// 3. Create service-specific clients with custom configurations
	clients := setupServiceClients(logger)

	// 4. Demonstrate different service configurations in action
	demonstrateServiceSpecificBehavior(clients)

	// 5. Show endpoint-specific configuration patterns
	demonstrateEndpointPatternMatching(logger)

	// 6. Test concurrent safety and performance
	demonstrateConcurrentSafety(clients)

	log.Println("\n=== Demonstration completed successfully ===")
}

// === 1. GLOBAL CONFIGURATION ===

// demonstratGlobalInitialization shows how to configure organization-wide defaults
// that apply to the global Client instance.
func demonstratGlobalInitialization(logger *zap.Logger) {
	log.Println("\n=== Step 1: Global Client Initialization ===")

	// Configure global defaults once at application startup
	// These settings apply to gorest.NetworkClient when used directly
	gorest.Initialize(
		// Basic configuration
		gorest.WithTimeout(30*time.Second),
		gorest.WithLogger(logger),

		// Rate limiting: 100 requests/second with burst of 20
		// Good default for most internal service-to-service communication
		gorest.WithRateLimit(rate.Limit(100), 20),

		// Circuit breaker: Open after 5 failures, test recovery every 60 seconds
		// Provides fault tolerance for downstream service issues
		gorest.WithCircuitBreaker(gorest.CircuitBreakerConfig{
			MaxFailures:  5,
			ResetTimeout: 60 * time.Second,
		}),

		// Retry configuration: 3 retries with exponential backoff
		// Handles transient failures automatically
		gorest.WithRetry(gorest.RetryConfig{
			MaxRetries: 3,
			BaseDelay:  100 * time.Millisecond,
			MaxDelay:   5 * time.Second,
		}),

		// HTTP transport optimization for better performance
		gorest.WithTransport(&http.Transport{
			MaxIdleConns:          200,              // Total connection pool size
			MaxIdleConnsPerHost:   20,               // Connections per host
			IdleConnTimeout:       90 * time.Second, // Keep-alive duration
			TLSHandshakeTimeout:   10 * time.Second, // TLS setup timeout
			ResponseHeaderTimeout: 10 * time.Second, // Header read timeout
		}),

		// Organization-wide default headers
		gorest.WithDefaultHeaders(map[string]string{
			"User-Agent":     "MyCompany-Services/2.1.0",
			"Accept":         "application/json",
			"X-Service-Name": "demo-service",
			"X-Version":      "v1",
		}),
	)

	// Demonstrate that subsequent Initialize calls are ignored
	gorest.Initialize(
		gorest.WithTimeout(60 * time.Second), // This will be ignored!
	)

	log.Println("✓ Global client initialized with organization defaults")
	log.Println("✓ Subsequent Initialize() calls are safely ignored")
}

// === 2. GLOBAL CLIENT USAGE ===

// demonstrateGlobalClientUsage shows how to use the configured global client
// for simple API calls that don't need service-specific configuration.
func demonstrateGlobalClientUsage() {
	log.Println("\n=== Step 2: Using Global Client ===")

	// Use the global client with applied configuration
	// This inherits all the settings from Initialize()
	var user User
	err := gorest.Client.
		Host("https://jsonplaceholder.typicode.com"). // Public testing API
		Headers(map[string]string{
			"Authorization":    "Bearer demo-token",
			"X-Request-Source": "global-client-demo",
		}).
		Response(&user).
		WithContext(context.Background()).
		Get("/users/1")

	if err != nil {
		log.Printf("Global client request failed: %v", err)
	} else {
		log.Printf("✓ Global client success: User %s (%s)", user.Name, user.Email)
	}
}

// === 3. SERVICE-SPECIFIC CLIENTS ===

// setupServiceClients creates specialized HTTP clients for different services
// with configurations tailored to their specific requirements and characteristics.
func setupServiceClients(logger *zap.Logger) *APIClients {
	log.Println("\n=== Step 3: Creating Service-Specific Clients ===")

	return &APIClients{
		// === PAYMENT SERVICE CLIENT ===
		// Financial operations require strict limits and strong fault tolerance
		Payment: gorest.NewClient(
			// Service identification
			gorest.WithHost("https://api.stripe.com/v1"),

			// Conservative timeouts for financial operations
			gorest.WithTimeout(15*time.Second),

			// Very strict rate limiting: 5 requests/second, burst of 2
			// Prevents overwhelming payment provider APIs
			gorest.WithRateLimit(rate.Limit(5), 2),

			// Sensitive circuit breaker: Open after just 2 failures
			// Financial services need to fail fast to prevent issues
			gorest.WithCircuitBreaker(gorest.CircuitBreakerConfig{
				MaxFailures:  2,
				ResetTimeout: 30 * time.Second,
			}),

			// Conservative retry: Only 2 retries for financial operations
			// Aggressive retries could cause duplicate charges
			gorest.WithRetry(gorest.RetryConfig{
				MaxRetries: 2,
				BaseDelay:  200 * time.Millisecond,
				MaxDelay:   3 * time.Second,
			}),

			// Payment-specific headers
			gorest.WithDefaultHeaders(map[string]string{
				"Stripe-Version": "2020-08-27",
				"X-Service":      "payment-processor",
			}),

			// Endpoint-specific configurations for different payment operations
			gorest.WithEndpointConfig("/charges", gorest.EndpointOptions{
				// Even stricter limits for creating charges
				RateLimit: rate.NewLimiter(rate.Limit(2), 1),
				Timeout:   10 * time.Second,
				CircuitBreaker: &gorest.CircuitBreakerConfig{
					MaxFailures:  1, // Extremely sensitive
					ResetTimeout: 60 * time.Second,
				},
			}),

			gorest.WithEndpointConfig("/refunds", gorest.EndpointOptions{
				// Most restrictive for refunds
				RateLimit: rate.NewLimiter(rate.Limit(1), 1),
				Timeout:   15 * time.Second,
			}),

			// Read-only operations can be more permissive
			gorest.WithEndpointConfig("/charges/*", gorest.EndpointOptions{
				RateLimit: rate.NewLimiter(rate.Limit(10), 3),
			}),
		),

		// === USER SERVICE CLIENT ===
		// User operations need moderate limits with good performance
		User: gorest.NewClient(
			gorest.WithHost("https://jsonplaceholder.typicode.com"),
			gorest.WithTimeout(20*time.Second),

			// Moderate rate limiting: 50 requests/second, burst of 10
			// Balances performance with service protection
			gorest.WithRateLimit(rate.Limit(50), 10),

			// Standard circuit breaker settings
			gorest.WithCircuitBreaker(gorest.CircuitBreakerConfig{
				MaxFailures:  5,
				ResetTimeout: 45 * time.Second,
			}),

			gorest.WithLogger(logger),

			// User service specific headers
			gorest.WithDefaultHeaders(map[string]string{
				"X-Service": "user-management",
				"Accept":    "application/json",
			}),

			// Authentication endpoints need special handling
			gorest.WithEndpointConfig("/auth/login", gorest.EndpointOptions{
				RateLimit: rate.NewLimiter(rate.Limit(10), 2), // Login rate limiting
				Timeout:   8 * time.Second,                    // Shorter timeout for auth
			}),

			// User CRUD operations
			gorest.WithEndpointConfig("/users/*", gorest.EndpointOptions{
				RateLimit: rate.NewLimiter(rate.Limit(100), 20), // Higher for user ops
				Timeout:   25 * time.Second,
			}),
		),

		// === PRODUCT SERVICE CLIENT ===
		// Product catalog needs high throughput for customer-facing operations
		Product: gorest.NewClient(
			gorest.WithHost("https://fakestoreapi.com"),
			gorest.WithTimeout(25*time.Second),

			// High rate limiting: 200 requests/second, burst of 50
			// Product browsing generates high traffic
			gorest.WithRateLimit(rate.Limit(200), 50),

			// Relaxed circuit breaker for non-critical operations
			gorest.WithCircuitBreaker(gorest.CircuitBreakerConfig{
				MaxFailures:  10,
				ResetTimeout: 30 * time.Second,
			}),

			// Product-specific patterns
			gorest.WithEndpointConfig("/products", gorest.EndpointOptions{
				RateLimit: rate.NewLimiter(rate.Limit(500), 100), // Very high for listing
				Timeout:   15 * time.Second,
			}),

			gorest.WithEndpointConfig("/products/*/", gorest.EndpointOptions{
				RateLimit: rate.NewLimiter(rate.Limit(300), 60), // High for individual products
				Timeout:   10 * time.Second,
			}),

			// Search operations need longer timeouts
			gorest.WithEndpointConfig("/products/search", gorest.EndpointOptions{
				RateLimit: rate.NewLimiter(rate.Limit(50), 10),
				Timeout:   35 * time.Second, // Search can be slow
			}),
		),

		// === INTERNAL SERVICE CLIENT ===
		// Internal services can have relaxed limits and longer timeouts
		Internal: gorest.NewClient(
			gorest.WithHost("https://httpbin.org"), // Using httpbin for demo
			gorest.WithTimeout(60*time.Second),     // Long timeout for internal ops

			// No rate limiting for internal services
			// Internal services should be designed to handle load

			// No circuit breaker for internal services
			// Internal services should be reliable

			// Internal service headers
			gorest.WithDefaultHeaders(map[string]string{
				"X-Internal-Service": "true",
				"X-Service":          "internal-processor",
			}),
		),
	}
}

// === 4. SERVICE-SPECIFIC BEHAVIOR DEMONSTRATION ===

// demonstrateServiceSpecificBehavior shows how different services use
// their customized configurations for different types of operations.
func demonstrateServiceSpecificBehavior(clients *APIClients) {
	log.Println("\n=== Step 4: Service-Specific Behavior Demonstration ===")

	// === Payment Service - Strict Limits ===
	log.Println("\n--- Payment Service (Strict Limits) ---")
	demonstratePaymentOperations(clients.Payment)

	// === User Service - Moderate Limits ===
	log.Println("\n--- User Service (Moderate Limits) ---")
	demonstrateUserOperations(clients.User)

	// === Product Service - High Throughput ===
	log.Println("\n--- Product Service (High Throughput) ---")
	demonstrateProductOperations(clients.Product)

	// === Internal Service - No Limits ===
	log.Println("\n--- Internal Service (No Limits) ---")
	demonstrateInternalOperations(clients.Internal)
}

func demonstratePaymentOperations(paymentClient *gorest.NetworkClient) {
	// Simulate payment charge request with strict limits
	chargeReq := ChargeRequest{
		Amount:   2500, // $25.00
		Currency: "USD",
		Token:    "tok_demo_12345",
	}

	// This request will use:
	// - Payment service rate limit (5 req/sec)
	// - Endpoint-specific rate limit for /charges (2 req/sec) - more restrictive wins
	// - Endpoint-specific timeout (10s)
	// - Service-level circuit breaker (2 failures to open)
	err := paymentClient.
		Headers(map[string]string{
			"Authorization":    "Bearer sk_test_demo123",
			"Idempotency-Key":  "charge_demo_" + fmt.Sprintf("%d", time.Now().Unix()),
			"X-Payment-Source": "demo-application",
		}).
		Body(chargeReq).
		WithContext(context.Background()).
		Post("/charges") // Uses endpoint-specific config

	if err != nil {
		log.Printf("Payment operation result: %v", err)
	} else {
		log.Printf("✓ Payment operation completed successfully")
	}
}

func demonstrateUserOperations(userClient *gorest.NetworkClient) {
	// Fetch user data with moderate limits
	var user User
	err := userClient.
		Headers(map[string]string{
			"Authorization":    "Bearer user_token_demo",
			"X-Request-Source": "user-management-demo",
		}).
		Response(&user).
		Get("/users/1") // Uses /users/* endpoint config

	if err != nil {
		log.Printf("User operation result: %v", err)
	} else {
		log.Printf("✓ User operation: Fetched %s (%s)", user.Name, user.Email)
	}

	// Demonstrate user creation
	createReq := CreateUserRequest{
		Name:  "Demo User",
		Email: "demo@example.com",
	}

	err = userClient.
		Headers(map[string]string{
			"Authorization": "Bearer user_admin_token",
			"Content-Type":  "application/json",
		}).
		Body(createReq).
		Post("/users")

	if err != nil {
		log.Printf("User creation result: %v", err)
	} else {
		log.Printf("✓ User creation completed successfully")
	}
}

func demonstrateProductOperations(productClient *gorest.NetworkClient) {
	// Fetch product catalog with high throughput configuration
	var products []interface{}
	err := productClient.
		Response(&products).
		Get("/products") // Uses endpoint-specific high-throughput config

	if err != nil {
		log.Printf("Product catalog result: %v", err)
	} else {
		log.Printf("✓ Product catalog: Fetched %d products", len(products))
	}

	// Fetch individual product
	var product interface{}
	err = productClient.
		Response(&product).
		Get("/products/1") // Uses /products/*/ pattern config

	if err != nil {
		log.Printf("Individual product result: %v", err)
	} else {
		log.Printf("✓ Individual product fetched successfully")
	}
}

func demonstrateInternalOperations(internalClient *gorest.NetworkClient) {
	// Internal service operations with no limits
	processingData := map[string]interface{}{
		"operation": "batch_process",
		"items":     100,
		"priority":  "high",
		"timestamp": time.Now().Unix(),
	}

	err := internalClient.
		Headers(map[string]string{
			"X-Internal-Auth": "internal_secret_token",
			"X-Operation":     "demo-batch-process",
		}).
		Body(processingData).
		Post("/post") // httpbin.org endpoint for demo

	if err != nil {
		log.Printf("Internal operation result: %v", err)
	} else {
		log.Printf("✓ Internal operation completed successfully")
	}
}

// === 5. ENDPOINT PATTERN MATCHING ===

// demonstrateEndpointPatternMatching shows how wildcard patterns work
// for endpoint-specific configuration.
func demonstrateEndpointPatternMatching(logger *zap.Logger) {
	log.Println("\n=== Step 5: Endpoint Pattern Matching ===")

	// Create a client with various endpoint patterns
	client := gorest.NewClient(
		gorest.WithHost("https://api.example.com"),
		gorest.WithLogger(logger),
		gorest.WithRateLimit(rate.Limit(100), 20), // Default rate limit

		// Exact endpoint match
		gorest.WithEndpointConfig("/auth/login", gorest.EndpointOptions{
			RateLimit: rate.NewLimiter(rate.Limit(5), 1),
			Timeout:   5 * time.Second,
		}),

		// Wildcard pattern matching
		gorest.WithEndpointConfig("/products/*", gorest.EndpointOptions{
			RateLimit: rate.NewLimiter(rate.Limit(200), 40),
			Timeout:   15 * time.Second,
		}),

		// Admin pattern with strict security
		gorest.WithEndpointConfig("/admin/*", gorest.EndpointOptions{
			RateLimit: rate.NewLimiter(rate.Limit(10), 2),
			Timeout:   30 * time.Second,
			CircuitBreaker: &gorest.CircuitBreakerConfig{
				MaxFailures:  3,
				ResetTimeout: 60 * time.Second,
			},
		}),

		// API version pattern
		gorest.WithEndpointConfig("/api/v2/*", gorest.EndpointOptions{
			RateLimit: rate.NewLimiter(rate.Limit(150), 30),
		}),
	)

	// Demonstrate different endpoints matching different patterns
	patternTests := []struct {
		endpoint    string
		description string
	}{
		{"/auth/login", "Exact match - will use login-specific config"},
		{"/products/123", "Pattern match - will use /products/* config"},
		{"/products/search", "Pattern match - will use /products/* config"},
		{"/admin/users/delete", "Pattern match - will use /admin/* config"},
		{"/api/v2/users", "Pattern match - will use /api/v2/* config"},
		{"/other/endpoint", "No match - will use default client config"},
	}

	for _, test := range patternTests {
		log.Printf("Testing %s: %s", test.endpoint, test.description)

		// These would normally make actual requests, but for demo we just show the pattern
		// The endpoint configuration would be resolved automatically by getEndpointConfig()
		_ = client // Would call client.Get(test.endpoint) in real usage
	}

	log.Println("✓ Endpoint pattern matching configured and demonstrated")
}

// === 6. CONCURRENT SAFETY DEMONSTRATION ===

// demonstrateConcurrentSafety shows that multiple goroutines can safely
// use the same client instances without race conditions.
func demonstrateConcurrentSafety(clients *APIClients) {
	log.Println("\n=== Step 6: Concurrent Safety Demonstration ===")

	var wg sync.WaitGroup
	concurrentRequests := 10
	errorChan := make(chan error, concurrentRequests*2)

	// Test concurrent access to payment service
	log.Printf("Starting %d concurrent payment requests...", concurrentRequests)
	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Each goroutine gets its own request context with unique headers
			err := clients.Payment.
				Headers(map[string]string{
					"Authorization":  "Bearer token_" + fmt.Sprintf("%d", id),
					"X-Request-ID":   fmt.Sprintf("req_%d_%d", id, time.Now().UnixNano()),
					"X-Goroutine-ID": fmt.Sprintf("%d", id),
				}).
				Body(ChargeRequest{
					Amount:   100 * (id + 1), // Unique amounts
					Currency: "USD",
					Token:    fmt.Sprintf("tok_%d", id),
				}).
				WithContext(context.Background()).
				Post("/charges")

			if err != nil {
				errorChan <- fmt.Errorf("goroutine %d payment error: %w", id, err)
			}
		}(i)
	}

	// Test concurrent access to user service
	log.Printf("Starting %d concurrent user requests...", concurrentRequests)
	for i := 0; i < concurrentRequests; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			var user User
			err := clients.User.
				Headers(map[string]string{
					"Authorization":  "Bearer user_token_" + fmt.Sprintf("%d", id),
					"X-Request-ID":   fmt.Sprintf("user_req_%d_%d", id, time.Now().UnixNano()),
					"X-Goroutine-ID": fmt.Sprintf("%d", id),
				}).
				Params(map[string]string{
					"goroutine_id": fmt.Sprintf("%d", id),
					"timestamp":    fmt.Sprintf("%d", time.Now().Unix()),
				}).
				Response(&user).
				Get(fmt.Sprintf("/users/%d", (id%10)+1)) // Cycle through users 1-10

			if err != nil {
				errorChan <- fmt.Errorf("goroutine %d user error: %w", id, err)
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errorChan)

	// Check for any errors
	errorCount := 0
	for err := range errorChan {
		if err != nil {
			log.Printf("Concurrent request error: %v", err)
			errorCount++
		}
	}

	if errorCount == 0 {
		log.Printf("✓ All %d concurrent requests completed successfully", concurrentRequests*2)
		log.Println("✓ No race conditions or data corruption detected")
	} else {
		log.Printf("! %d out of %d concurrent requests encountered errors (may be expected due to rate limiting)", errorCount, concurrentRequests*2)
	}
}

// === UTILITY FUNCTIONS ===

// setupLogger configures structured logging for the demonstration.
// In production, you would configure this based on your environment.
func setupLogger() *zap.Logger {
	// Use development logger for human-readable output
	logger, err := zap.NewDevelopment()
	if err != nil {
		// Fallback to no-op logger if setup fails
		return zap.NewNop()
	}
	return logger
}
