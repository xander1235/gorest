package gorest

import (
	"context"
	"go.uber.org/zap"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/xander1235/gorest/v2/constants/enums"
	"github.com/xander1235/gorest/v2/exceptions/errors"
	"github.com/xander1235/gorest/v2/parsers"
	"github.com/xander1235/gorest/v2/types"
)

// === GLOBAL CLIENT MANAGEMENT ===

// Client is the global singleton client instance available for immediate use.
// It's initialized with sensible defaults and can be configured once using Initialize().
// For service-specific configurations, use NewClient() instead of the global instance.
//
// Default configuration:
//   - 30 second timeout
//   - Connection pooling with 100 max idle connections
//   - No rate limiting or circuit breaker
//   - JSON request type
//   - Standard response and error parsers
//
// Thread safety:
//
//	The global client uses smart copy-on-write with request ID tracking for thread-safe request handling.
//	Multiple goroutines can safely use Client.Host().Get() concurrently.
var Client *NetworkClient

// globalInitialized tracks whether Initialize() has been called to prevent
// multiple initialization attempts that could override user configuration.
var globalInitialized bool

// initMutex protects concurrent access to Initialize() to ensure thread-safe
// one-time initialization even when called from multiple goroutines.
var initMutex sync.Mutex

// NetworkClient represents the main HTTP client with advanced networking capabilities.
// It supports multiple configuration levels and provides thread-safe request handling
// through a smart copy-on-write pattern that shares infrastructure while isolating request data.
//
// The client maintains shared infrastructure (HTTP transport, rate limiters, circuit breakers)
// for efficiency while ensuring request-specific data (headers, body, params) is isolated
// to prevent race conditions in concurrent usage.
//
// Smart Copy-on-Write Pattern:
//   - Fresh clients have empty requestID
//   - First method call creates a copy with new requestID
//   - Subsequent method calls on same chain modify the copy in-place
//   - Result: Exactly ONE copy per request chain, not per method call
type NetworkClient struct {
	// === SHARED INFRASTRUCTURE (Thread-safe, immutable after creation) ===
	// These components are shared across all requests for efficiency and state consistency

	// httpClient handles the actual HTTP communication with optimized transport configuration.
	// Shared to enable connection pooling and reuse across requests.
	httpClient *http.Client

	// logger provides structured logging for request/response tracing and debugging.
	// Shared to maintain consistent logging configuration and output destination.
	logger *zap.Logger

	// endpointConfigs stores per-endpoint configuration with rate limiters and circuit breakers.
	// Each endpoint gets its own isolated rate limiter and circuit breaker for proper isolation.
	endpointConfigs map[string]*EndpointConfig

	// defaultEndpointConfig provides fallback configuration for endpoints without specific config.
	// Contains default rate limiter, circuit breaker, retry, and timeout settings.
	defaultEndpointConfig *EndpointConfig
	// middlewares contains the middleware chain executed for all requests from this client.
	// Middlewares execute in the order they were added and provide cross-cutting concerns
	// like authentication, logging, metrics, and error handling.
	middlewares []Middleware

	// requestInterceptors are simple functions executed before sending HTTP requests.
	// Use these for simple request modifications like adding headers or validation.
	requestInterceptors []RequestInterceptor

	// responseInterceptors are simple functions executed after receiving HTTP responses.
	// Use these for simple response processing like logging, metrics, or error handling.
	responseInterceptors []ResponseInterceptor

	// === CONFIGURATION DEFAULTS ===
	// These are client-level defaults that can be overridden per request

	// defaultHeaders contains headers automatically added to all requests from this client.
	// Used for organization-wide standards like User-Agent, Accept headers, etc.
	defaultHeaders map[string]string

	// === REQUEST-SPECIFIC DATA (Smart copy-on-write for thread safety) ===
	// These fields are managed by the smart copy-on-write pattern

	// requestID tracks whether this is a fresh client ("") or request-specific copy (uuid).
	// This is the key to the smart copy-on-write pattern:
	//   - Empty requestID = fresh client, needs copy on first method call
	//   - Non-empty requestID = already copied, modify in-place
	requestID string

	// host is the base URL for API requests (e.g., "https://api.example.com").
	// Set per request chain to allow different endpoints in the same client.
	host string

	// headers contains request-specific HTTP headers that override defaults.
	// Copied per request to prevent concurrent modification issues.
	headers map[string]string

	// params contains URL query parameters for the request.
	// Copied per request to allow different parameters for concurrent requests.
	params map[string]string

	// body contains the request payload for POST/PUT/PATCH operations.
	// Can be any serializable type (struct, map, string, etc.).
	body any

	// multipart contains multipart form data for file uploads and complex forms.
	// Used when request type is set to multipart/form-data.
	multipart *types.MultipartBody

	// response is a pointer to the struct where response data should be unmarshaled.
	// Set per request chain to capture the response in the desired format.
	response any

	// requestType specifies the Content-Type header (JSON, multipart, form-urlencoded).
	// Determines how the request body is serialized and sent.
	requestType string

	// ctx provides request-level context for cancellation, timeouts, and tracing.
	// Allows per-request timeout and cancellation without affecting other requests.
	ctx context.Context
	// === SSE STREAMING CONFIGURATION ===
	// These fields control Server-Sent Events streaming behavior

	// sseConfig contains configuration for SSE streams (timeouts, reconnect, etc.)
	// Allows customization of streaming behavior per request chain
	sseConfig *types.SSEConfig

	// === INTERNAL PARSING FUNCTIONS ===
	// These handle response and error parsing with customizable logic

	// parser converts successful HTTP response body to the desired Go struct.
	// Customizable to handle different API response formats.
	parser func(string, interface{}) *errors.ErrorDetails

	// errorParser extracts error information from HTTP error responses.
	// Customizable to handle different API error formats and structures.
	errorParser func(string) *errors.ErrorDetails
}

