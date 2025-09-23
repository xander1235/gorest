package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/xander1235/gorest/v2"
	"github.com/xander1235/gorest/v2/types"
)

func main() {
	// Create a new client instance
	client := gorest.NewClient()

	// Example 1: Basic SSE Streaming
	fmt.Println("=== Basic SSE Streaming ===")
	basicStreamingExample(client)

	// Example 2: Advanced SSE with Configuration
	fmt.Println("\n=== Advanced SSE Configuration ===")
	advancedStreamingExample(client)

	// Example 3: POST Streaming with Request Data
	fmt.Println("\n=== POST Streaming with Data ===")
	postStreamingExample(client)

	// Example 4: Stream with Middleware and Authentication
	fmt.Println("\n=== Authenticated Streaming ===")
	authenticatedStreamingExample(client)
}

// Basic SSE streaming example
func basicStreamingExample(client *gorest.NetworkClient) {
	// Start a simple GET streaming request
	stream, err := client.
		Host("https://api.example.com").
		StreamGet("/events")

	if err != nil {
		log.Printf("Failed to start stream: %v", err)
		return
	}

	// Ensure stream is closed when done
	defer stream.Close()

	// Create a context with timeout for this example
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Process events from the stream
	for {
		select {
		case event := <-stream.Events:
			fmt.Printf("📨 Received Event: %s\n", event.Event)
			fmt.Printf("   Data: %s\n", event.Data)
			if event.ID != "" {
				fmt.Printf("   ID: %s\n", event.ID)
			}
			fmt.Printf("   Timestamp: %s\n", event.Timestamp.Format(time.RFC3339))

		case err := <-stream.Errors:
			log.Printf("❌ Stream error: %v", err)

		case <-stream.Done:
			fmt.Println("✅ Stream completed")
			return

		case <-ctx.Done():
			fmt.Println("⏰ Timeout reached, closing stream")
			return
		}
	}
}

// Advanced SSE streaming with custom configuration
func advancedStreamingExample(client *gorest.NetworkClient) {
	// Configure SSE streaming behavior
	sseConfig := &types.SSEConfig{
		ReadTimeout:          15 * time.Second,
		ReconnectInterval:    5 * time.Second,
		MaxReconnectAttempts: 3,
		BufferSize:           50,
		EnableReconnect:      true,
		OnReconnect: func(attempt int, lastEventID string) {
			log.Printf("🔄 Reconnection attempt %d, last event ID: %s", attempt, lastEventID)
		},
		EventFilter: func(event types.SSEEvent) bool {
			// Only process events of type "update" or "alert"
			return event.Event == "update" || event.Event == "alert"
		},
	}

	// Start streaming with custom configuration
	stream, err := client.
		Host("https://api.example.com").
		Headers(map[string]string{
			"User-Agent": "GoRest-SSE-Client/1.0",
			"Accept":     "text/event-stream",
		}).
		WithSSEConfig(sseConfig).
		StreamGet("/filtered-events")

	if err != nil {
		log.Printf("Failed to start configured stream: %v", err)
		return
	}

	defer stream.Close()

	// Process filtered events
	timeout := time.After(20 * time.Second)
	eventCount := 0

	for {
		select {
		case event := <-stream.Events:
			eventCount++
			fmt.Printf("📊 Filtered Event #%d: %s\n", eventCount, event.Event)
			fmt.Printf("   Data: %s\n", event.Data)

			// Stop after receiving 5 filtered events
			if eventCount >= 5 {
				fmt.Println("✅ Received 5 filtered events, stopping")
				return
			}

		case err := <-stream.Errors:
			log.Printf("❌ Configured stream error: %v", err)

		case <-stream.Done:
			fmt.Println("✅ Configured stream completed")
			return

		case <-timeout:
			fmt.Println("⏰ Advanced example timeout reached")
			return
		}
	}
}

