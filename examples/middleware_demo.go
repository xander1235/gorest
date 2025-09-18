package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"go.uber.org/zap"
	gorest "github.com/xander1235/gorest"
)

// This example demonstrates the comprehensive middleware and interceptor system
// in gorest, showing how to add cross-cutting concerns like authentication,
// logging, metrics, and custom processing to HTTP requests.

func main() {
	fmt.Println("=== Gorest Middleware & Interceptors Demo ===")
	fmt.Println("Demonstrating authentication, logging, metrics, and custom processing\n")

	// Setup logger for demonstration
	logger := setupLogger()

	// Demonstrate different middleware approaches
	demonstrateBasicMiddlewares(logger)
	demonstrateBuiltInMiddlewares(logger)
	demonstratAdvancedMiddlewareChain(logger)
	demonstrateInterceptors()
	demonstrateCustomMiddleware(logger)
}

// === BASIC MIDDLEWARE EXAMPLES ===

func demonstrateBasicMiddlewares(logger *zap.Logger) {
	fmt.Println("=== Basic Middleware Examples ===")

	// Simple logging middleware
	client := gorest.NewClient(
		gorest.WithHost("https://httpbin.org"),
		gorest.WithMiddleware(func(ctx *gorest.MiddlewareContext, next gorest.NextFunc) {
			// Pre-request processing
			start := time.Now()
			fmt.Printf("→ Starting %s %s\n", ctx.Method, ctx.Request.URL.Path)

			// Execute request
			if next() {
				// Post-response processing
				duration := time.Since(start)
				fmt.Printf("← Completed %s %s in %v (Status: %d)\n",
					ctx.Method, ctx.Request.URL.Path, duration, ctx.Response.StatusCode)
			} else {
				// Error processing
				fmt.Printf("✗ Failed %s %s: %v\n",
					ctx.Method, ctx.Request.URL.Path, ctx.Error.Message)
			}
		}),
	)

	// Make a test request
	err := client.Get("/get")
	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
	}

	fmt.Println()
}

// === BUILT-IN MIDDLEWARE EXAMPLES ===

func demonstrateBuiltInMiddlewares(logger *zap.Logger) {
	fmt.Println("=== Built-in Middleware Examples ===")

	// Create client with multiple built-in middlewares
	client := gorest.NewClient(
		gorest.WithHost("https://httpbin.org"),

		// Authentication middleware
		gorest.WithAuthMiddleware(func() (string, error) {
			// In real applications, this would fetch from secure storage
			return "demo-api-key-12345", nil
		}),

		// Logging middleware
		gorest.WithLoggingMiddleware(logger),

		// Metrics middleware (with demo collector)
		gorest.WithMetricsMiddleware(&DemoMetricsCollector{}),

		// Retry middleware
		gorest.WithRetryMiddleware(3),
	)

	// Make a test request that will show all middlewares in action
	err := client.Headers(map[string]string{
		"X-Demo-Request": "built-in-middlewares",
	}).Get("/get")

	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
	} else {
		fmt.Println("✓ Built-in middlewares executed successfully")
	}

	fmt.Println()
}

// === ADVANCED MIDDLEWARE CHAIN ===

func demonstratAdvancedMiddlewareChain(logger *zap.Logger) {
	fmt.Println("=== Advanced Middleware Chain ===")

	client := gorest.NewClient(
		gorest.WithHost("https://httpbin.org"),

		// Middleware chain executes in order:
		gorest.WithMiddlewares(
			// 1. Request ID middleware (adds correlation ID)
			func(ctx *gorest.MiddlewareContext, next gorest.NextFunc) {
				requestID := fmt.Sprintf("req-%d", time.Now().UnixNano())
				ctx.Request.Header.Set("X-Request-ID", requestID)
				ctx.Metadata["request_id"] = requestID
				next()
			},

			// 2. Performance monitoring middleware
			func(ctx *gorest.MiddlewareContext, next gorest.NextFunc) {
				start := time.Now()
				ctx.Metadata["start_time"] = start

				if next() {
					duration := time.Since(start)
					requestID := ctx.Metadata["request_id"].(string)
					fmt.Printf("📊 Performance [%s]: %v\n", requestID, duration)

					// Set performance thresholds
					if duration > 2*time.Second {
						fmt.Printf("⚠️  Slow request detected [%s]: %v\n", requestID, duration)
					}
				}
			},

			// 3. Response transformation middleware
			func(ctx *gorest.MiddlewareContext, next gorest.NextFunc) {
				if next() {
					// Add custom response headers for tracking
					if ctx.Response != nil {
						requestID := ctx.Metadata["request_id"].(string)
						ctx.Response.Header.Set("X-Processed-By", "gorest-middleware")
						ctx.Response.Header.Set("X-Request-ID", requestID)
						fmt.Printf("🔄 Response transformed [%s]\n", requestID)
					}
				}
			},
		),
	)

	// Make request to demonstrate the middleware chain
	err := client.Get("/get")
	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
	} else {
		fmt.Println("✓ Advanced middleware chain executed successfully")
	}

	fmt.Println()
}

// === INTERCEPTOR EXAMPLES ===