// Package initialization creates the default global client ready for immediate use.
// This enables zero-configuration usage while still allowing customization through Initialize().
func init() {
	Client = newDefaultClient()
}

// newDefaultClient creates a Client with production-ready default settings.
// These defaults balance performance, reliability, and ease of use for most applications.
//
// Default settings rationale:
//   - 30s timeout: Covers most API operations without being too aggressive
//   - 100 max idle connections: Good balance for connection reuse without excessive memory
//   - 10 connections per host: Reasonable parallelism for most services
//   - 90s idle timeout: Matches typical server keep-alive settings
//   - No rate limiting: Allows maximum throughput by default
//   - JSON content type: Most common API format
//
// Returns:
//   - *Client: A fully configured client ready for use
func newDefaultClient() *NetworkClient {
	return &NetworkClient{
		httpClient: &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:          100,              // Total connection pool size
				MaxIdleConnsPerHost:   10,               // Connections per host
				IdleConnTimeout:       90 * time.Second, // Keep-alive duration
				TLSHandshakeTimeout:   10 * time.Second, // TLS setup timeout
				ResponseHeaderTimeout: 10 * time.Second, // Header read timeout
			},
			Timeout: 30 * time.Second, // Overall request timeout
		},
		parser:          parsers.ParseResponse,
		errorParser:     parsers.ParseError,
		requestType:     enums.Json.ToString(),
		endpointConfigs: make(map[string]*EndpointConfig),
		defaultEndpointConfig: &EndpointConfig{
			// No rate limiting by default - maximum throughput
			rateLimiter: nil,
			// No circuit breaker by default - let all requests through
			circuitBreaker: nil,
			// No custom timeout - use client default
			timeout: 0,
			// No retry config - single attempt by default
			retryConfig: nil,
		},
		defaultHeaders: make(map[string]string),
		// requestID starts empty, indicating this is a fresh client
	}
}

// Initialize configures the global Client with custom options.
// This method can only be called once successfully - subsequent calls are ignored
// to prevent accidental reconfiguration that could disrupt running applications.
//
// Best practices:
//   - Call Initialize() once at application startup before using Client
//   - Configure organization-wide defaults (timeouts, rate limits, logging)
//   - Use consistent configuration across all services in your organization
//   - Consider environment-specific settings (prod vs dev timeouts)
//
// Parameters:
//   - options: Variable number of ClientOption functions for configuration
//
// Example:
//
//	func main() {
//	    // Configure global defaults once at startup
//	    gorest.Initialize(
//	        gorest.WithTimeout(30 * time.Second),
//	        gorest.WithRateLimit(rate.Limit(100), 20),
//	        gorest.WithLogger(logger),
//	        gorest.WithDefaultHeaders(map[string]string{
//	            "User-Agent": "MyApp/1.0",
//	            "Accept":     "application/json",
//	        }),
//	    )
//
//	    // Now use the configured global client
//	    gorest.Client.Host("https://api.example.com").Get("/users")
//	}
//
// Thread safety:
//
//	Uses mutex to ensure only one goroutine can initialize successfully,
//	even if Initialize() is called concurrently from multiple goroutines.
func Initialize(options ...ClientOption) {
	initMutex.Lock()
	defer initMutex.Unlock()

	if globalInitialized {
		// Log warning about duplicate initialization if logger is available
		if Client.logger != nil {
			Client.logger.Warn("gorest.Initialize() called multiple times - ignoring subsequent calls")
		}
		return
	}

	// Apply all configuration options to the global client
	for _, opt := range options {
		opt(Client)
	}

	globalInitialized = true
}