// POST streaming with request data
func postStreamingExample(client *gorest.NetworkClient) {
	// Subscription data to send with the POST request
	subscriptionData := map[string]interface{}{
		"topics":    []string{"user-activity", "system-alerts"},
		"filter":    "priority=high",
		"client_id": "gorest-example-client",
	}

	// Start POST streaming with request body
	stream, err := client.
		Host("https://api.example.com").
		Body(subscriptionData).
		Headers(map[string]string{
			"Content-Type": "application/json",
			"Accept":       "text/event-stream",
		}).
		WithSSEConfig(&types.SSEConfig{
			BufferSize:      100,
			EnableReconnect: true,
		}).
		StreamPost("/subscribe")

	if err != nil {
		log.Printf("Failed to start POST stream: %v", err)
		return
	}

	defer stream.Close()

	// Process subscription events
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	for {
		select {
		case event := <-stream.Events:
			fmt.Printf("📮 Subscription Event: %s\n", event.Event)
			fmt.Printf("   Data: %s\n", event.Data)

			// Handle different event types
			switch event.Event {
			case "user-activity":
				fmt.Println("   👤 User activity detected")
			case "system-alert":
				fmt.Println("   🚨 System alert received")
			case "heartbeat":
				fmt.Println("   💓 Connection heartbeat")
			}

		case err := <-stream.Errors:
			log.Printf("❌ POST stream error: %v", err)

		case <-stream.Done:
			fmt.Println("✅ POST stream completed")
			return

		case <-ctx.Done():
			fmt.Println("⏰ POST streaming timeout reached")
			return
		}
	}
}

// Authenticated streaming with middleware integration
func authenticatedStreamingExample(client *gorest.NetworkClient) {
	// This demonstrates how SSE streaming works with your existing middleware
	// The middleware pipeline (auth, logging, metrics, etc.) runs as normal

	stream, err := client.
		Host("https://secure-api.example.com").
		Headers(map[string]string{
			"Authorization": "Bearer your-jwt-token-here",
			"X-Client-ID":   "gorest-streaming-client",
		}).
		WithSSEConfig(&types.SSEConfig{
			ReadTimeout:     30 * time.Second,
			BufferSize:      200,
			EnableReconnect: true,
			LastEventID:     "", // Could resume from a previous event
		}).
		StreamGet("/secure-events")

	if err != nil {
		log.Printf("Failed to start authenticated stream: %v", err)
		return
	}

	defer stream.Close()

	// Handle authenticated events with business logic
	eventProcessor := NewEventProcessor()
	timeout := time.After(30 * time.Second)

	for {
		select {
		case event := <-stream.Events:
			// Process events with your business logic
			if err := eventProcessor.ProcessEvent(event); err != nil {
				log.Printf("❌ Event processing error: %v", err)
			}

		case err := <-stream.Errors:
			log.Printf("❌ Authenticated stream error: %v", err)

			// Implement custom error handling
			if isAuthenticationError(err) {
				fmt.Println("🔐 Authentication error, need to refresh token")
				return
			}

		case <-stream.Done:
			fmt.Println("✅ Authenticated stream completed")
			return

		case <-timeout:
			fmt.Println("⏰ Authenticated streaming timeout reached")
			return
		}
	}
}

// Example event processor for business logic
type EventProcessor struct {
	processedCount int
}

func NewEventProcessor() *EventProcessor {
	return &EventProcessor{}
}

func (ep *EventProcessor) ProcessEvent(event types.SSEEvent) error {
	ep.processedCount++

	fmt.Printf("⚙️  Processing event #%d: %s\n", ep.processedCount, event.Event)

	// Add your business logic here
	switch event.Event {
	case "order-created":
		fmt.Println("   📦 New order received")
	case "payment-processed":
		fmt.Println("   💳 Payment completed")
	case "inventory-updated":
		fmt.Println("   📊 Inventory levels changed")
	default:
		fmt.Printf("   ℹ️  General event: %s\n", event.Data)
	}

	return nil
}

func isAuthenticationError(err error) bool {
	// Implement your authentication error detection logic
	return false
}
