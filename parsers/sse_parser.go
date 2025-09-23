package parsers

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xander1235/gorest/v2/types"
)

// SSEParser handles parsing of Server-Sent Event streams
type SSEParser struct {
	config *types.SSEConfig
	reader *bufio.Reader
}

// NewSSEParser creates a new SSE parser with the given configuration
func NewSSEParser(config *types.SSEConfig) *SSEParser {
	if config == nil {
		config = types.DefaultSSEConfig()
	}

	return &SSEParser{
		config: config,
	}
}

// ParseStream starts parsing an SSE stream from the HTTP response
// Returns an SSEStream that provides channels for events, errors, and completion
func (p *SSEParser) ParseStream(ctx context.Context, resp *http.Response) *types.SSEStream {
	// Ensure we have a valid context - use background if nil
	if ctx == nil {
		ctx = context.Background()
	}
	streamCtx, cancel := context.WithCancel(ctx)

	stream := &types.SSEStream{
		Events:   make(chan types.SSEEvent, p.config.BufferSize),
		Errors:   make(chan error, 10),
		Done:     make(chan struct{}),
		Context:  streamCtx,
		Cancel:   cancel,
		Response: resp,
	}

	p.reader = bufio.NewReader(resp.Body)

	// Start parsing in a separate goroutine
	go p.parseLoop(stream)

	return stream
}

// parseLoop continuously reads and parses events from the stream
func (p *SSEParser) parseLoop(stream *types.SSEStream) {
	defer func() {
		close(stream.Events)
		close(stream.Errors)
		// Note: We don't call stream.Close() here to avoid double-close
		// The caller is responsible for closing the stream
	}()

	var event types.SSEEvent
	var fieldBuffer strings.Builder

	for {
		select {
		case <-stream.Context.Done():
			return
		default:
			// Set read timeout if configured
			if p.config.ReadTimeout > 0 {
				if conn := stream.Response.Body; conn != nil {
					if tcpConn, ok := conn.(interface{ SetReadDeadline(time.Time) error }); ok {
						_ = tcpConn.SetReadDeadline(time.Now().Add(p.config.ReadTimeout))
					}
				}
			}

			line, err := p.reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					// Stream ended, check if we should reconnect
					if p.config.EnableReconnect {
						select {
						case stream.Errors <- fmt.Errorf("stream ended, attempting reconnection"):
						case <-stream.Context.Done():
							return
						}
					}
					return
				}

				select {
				case stream.Errors <- fmt.Errorf("error reading stream: %w", err):
				case <-stream.Context.Done():
					return
				}
				return
			}

			// Remove trailing newline
			line = strings.TrimRight(line, "\r\n")

			// Add to raw field buffer
			if fieldBuffer.Len() > 0 {
				fieldBuffer.WriteString("\n")
			}
			fieldBuffer.WriteString(line)

			// Empty line indicates end of event
			if line == "" {
				if fieldBuffer.Len() > 0 {
					event.Raw = fieldBuffer.String()
					event.Timestamp = time.Now()

					// Apply event filter if configured
					if p.config.EventFilter == nil || p.config.EventFilter(event) {
						select {
						case stream.Events <- event:
						case <-stream.Context.Done():
							return
						}
					}

					// Reset for next event
					event = types.SSEEvent{}
					fieldBuffer.Reset()
				}
				continue
			}

			// Parse field
			if !strings.HasPrefix(line, ":") { // Skip comments
				p.parseField(line, &event)
			}
		}
	}
}

// parseField parses a single SSE field and updates the event
func (p *SSEParser) parseField(line string, event *types.SSEEvent) {
	colonIndex := strings.Index(line, ":")
	if colonIndex == -1 {
		// Field name only, no value
		field := strings.TrimSpace(line)
		p.setFieldValue(field, "", event)
		return
	}

	field := strings.TrimSpace(line[:colonIndex])
	value := line[colonIndex+1:]

	// Remove leading space from value if present
	value = strings.TrimPrefix(value, " ")

	p.setFieldValue(field, value, event)
}

// setFieldValue sets the appropriate field in the SSE event
func (p *SSEParser) setFieldValue(field, value string, event *types.SSEEvent) {
	switch field {
	case "event":
		event.Event = value
	case "data":
		if event.Data != "" {
			event.Data += "\n" + value
		} else {
			event.Data = value
		}
	case "id":
		event.ID = value
	case "retry":
		if retryMs, err := strconv.Atoi(value); err == nil {
			event.Retry = retryMs
		}
	}
}

// ParseSingleEvent parses a single SSE event from a string
// Useful for testing or processing individual events
func ParseSingleEvent(eventData string) types.SSEEvent {
	event := types.SSEEvent{
		Raw:       eventData,
		Timestamp: time.Now(),
	}

	lines := strings.Split(eventData, "\n")
	for _, line := range lines {
		line = strings.TrimRight(line, "\r")

		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		parser := &SSEParser{}
		parser.parseField(line, &event)
	}

	return event
}
