package types

import (
	"context"
	"net/http"
	"time"
)

// SSEEvent represents a single Server-Sent Event
type SSEEvent struct {
	// Event type (e.g., "message", "update", "error")
	Event string
	// Data payload of the event
	Data string
	// Unique identifier for this event
	ID string
	// Retry interval in milliseconds (client should wait before reconnecting)
	Retry int
	// Raw event data as received
	Raw string
	// Timestamp when event was received
	Timestamp time.Time
}

// SSEStream represents an active Server-Sent Events stream
type SSEStream struct {
	// Events channel for receiving SSE events
	Events chan SSEEvent
	// Errors channel for receiving stream errors
	Errors chan error
	// Done channel signals when stream is closed
	Done chan struct{}
	// Context for stream cancellation
	Context context.Context
	// CancelFunc to stop the stream
	Cancel context.CancelFunc
	// Response associated with the stream
	Response *http.Response
}

// SSEConfig contains configuration for SSE streams
type SSEConfig struct {
	// ReadTimeout for individual event reads
	ReadTimeout time.Duration
	// ReconnectInterval for automatic reconnection attempts
	ReconnectInterval time.Duration
	// MaxReconnectAttempts before giving up (-1 for unlimited)
	MaxReconnectAttempts int
	// LastEventID for resuming streams (sent as Last-Event-ID header)
	LastEventID string
	// BufferSize for the events channel
	BufferSize int
	// EnableReconnect whether to automatically reconnect on connection loss
	EnableReconnect bool
	// OnReconnect callback function called before reconnection attempts
	OnReconnect func(attempt int, lastEventID string)
	// EventFilter function to filter events before sending to channel
	EventFilter func(event SSEEvent) bool
}

// DefaultSSEConfig returns default configuration for SSE streams
func DefaultSSEConfig() *SSEConfig {
	return &SSEConfig{
		ReadTimeout:          30 * time.Second,
		ReconnectInterval:    3 * time.Second,
		MaxReconnectAttempts: 5,
		BufferSize:           100,
		EnableReconnect:      true,
	}
}

// Close gracefully closes the SSE stream
func (s *SSEStream) Close() error {
	if s.Cancel != nil {
		s.Cancel()
	}

	// Close channels
	select {
	case <-s.Done:
		// Already closed
	default:
		close(s.Done)
	}

	// Close response body if available
	if s.Response != nil && s.Response.Body != nil {
		return s.Response.Body.Close()
	}

	return nil
}

// IsActive returns whether the stream is still active
func (s *SSEStream) IsActive() bool {
	select {
	case <-s.Done:
		return false
	default:
		return true
	}
}
