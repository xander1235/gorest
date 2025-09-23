package gorest

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"context"
	"github.com/xander1235/gorest/v2/constants/enums"
	"github.com/xander1235/gorest/v2/exceptions/errors"
	"github.com/xander1235/gorest/v2/parsers"
	"github.com/xander1235/gorest/v2/types"
)

// === SERVER-SENT EVENTS (SSE) STREAMING METHODS ===

// WithSSEConfig configures Server-Sent Events streaming settings for the request chain.
// This method allows customization of streaming behavior including timeouts, reconnection,
// event filtering, and buffer sizes.
//
// SSE Configuration options:
//   - ReadTimeout: Maximum time to wait for individual event reads
//   - ReconnectInterval: Time to wait between reconnection attempts
//   - MaxReconnectAttempts: Maximum number of reconnection attempts (-1 for unlimited)
//   - LastEventID: Resume streaming from a specific event ID
//   - BufferSize: Size of the events channel buffer
//   - EnableReconnect: Whether to automatically reconnect on connection loss
//   - OnReconnect: Callback function called before each reconnection attempt
//   - EventFilter: Function to filter events before sending to channel
//
// Parameters:
//   - config: SSE configuration object, nil uses default settings
//
// Returns:
//   - *NetworkClient: New client instance with SSE configuration applied
//
// Example:
//
//	stream, err := client.
//	  WithSSEConfig(&types.SSEConfig{
//	    ReadTimeout: 30 * time.Second,
//	    EnableReconnect: true,
//	    BufferSize: 200,
//	    EventFilter: func(event types.SSEEvent) bool {
//	      return event.Event == "update" // Only process "update" events
//	    },
//	  }).
//	  StreamGet("/events")
func (nc *NetworkClient) WithSSEConfig(config *types.SSEConfig) *NetworkClient {
	copyClient := nc.ensureRequestCopy()
	if config == nil {
		config = types.DefaultSSEConfig()
	}
	copyClient.sseConfig = config
	// Set the request type for SSE streaming
	copyClient.requestType = "text/event-stream"
	return copyClient
}

// StreamGet initiates a GET request for Server-Sent Events streaming.
// This method establishes a persistent connection to receive real-time events
// from the server in the SSE format. The method integrates with the existing
// middleware pipeline for authentication, logging, and error handling.
//
// The returned SSEStream provides channels for:
//   - Events: Channel of parsed SSE events
//   - Errors: Channel for streaming errors
//   - Done: Channel that signals when stream is closed
//
// Parameters:
//   - endpoint: The API endpoint path for streaming (e.g., "/events", "/notifications")
//
// Returns:
//   - *types.SSEStream: Stream object with event, error, and completion channels
//   - *errors.ErrorDetails: Error if stream setup failed, nil on success
//
// Example:
//
//	stream, err := client.
//	  Headers(map[string]string{"Authorization": "Bearer token"}).
//	  WithSSEConfig(&types.SSEConfig{BufferSize: 100}).
//	  StreamGet("/live-updates")
//
//	if err != nil {
//	  log.Fatal(err)
//	}
//	defer stream.Close()
//
//	for {
//	  select {
//	  case event := <-stream.Events:
//	    fmt.Printf("Event: %s, Data: %s\n", event.Event, event.Data)
//	  case err := <-stream.Errors:
//	    log.Printf("Stream error: %v\n", err)
//	  case <-stream.Done:
//	    log.Println("Stream closed")
//	    return
//	  }
//	}
func (nc *NetworkClient) StreamGet(endpoint string) (*types.SSEStream, *errors.ErrorDetails) {
	copyClient := nc.ensureRequestCopy()
	copyClient.requestType = enums.ServerSentEvents.ToString()
	return copyClient.executeStreamRequest(enums.GET, endpoint)
}