// NewClient creates a new Client instance with custom configuration.
// Unlike the global Client, each NewClient() call creates a completely
// independent client instance suitable for service-specific configurations.
//
// Use cases for NewClient():
//   - Different services with different rate limits or timeouts
//   - Service-specific circuit breaker configurations
//   - Endpoint-specific policies for different API groups
//   - Testing with isolated client configurations
//   - Multi-tenant applications with per-tenant settings
//
// Parameters:
//   - options: Variable number of ClientOption functions for configuration
//
// Returns:
//   - *Client: A new client instance with applied configuration
//
// Example:
//
//	func setupAPIClients() {
//	    // Payment service - strict limits for financial operations
//	    paymentClient := gorest.NewClient(
//	        gorest.WithHost("https://payment-api.com"),
//	        gorest.WithTimeout(10 * time.Second),
//	        gorest.WithRateLimit(rate.Limit(5), 2),
//	        gorest.WithCircuitBreaker(gorest.CircuitBreakerConfig{
//	            MaxFailures:  2,
//	            ResetTimeout: 30 * time.Second,
//	        }),
//	    )
//
//	    // User service - moderate limits for user operations
//	    userClient := gorest.NewClient(
//	        gorest.WithHost("https://user-api.com"),
//	        gorest.WithRateLimit(rate.Limit(50), 10),
//	    )
//
//	    return paymentClient, userClient
//	}
//
// Thread safety:
//
//	Each client instance is independent and uses smart copy-on-write for thread safety.
//	Multiple goroutines can safely use the same client instance concurrently.
func NewClient(options ...ClientOption) *NetworkClient {
	// Start with fresh default configuration
	client := newDefaultClient()

	// Apply all provided configuration options
	for _, opt := range options {
		opt(client)
	}

	return client
}

// === INTERNAL COPY-ON-WRITE HELPER ===

// ensureRequestCopy ensures we have a request-specific copy of the Client.
// This implements the smart copy-on-write pattern:
//   - If requestID is empty (fresh client), creates a copy and assigns a new requestID
//   - If requestID is not empty (already copied), returns the same instance
//
// This eliminates code duplication across all fluent API methods and ensures
// exactly one copy per request chain, not per method call.
//
// Returns:
//   - *Client: A request-specific copy for fresh clients, or the same instance for copied clients
func (nc *NetworkClient) ensureRequestCopy() *NetworkClient {
	if nc.requestID == "" {
		// Fresh client - create a copy for this request chain
		copyClient := nc.copyForRequest()
		copyClient.requestID = uuid.New().String() // Mark as request in progress
		return copyClient
	}
	// Already a request-specific copy - return same instance
	return nc
}

// === SMART COPY-ON-WRITE FLUENT API METHODS ===
// These methods implement the fluent/builder pattern with smart copy-on-write:
// - Uses ensureRequestCopy() to handle the copy logic consistently
// - Each method is now clean and focused on its specific responsibility

// Host sets the base URL for the request chain and returns a Client.
// Uses smart copy-on-write to ensure exactly one copy per request chain.
//
// The host should include the protocol (http:// or https://) and may include
// a port number. The host is combined with endpoint paths to form complete URLs.
//
// Parameters:
//   - host: Complete base URL including protocol (e.g., "https://api.example.com")
//
// Returns:
//   - *Client: Copy for fresh clients, same instance for request chains
//
// Example:
//
//	client.Host("https://api.stripe.com/v1").Get("/customers")
//	// Results in GET https://api.stripe.com/v1/customers
func (nc *NetworkClient) Host(host string) *NetworkClient {
	copyClient := nc.ensureRequestCopy()
	copyClient.host = host
	return copyClient
}

