package errors

// ErrorDetails represents the details of an error.
// It is defined as a struct for better type safety.
type ErrorDetails struct {
	ErrorTimestamp int64  `json:"timestamp"`
	Message        string `json:"message"`
	Error          any    `json:"error"`
	ResponseCode   int    `json:"response_code"`
}
