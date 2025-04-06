package types

// ResponseData represents the response data structure.
type ResponseData struct {
	ResponseCode *int
	Error        any
	Response     string
}