// Headers sets custom HTTP headers for the request and returns a Client.
// Uses smart copy-on-write for optimal performance.
//
// Headers specified here will be merged with default headers, with request-specific
// headers taking precedence over defaults.
//
// Common headers:
//   - Authorization: "Bearer token" or "Basic base64credentials"
//   - Content-Type: Usually set automatically based on request type
//   - Accept: Specify desired response format
//   - Custom headers: API-specific headers for routing, versioning, etc.
//
// Parameters:
//   - headers: Map of header names to values
//
// Returns:
//   - *Client: Copy for fresh clients, same instance for request chains
//
// Example:
//
//	client.Headers(map[string]string{
//	    "Authorization": "Bearer " + token,
//	    "Accept":        "application/json",
//	    "X-API-Version": "v2",
//	}).Post("/users")
func (nc *NetworkClient) Headers(headers map[string]string) *NetworkClient {
	copyClient := nc.ensureRequestCopy()
	copyClient.headers = headers
	return copyClient
}

// Params sets URL query parameters for the request and returns a Client.
// Uses smart copy-on-write pattern for optimal performance.
//
// Parameters are encoded and appended to the URL query string automatically.
//
// Query parameters are commonly used for:
//   - Filtering: ?status=active&category=electronics
//   - Pagination: ?page=2&limit=50
//   - Sorting: ?sort=name&order=desc
//   - Search: ?q=golang+http+client
//   - API versioning: ?version=v2
//
// Parameters:
//   - params: Map of parameter names to values
//
// Returns:
//   - *Client: Copy for fresh clients, same instance for request chains
//
// Example:
//
//	client.Params(map[string]string{
//	    "page":     "1",
//	    "limit":    "50",
//	    "status":   "active",
//	}).Get("/users")
func (nc *NetworkClient) Params(params map[string]string) *NetworkClient {
	copyClient := nc.ensureRequestCopy()
	copyClient.params = params
	return copyClient
}

// Body sets the request body payload and returns a Client.
// Uses smart copy-on-write for efficient memory usage.
//
// The body can be any serializable Go type and will be encoded based on the
// current request type (JSON by default, multipart, or form-urlencoded).
//
// Supported body types:
//   - Structs: Serialized to JSON (or other formats based on request type)
//   - Maps: map[string]interface{} or map[string]string
//   - Slices: []interface{} for JSON arrays
//   - Strings: Used directly (set appropriate Content-Type header)
//
// Parameters:
//   - body: Request payload of any serializable type
//
// Returns:
//   - *Client: Copy for fresh clients, same instance for request chains
//
// Example:
//
//	user := User{Name: "John", Email: "john@example.com"}
//	client.Body(user).Post("/users")
func (nc *NetworkClient) Body(body interface{}) *NetworkClient {
	copyClient := nc.ensureRequestCopy()
	copyClient.body = body
	return copyClient
}

// MultipartBody sets multipart form data for file uploads and complex forms.
// Uses smart copy-on-write and automatically changes the request type to multipart/form-data.
//
// Multipart requests are commonly used for:
//   - File uploads with metadata
//   - Forms with mixed content types (text + files)
//   - API endpoints expecting multipart/form-data
//
// Parameters:
//   - multipart: Configured MultipartBody with fields and files
//
// Returns:
//   - *Client: Copy for fresh clients, same instance for request chains
//
// Example:
//
//	multipart := &types.MultipartBody{}
//	multipart.Add("name", "John Doe")
//	multipart.AddFile("avatar", avatarFile)
//	client.MultipartBody(multipart).Post("/users/upload")
func (nc *NetworkClient) MultipartBody(multipart *types.MultipartBody) *NetworkClient {
	copyClient := nc.ensureRequestCopy()
	copyClient.multipart = multipart
	copyClient.requestType = enums.Multipart.ToString()
	return copyClient
}

// RequestType explicitly sets the request content type and encoding method.
// Uses smart copy-on-write pattern.
//
// Supported request types:
//   - JSON: application/json (default) - most common for REST APIs
//   - Multipart: multipart/form-data - for file uploads and complex forms
//   - FormUrlEncoded: application/x-www-form-urlencoded - for simple forms
//
// Parameters:
//   - requestType: enums.RequestType specifying the encoding method
//
// Returns:
//   - *Client: Copy for fresh clients, same instance for request chains
//
// Example:
//
//	client.Body(map[string]string{
//	    "username": "john",
//	    "password": "secret",
//	}).RequestType(enums.FormUrlEncoded).Post("/login")
func (nc *NetworkClient) RequestType(requestType enums.RequestType) *NetworkClient {
	copyClient := nc.ensureRequestCopy()
	copyClient.requestType = requestType.ToString()
	return copyClient
}

