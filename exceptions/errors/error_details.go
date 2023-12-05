package errors

type ErrorDetails struct {
	ErrorTimestamp int64  `json:"timestamp"`
	Message        string `json:"message"`
	Error          *any   `json:"error"`
	ResponseCode   int    `json:"response_code"`
}