// StreamPost initiates a POST request for Server-Sent Events streaming.
// This method combines the ability to send request data with receiving
// real-time streaming responses. Useful for APIs that need initial parameters
// but then stream continuous updates.
//
// The request body (set with Body() method) is sent as part of the POST request,
// then the response is treated as an SSE stream for continuous data flow.
//
// Parameters:
//   - endpoint: The API endpoint path for streaming (e.g., "/subscribe", "/watch")
//
// Returns:
//   - *types.SSEStream: Stream object with event, error, and completion channels
//   - *errors.ErrorDetails: Error if stream setup failed, nil on success
//
// Example:
//
//	subscriptionData := map[string]interface{}{
//	  "topics": []string{"alerts", "updates"},
//	  "filter": "priority=high",
//	}
//
//	stream, err := client.
//	  Body(subscriptionData).
//	  Headers(map[string]string{"Authorization": "Bearer token"}).
//	  WithSSEConfig(&types.SSEConfig{EnableReconnect: true}).
//	  StreamPost("/subscribe")
//
//	if err != nil {
//	  log.Fatal(err)
//	}
//	defer stream.Close()
//
//	for event := range stream.Events {
//	  processEvent(event)
//	}
func (nc *NetworkClient) StreamPost(endpoint string) (*types.SSEStream, *errors.ErrorDetails) {
	copyClient := nc.ensureRequestCopy()
	copyClient.requestType = enums.ServerSentEvents.ToString()
	return copyClient.executeStreamRequest(enums.POST, endpoint)
}

// executeStreamRequest handles the core SSE streaming request execution.
// This method integrates with the existing middleware pipeline while adapting
// the response handling for streaming rather than single-response processing.
//
// The method:
// 1. Builds the HTTP request with SSE-specific headers
// 2. Executes request interceptors for validation and modification
// 3. Runs the middleware chain for authentication, logging, etc.
// 4. Establishes the SSE stream for continuous event processing
// 5. Executes response interceptors for post-processing
//
// Parameters:
//   - method: HTTP method for the streaming request (GET, POST)
//   - endpoint: The API endpoint path for streaming
//
// Returns:
//   - *types.SSEStream: Stream object for event consumption
//   - *errors.ErrorDetails: Error if stream setup failed, nil on success
func (nc *NetworkClient) executeStreamRequest(method enums.HttpMethods, endpoint string) (*types.SSEStream, *errors.ErrorDetails) {
	// Ensure SSE configuration is set
	if nc.sseConfig == nil {
		nc.sseConfig = types.DefaultSSEConfig()
	}

	// Build the HTTP request with SSE-specific headers
	req, err := nc.buildSSEHTTPRequest(method, endpoint)
	if err != nil {
		return nil, err
	}

	// Create middleware context for the request pipeline
	ctx := &MiddlewareContext{
		Request:   req,
		Response:  nil,
		Error:     nil,
		StartTime: time.Now(),
		Metadata:  make(map[string]interface{}),
		Endpoint:  endpoint,
		Method:    method.String(),
		Client:    nc,
	}

	// Mark this as a streaming request in the context
	ctx.Metadata["streaming"] = true
	ctx.Metadata["stream_type"] = "sse"

	// Execute request interceptors (simple pre-request processing)
	for _, interceptor := range nc.requestInterceptors {
		if !interceptor(ctx) {
			// Request interceptor aborted the request
			return nil, &errors.ErrorDetails{
				Message:      "Streaming request aborted by request interceptor",
				ResponseCode: 0,
			}
		}
	}

	// Execute middleware chain for streaming (modified for long-lived connections)
	success := nc.executeStreamingMiddlewareChain(ctx)
	if !success || ctx.Error != nil {
		return nil, ctx.Error
	}

	// At this point, ctx.Response should contain the HTTP response
	httpResp := ctx.Response
	if httpResp == nil {
		return nil, &errors.ErrorDetails{
			Message:      "No response received for SSE stream",
			ResponseCode: 0,
		}
	}

	// Validate that the response is suitable for SSE streaming
	if httpResp.StatusCode != http.StatusOK {
		return nil, &errors.ErrorDetails{
			Message:      fmt.Sprintf("SSE stream failed with status: %d %s", httpResp.StatusCode, httpResp.Status),
			ResponseCode: httpResp.StatusCode,
		}
	}

	contentType := httpResp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		return nil, &errors.ErrorDetails{
			Message:      fmt.Sprintf("Expected text/event-stream, got: %s", contentType),
			ResponseCode: httpResp.StatusCode,
		}
	}

	// Create SSE parser and start stream processing with proper context
	parser := parsers.NewSSEParser(nc.sseConfig)
	// Use background context if nc.ctx is nil
	streamCtx := nc.ctx

	if streamCtx == nil {
		streamCtx = context.Background()
	}

	stream := parser.ParseStream(streamCtx, httpResp)

	// Execute response interceptors (simple post-response processing)
	// Note: For streaming, these interceptors run after stream setup but before events start flowing
	for _, interceptor := range nc.responseInterceptors {
		if !interceptor(ctx) {
			// Response interceptor indicated an issue
			stream.Close()
			break
		}
	}

	return stream, nil
}