// Response sets the destination struct for unmarshaling the HTTP response.
// Uses smart copy-on-write pattern.
//
// The response body will be automatically parsed and populated into the
// provided struct based on the response Content-Type.
//
// Parameters:
//   - response: Pointer to struct where response data should be stored
//
// Returns:
//   - *Client: Copy for fresh clients, same instance for request chains
//
// Example:
//
//	var user User
//	err := client.Response(&user).Get("/users/123")
//	if err == nil {
//	    fmt.Printf("User: %+v", user)
//	}
func (nc *NetworkClient) Response(response interface{}) *NetworkClient {
	copyClient := nc.ensureRequestCopy()
	copyClient.response = response
	return copyClient
}

// WithContext sets the request context for cancellation, timeouts, and tracing.
// Uses smart copy-on-write pattern.
//
// Context use cases:
//   - Request cancellation: Cancel requests when user navigates away
//   - Custom timeouts: Override default client timeouts per request
//   - Distributed tracing: Propagate trace spans across service calls
//   - Request-scoped values: Pass request-specific data (user ID, correlation ID)
//
// Parameters:
//   - ctx: Context for request lifecycle management
//
// Returns:
//   - *Client: Copy for fresh clients, same instance for request chains
//
// Example:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
//	defer cancel()
//	client.WithContext(ctx).Post("/long-running-operation")
func (nc *NetworkClient) WithContext(ctx context.Context) *NetworkClient {
	copyClient := nc.ensureRequestCopy()
	copyClient.ctx = ctx
	return copyClient
}

// === HTTP METHOD IMPLEMENTATIONS ===
// These methods execute HTTP requests with the configured parameters.
// They use ensureRequestCopy() to handle the copy logic consistently.

// Get executes an HTTP GET request to the specified endpoint.
// GET requests are used for retrieving data and should be idempotent.
//
// Parameters:
//   - endpoint: URL path to append to the configured host
//
// Returns:
//   - *errors.ErrorDetails: Error information if the request failed, nil on success
//
// Example:
//
//	err := client.Host("https://api.example.com").Get("/users/123")
func (nc *NetworkClient) Get(endpoint string) *errors.ErrorDetails {
	copyClient := nc.ensureRequestCopy()
	return copyClient.executeRequest(enums.GET, endpoint)
}

// Post executes an HTTP POST request to the specified endpoint.
// POST requests are used for creating new resources or submitting data.
//
// Parameters:
//   - endpoint: URL path to append to the configured host
//
// Returns:
//   - *errors.ErrorDetails: Error information if the request failed, nil on success
//
// Example:
//
//	err := client.Body(userData).Post("/users")
func (nc *NetworkClient) Post(endpoint string) *errors.ErrorDetails {
	copyClient := nc.ensureRequestCopy()
	return copyClient.executeRequest(enums.POST, endpoint)
}

// Put executes an HTTP PUT request to the specified endpoint.
// PUT requests are used for updating existing resources or creating with known ID.
//
// Parameters:
//   - endpoint: URL path to append to the configured host
//
// Returns:
//   - *errors.ErrorDetails: Error information if the request failed, nil on success
//
// Example:
//
//	err := client.Body(updatedUser).Put("/users/123")
func (nc *NetworkClient) Put(endpoint string) *errors.ErrorDetails {
	copyClient := nc.ensureRequestCopy()
	return copyClient.executeRequest(enums.PUT, endpoint)
}

// Patch executes an HTTP PATCH request to the specified endpoint.
// PATCH requests are used for partial updates to existing resources.
//
// Parameters:
//   - endpoint: URL path to append to the configured host
//
// Returns:
//   - *errors.ErrorDetails: Error information if the request failed, nil on success
//
// Example:
//
//	err := client.Body(partialUpdate).Patch("/users/123")
func (nc *NetworkClient) Patch(endpoint string) *errors.ErrorDetails {
	copyClient := nc.ensureRequestCopy()
	return copyClient.executeRequest(enums.PATCH, endpoint)
}

// Delete executes an HTTP DELETE request to the specified endpoint.
// DELETE requests are used for removing resources.
//
// Parameters:
//   - endpoint: URL path to append to the configured host
//
// Returns:
//   - *errors.ErrorDetails: Error information if the request failed, nil on success
//
// Example:
//
//	err := client.Delete("/users/123")
func (nc *NetworkClient) Delete(endpoint string) *errors.ErrorDetails {
	copyClient := nc.ensureRequestCopy()
	return copyClient.executeRequest(enums.DELETE, endpoint)
}

