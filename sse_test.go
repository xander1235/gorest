package gorest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/xander1235/gorest/types"
)

// TestSSEBasicStreaming tests basic SSE streaming functionality
func TestSSEBasicStreaming(t *testing.T) {
	// Create a test SSE server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify SSE headers
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("Expected Accept: text/event-stream, got: %s", r.Header.Get("Accept"))
		}

		// Set SSE headers
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter doesn't support flushing")
		}

		// Send test events
		events := []string{
			"event: message\ndata: Hello World\nid: 1\n\n",
			"event: update\ndata: {\"status\": \"active\"}\nid: 2\n\n",
			"event: notification\ndata: Task completed\nid: 3\n\n",
		}

		for _, event := range events {
			fmt.Fprint(w, event)
			flusher.Flush()
			time.Sleep(100 * time.Millisecond)
		}
	}))
	defer server.Close()

	// Create client and start streaming
	client := NewClient()

	stream, err := client.
		Host(server.URL).
		WithSSEConfig(&types.SSEConfig{
			BufferSize:  10,
			ReadTimeout: 5 * time.Second,
		}).
		StreamGet("/events")

	if err != nil {
		t.Fatalf("Failed to start stream: %v", err)
	}
	defer stream.Close()

	// Collect events
	var receivedEvents []types.SSEEvent
	timeout := time.After(1000 * time.Second)

	for len(receivedEvents) < 3 {
		select {
		case event := <-stream.Events:
			receivedEvents = append(receivedEvents, event)
			t.Logf("Received event: %s, data: %s, id: %s", event.Event, event.Data, event.ID)

		case err := <-stream.Errors:
			t.Fatalf("Stream error: %v", err)

		case <-stream.Done:
			break

		case <-timeout:
			t.Fatalf("Timeout waiting for events, received %d events", len(receivedEvents))
		}
	}

	// Verify received events
	expectedEvents := []struct {
		event string
		data  string
		id    string
	}{
		{"message", "Hello World", "1"},
		{"update", "{\"status\": \"active\"}", "2"},
		{"notification", "Task completed", "3"},
	}

	if len(receivedEvents) != len(expectedEvents) {
		t.Errorf("Expected %d events, got %d", len(expectedEvents), len(receivedEvents))
	}

	for i, expected := range expectedEvents {
		if i >= len(receivedEvents) {
			t.Errorf("Missing event %d", i)
			continue
		}

		actual := receivedEvents[i]
		if actual.Event != expected.event {
			t.Errorf("Event %d: expected event '%s', got '%s'", i, expected.event, actual.Event)
		}
		if actual.Data != expected.data {
			t.Errorf("Event %d: expected data '%s', got '%s'", i, expected.data, actual.Data)
		}
		if actual.ID != expected.id {
			t.Errorf("Event %d: expected ID '%s', got '%s'", i, expected.id, actual.ID)
		}
	}
}

// TestSSEWithEventFilter tests SSE streaming with event filtering
func TestSSEWithEventFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter doesn't support flushing")
		}

		// Send mixed events - some should be filtered out
		events := []string{
			"event: keep\ndata: This should pass\n\n",
			"event: filter\ndata: This should be filtered\n\n",
			"event: keep\ndata: This should also pass\n\n",
			"event: ignore\ndata: This should be ignored\n\n",
			"event: keep\ndata: Final passing event\n\n",
		}

		for _, event := range events {
			fmt.Fprint(w, event)
			flusher.Flush()
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer server.Close()

	// Create client with event filter
	client := NewClient()

	stream, err := client.
		Host(server.URL).
		WithSSEConfig(&types.SSEConfig{
			BufferSize: 10,
			EventFilter: func(event types.SSEEvent) bool {
				// Only keep events with type "keep"
				return event.Event == "keep"
			},
		}).
		StreamGet("/filtered-events")

	if err != nil {
		t.Fatalf("Failed to start filtered stream: %v", err)
	}
	defer stream.Close()

	// Collect filtered events
	var filteredEvents []types.SSEEvent
	timeout := time.After(2 * time.Second)

	for len(filteredEvents) < 3 {
		select {
		case event := <-stream.Events:
			filteredEvents = append(filteredEvents, event)
			t.Logf("Filtered event: %s, data: %s", event.Event, event.Data)

		case err := <-stream.Errors:
			t.Fatalf("Stream error: %v", err)

		case <-stream.Done:
			break

		case <-timeout:
			t.Fatalf("Timeout waiting for filtered events, received %d", len(filteredEvents))
		}
	}

	// Verify only "keep" events were received
	if len(filteredEvents) != 3 {
		t.Errorf("Expected 3 filtered events, got %d", len(filteredEvents))
	}

	for i, event := range filteredEvents {
		if event.Event != "keep" {
			t.Errorf("Event %d should have been filtered: %s", i, event.Event)
		}
	}

	expectedData := []string{
		"This should pass",
		"This should also pass",
		"Final passing event",
	}

	for i, expectedData := range expectedData {
		if i >= len(filteredEvents) {
			t.Errorf("Missing filtered event %d", i)
			continue
		}
		if filteredEvents[i].Data != expectedData {
			t.Errorf("Event %d: expected data '%s', got '%s'", i, expectedData, filteredEvents[i].Data)
		}
	}
}