func demonstrateInterceptors() {
	fmt.Println("=== Request & Response Interceptors ===")

	client := gorest.NewClient(
		gorest.WithHost("https://httpbin.org"),

		// Request interceptors (simple pre-request processing)
		gorest.WithRequestInterceptors(
			// Add user agent
			func(ctx *gorest.MiddlewareContext) bool {
				ctx.Request.Header.Set("User-Agent", "GoRest-Demo/2.0")
				fmt.Println("🔧 Added User-Agent header")
				return true // Continue processing
			},

			// Add request timestamp
			func(ctx *gorest.MiddlewareContext) bool {
				ctx.Request.Header.Set("X-Request-Time", time.Now().Format(time.RFC3339))
				fmt.Println("⏰ Added request timestamp")
				return true
			},

			// Validate request (could abort if needed)
			func(ctx *gorest.MiddlewareContext) bool {
				if ctx.Request.URL.Path == "/forbidden" {
					fmt.Println("🚫 Request blocked by validation interceptor")
					return false // Abort request
				}
				fmt.Println("✅ Request validated")
				return true
			},
		),

		// Response interceptors (simple post-response processing)
		gorest.WithResponseInterceptors(
			// Log response summary
			func(ctx *gorest.MiddlewareContext) {
				if ctx.Response != nil {
					fmt.Printf("📨 Response received: %d %s\n",
						ctx.Response.StatusCode, ctx.Response.Status)
				}
			},

			// Collect response metrics
			func(ctx *gorest.MiddlewareContext) {
				fmt.Printf("📈 Response processed in %v\n", ctx.Duration)
			},

			// Error transformation
			func(ctx *gorest.MiddlewareContext) {
				if ctx.Error != nil && ctx.Error.ResponseCode == 404 {
					// Transform 404 errors to be more user-friendly
					ctx.Error.Message = "The requested resource was not found"
					fmt.Println("🔄 Error message transformed for user friendliness")
				}
			},
		),
	)

	// Test normal request
	fmt.Println("\n--- Normal Request ---")
	err := client.Get("/get")
	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
	} else {
		fmt.Println("✓ Request completed successfully")
	}

	fmt.Println()
}

// === CUSTOM MIDDLEWARE EXAMPLE ===

func demonstrateCustomMiddleware(logger *zap.Logger) {
	fmt.Println("=== Custom Middleware Example ===")

	// Create a custom caching middleware
	cache := make(map[string]CacheEntry)
	cachingMiddleware := func(ctx *gorest.MiddlewareContext, next gorest.NextFunc) {
		// Simple cache key based on method and URL
		cacheKey := ctx.Method + ":" + ctx.Request.URL.String()

		// Check cache first
		if entry, exists := cache[cacheKey]; exists {
			if time.Since(entry.Timestamp) < 5*time.Minute {
				// Cache hit
				fmt.Printf("💾 Cache hit for %s\n", cacheKey)
				ctx.Metadata["cached"] = true
				// Note: In a real implementation, you'd set the response from cache
				return
			}
		}

		// Cache miss - execute request
		fmt.Printf("🌐 Cache miss for %s - executing request\n", cacheKey)
		if next() {
			// Cache the successful response
			cache[cacheKey] = CacheEntry{
				Timestamp: time.Now(),
				Response:  "cached-response-data", // In real implementation, cache actual response
			}
			fmt.Printf("💾 Response cached for %s\n", cacheKey)
		}
	}

	client := gorest.NewClient(
		gorest.WithHost("https://httpbin.org"),
		gorest.WithMiddleware(cachingMiddleware),
		gorest.WithLoggingMiddleware(logger),
	)

	// First request - cache miss
	fmt.Println("\n--- First Request (Cache Miss) ---")
	err := client.Get("/get")
	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
	}

	// Second request - cache hit
	fmt.Println("\n--- Second Request (Cache Hit) ---")
	err = client.Get("/get")
	if err != nil {
		fmt.Printf("Request failed: %v\n", err)
	}

	fmt.Println()
	fmt.Println("=== Middleware Demo Complete ===")
	fmt.Println("Key benefits demonstrated:")
	fmt.Println("  🔐 Authentication with token management")
	fmt.Println("  📝 Comprehensive logging and monitoring")
	fmt.Println("  📊 Performance metrics and tracking")
	fmt.Println("  🔄 Automatic retry with exponential backoff")
	fmt.Println("  ⚡ Request/response transformation")
	fmt.Println("  🧩 Composable middleware chain")
	fmt.Println("  💾 Custom caching middleware")
	fmt.Println("  🎯 Simple interceptors for common tasks")
}

// === HELPER TYPES AND FUNCTIONS ===

// DemoMetricsCollector implements MetricsCollector for demonstration
type DemoMetricsCollector struct{}

func (d *DemoMetricsCollector) RecordRequestDuration(endpoint, method string, duration time.Duration, success bool) {
	status := "success"
	if !success {
		status = "failure"
	}
	fmt.Printf("📊 Metrics: %s %s took %v (%s)\n", method, endpoint, duration, status)
}

func (d *DemoMetricsCollector) RecordRequestCount(endpoint, method string, statusCode int) {
	fmt.Printf("📊 Metrics: %s %s returned %d\n", method, endpoint, statusCode)
}

func (d *DemoMetricsCollector) RecordRequestSize(endpoint, method string, size int64) {
	fmt.Printf("📊 Metrics: %s %s request size: %d bytes\n", method, endpoint, size)
}

func (d *DemoMetricsCollector) RecordResponseSize(endpoint, method string, size int64) {
	fmt.Printf("📊 Metrics: %s %s response size: %d bytes\n", method, endpoint, size)
}

// CacheEntry represents a cached response
type CacheEntry struct {
	Timestamp time.Time
	Response  string
}

func setupLogger() *zap.Logger {
	config := zap.NewDevelopmentConfig()
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	logger, err := config.Build()
	if err != nil {
		log.Printf("Failed to create logger: %v", err)
		return zap.NewNop()
	}
	return logger
}