// === SERVER-SENT EVENTS (SSE) STREAMING METHODS ===

// WithSSEConfig configures Server-Sent Events streaming settings for the request chain.

// === INTERNAL HELPER FUNCTIONS ===

// getEndpointConfig resolves the appropriate configuration for a specific endpoint
// by checking exact matches first, then pattern matches, then falling back to defaults.
// This implements the configuration hierarchy that gives endpoints fine-grained control.
//
// Resolution order:
//  1. Exact endpoint match (e.g., "/auth/login")
//  2. Pattern match (e.g., "/products/*" matches "/products/123")
//  3. Client default configuration
//
// Parameters:
//   - nc: NetworkClient instance containing endpoint configurations
//   - endpoint: The endpoint path to find configuration for
//
// Returns:
//   - *EndpointConfig: The most specific configuration for the endpoint
//
// Pattern matching details:
//   - Uses filepath.Match for wildcard support
//   - Supports * for any sequence of characters
//   - Does not support ** for recursive matching
//   - Case-sensitive matching
//
// Example patterns:
//   - "/users/*" matches "/users/123", "/users/profile", "/users/settings"
//   - "/api/v1/*" matches "/api/v1/users", "/api/v1/orders"
//   - "/admin/*/*" matches "/admin/users/delete", "/admin/settings/update"
func (nc *NetworkClient) getEndpointConfig(endpoint string) *EndpointConfig {
	// 1. Check for exact match first (highest priority)
	if config, exists := nc.endpointConfigs[endpoint]; exists {
		return config
	}

	// 2. Check pattern matches (wildcards)
	for pattern, config := range nc.endpointConfigs {
		if matched, _ := filepath.Match(pattern, endpoint); matched {
			return config
		}
	}

	// 3. Return default endpoint configuration (fallback)
	return nc.defaultEndpointConfig
}

// === INTERNAL IMPLEMENTATION ===

// copyForRequest creates a shallow copy of the Client for copy-on-write pattern.
// This ensures thread safety by giving each request chain its own isolated configuration
// while sharing the expensive infrastructure components (HTTP client, rate limiter, etc.).
//
// Shared components (not copied):
//   - httpClient: HTTP transport and connection pool
//   - rateLimiter: Rate limiting state for consistent throttling
//   - circuitBreaker: Failure tracking for fault tolerance
//   - logger: Logging configuration and output destination
//   - endpointConfigs: Endpoint-specific configuration mapping
//   - defaultHeaders: Organization-wide header defaults
//   - retryConfig: Default retry behavior
//
// Copied components (isolated per request):
//   - host: Request-specific target host
//   - headers: Request-specific HTTP headers
//   - params: Request-specific query parameters
//   - body: Request payload
//   - response: Response destination
//   - requestType: Content-Type for request encoding
//   - ctx: Request-specific context
//   - requestID: Unique identifier for this request chain
//
// Returns:
//   - *Client: New instance sharing infrastructure but with isolated request data
func (nc *NetworkClient) copyForRequest() *NetworkClient {
	return &NetworkClient{
		// === SHARED INFRASTRUCTURE (same references) ===
		httpClient:            nc.httpClient,
		logger:                nc.logger,
		middlewares:           nc.middlewares,
		requestInterceptors:   nc.requestInterceptors,
		responseInterceptors:  nc.responseInterceptors,
		endpointConfigs:       nc.endpointConfigs,
		defaultHeaders:        nc.defaultHeaders,
		defaultEndpointConfig: nc.defaultEndpointConfig,
		parser:                nc.parser,
		errorParser:           nc.errorParser,

		// === REQUEST-SPECIFIC DATA (copied) ===
		headers:     copyStringMap(nc.headers),
		params:      copyStringMap(nc.params),
		host:        nc.host,
		body:        nc.body,
		multipart:   nc.multipart,
		response:    nc.response,
		requestType: nc.requestType,
		ctx:         nc.ctx,
		sseConfig:   nc.sseConfig,
		// requestID will be set by ensureRequestCopy()
	}
}

// copyStringMap creates a deep copy of a string map to prevent shared state issues.
// Returns nil for nil input to handle uninitialized maps correctly.
func copyStringMap(original map[string]string) map[string]string {
	if original == nil {
		return nil
	}
	mapCopy := make(map[string]string, len(original))
	for k, v := range original {
		mapCopy[k] = v
	}
	return mapCopy
}