// ADD these functions after the previous code to complete sse_client.go:

// buildSSEHTTPRequest creates an HTTP request configured for Server-Sent Events.
// This method extends the regular request building with SSE-specific headers
// and optimizations for long-lived streaming connections.
//
// SSE-specific headers added:
//   - Accept: text/event-stream
//   - Cache-Control: no-cache
//   - Connection: keep-alive (for HTTP/1.1)
//   - Last-Event-ID: If resuming from a specific event
//
// Parameters:
//   - method: HTTP method for the streaming request
//   - endpoint: The API endpoint path for streaming
//
// Returns:
//   - *http.Request: Configured HTTP request for SSE streaming
//   - *errors.ErrorDetails: Error if request building failed, nil on success
func (nc *NetworkClient) buildSSEHTTPRequest(method enums.HttpMethods, endpoint string) (*http.Request, *errors.ErrorDetails) {
	// Build base HTTP request using existing functionality
	req, err := nc.buildHTTPRequest(method, endpoint)
	if err != nil {
		return nil, err
	}

	// Add SSE-specific headers
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	// For HTTP/1.1, ensure keep-alive connection
	if req.ProtoMajor == 1 && req.ProtoMinor == 1 {
		req.Header.Set("Connection", "keep-alive")
	}

	// Add Last-Event-ID header if resuming from a specific event
	if nc.sseConfig != nil && nc.sseConfig.LastEventID != "" {
		req.Header.Set("Last-Event-ID", nc.sseConfig.LastEventID)
	}

	return req, nil
}

// executeStreamingMiddlewareChain executes the middleware chain with adaptations
// for streaming requests. This method reuses the existing middleware infrastructure
// while making necessary adjustments for long-lived streaming connections.
//
// Streaming adaptations:
//   - Timeout handling is adjusted for persistent connections
//   - Circuit breaker logic accounts for streaming vs single-request patterns
//   - Rate limiting may be bypassed or adjusted for streaming endpoints
//   - Logging is optimized for streaming connection establishment
//
// Parameters:
//   - ctx: Middleware context containing request, response, and metadata
//
// Returns:
//   - bool: True if middleware chain completed successfully, false otherwise
func (nc *NetworkClient) executeStreamingMiddlewareChain(ctx *MiddlewareContext) bool {
	// For now, delegate to the existing middleware chain
	// In the future, this could have streaming-specific optimizations
	// Execute the middleware chain and check for a valid response
	success := nc.executeMiddlewareChain(ctx, true)
	// Add debugging to check if response is nil after middleware
	if ctx.Response == nil {
		fmt.Println("Warning: Response is nil after middleware chain execution")
	}
	return success
}