// TestSSEPostStreaming tests POST requests with SSE streaming
func TestSSEPostStreaming(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify it's a POST request
		if r.Method != "POST" {
			t.Errorf("Expected POST method, got %s", r.Method)
		}

		// Verify request body (subscription data)
		body := make([]byte, r.ContentLength)
		r.Body.Read(body)
		bodyStr := string(body)

		if !strings.Contains(bodyStr, "topics") {
			t.Errorf("Request body should contain subscription topics")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter doesn't support flushing")
		}

		// Send subscription confirmation and events
		events := []string{
			"event: subscribed\ndata: Subscription confirmed\n\n",
			"event: data\ndata: {\"user_id\": 123, \"action\": \"login\"}\n\n",
		}

		for _, event := range events {
			fmt.Fprint(w, event)
			flusher.Flush()
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer server.Close()

	// Subscription data
	subscriptionData := map[string]interface{}{
		"topics":    []string{"user-events", "system-alerts"},
		"client_id": "test-client",
	}

	// Create client and start POST streaming
	client := NewClient()

	stream, err := client.
		Host(server.URL).
		Body(subscriptionData).
		WithSSEConfig(&types.SSEConfig{BufferSize: 5}).
		StreamPost("/subscribe")

	if err != nil {
		t.Fatalf("Failed to start POST stream: %v", err)
	}
	defer stream.Close()

	// Collect events
	var events []types.SSEEvent
	timeout := time.After(1 * time.Second)

	for len(events) < 2 {
		select {
		case event := <-stream.Events:
			events = append(events, event)
			t.Logf("POST stream event: %s, data: %s", event.Event, event.Data)

		case err := <-stream.Errors:
			t.Fatalf("POST stream error: %v", err)

		case <-stream.Done:
			break

		case <-timeout:
			t.Fatalf("Timeout waiting for POST events, received %d", len(events))
		}
	}

	// Verify subscription flow
	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}

	if events[0].Event != "subscribed" {
		t.Errorf("First event should be 'subscribed', got '%s'", events[0].Event)
	}

	if events[1].Event != "data" {
		t.Errorf("Second event should be 'data', got '%s'", events[1].Event)
	}
}

// TestSSEStreamClose tests proper stream closure
func TestSSEStreamClose(t *testing.T) {
	// Create a context with cancel for the server handler
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // This will signal the handler to exit

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// CRITICAL: Flush headers to client before blocking
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		} else {
			t.Log("Warning: ResponseWriter doesn't support Flush")
		}

		// Keep connection open, but allow graceful shutdown
		select {
		case <-ctx.Done():
			// Server is shutting down, exit handler
			return
		case <-r.Context().Done():
			// Client closed connection
			return
		}
	}))
	defer server.Close()

	client := NewClient(WithTimeout(0))

	stream, err := client.
		Host(server.URL).
		StreamGet("/long-running")

	if err != nil {
		t.Fatalf("Failed to start stream: %v", err)
	}

	// Verify stream is active
	if !stream.IsActive() {
		t.Error("Stream should be active")
	}

	// Close the stream
	inbuiltErr := stream.Close()
	if inbuiltErr != nil {
		t.Errorf("Error closing stream: %v", err)
	}

	// Verify stream is no longer active
	time.Sleep(10 * time.Millisecond) // Give it time to close
	if stream.IsActive() {
		t.Error("Stream should be inactive after close")
	}

	// Verify Done channel is closed
	select {
	case <-stream.Done:
		t.Log("Stream closed")
		// Expected - Done channel should be closed
	case <-time.After(100 * time.Millisecond):
		t.Error("Done channel should be closed after stream.Close()")
	}
}

// BenchmarkSSEThroughput benchmarks SSE event processing throughput
func BenchmarkSSEThroughput(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		// Send many events quickly
		for i := 0; i < b.N; i++ {
			fmt.Fprintf(w, "data: Event %d\n\n", i)
			flusher.Flush()
		}
	}))
	defer server.Close()

	client := NewClient()

	stream, err := client.
		Host(server.URL).
		WithSSEConfig(&types.SSEConfig{
			BufferSize: b.N + 100, // Large buffer to avoid blocking
		}).
		StreamGet("/benchmark")

	if err != nil {
		b.Fatalf("Failed to start stream: %v", err)
	}
	defer stream.Close()

	b.ResetTimer()

	// Process all events
	eventCount := 0
	for eventCount < b.N {
		select {
		case <-stream.Events:
			eventCount++
		case err := <-stream.Errors:
			b.Fatalf("Stream error: %v", err)
		case <-stream.Done:
			break
		}
	}
}

// Example of how to test SSE with your existing middleware
// TestSSEWithMiddleware tests SSE with middleware integration
func TestSSEWithMiddleware(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify middleware added headers
		if r.Header.Get("X-Test-Middleware") != "active" {
			t.Error("Middleware header not found")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: Middleware test successful\n\n")
	}))
	defer server.Close()

	// Create a simple test middleware
	testMiddleware := func(ctx *MiddlewareContext, next NextFunc) {
		// Add test header
		ctx.Request.Header.Set("X-Test-Middleware", "active")
		ctx.Metadata["middleware_executed"] = true
		next()
	}

	// Create client with middleware using your functional options pattern
	client := NewClient(
		WithMiddleware(testMiddleware),
	)

	stream, err := client.
		Host(server.URL).
		StreamGet("/middleware-test")

	if err != nil {
		t.Fatalf("Failed to start stream with middleware: %v", err)
	}
	defer stream.Close()

	// Verify event is received (middleware worked)
	select {
	case event := <-stream.Events:
		if event.Data != "Middleware test successful" {
			t.Errorf("Expected success message, got: %s", event.Data)
		}
	case err := <-stream.Errors:
		t.Fatalf("Middleware stream error: %v", err)
	case <-time.After(1 * time.Second):
		t.Fatal("Timeout waiting for middleware test event")
	}
}
