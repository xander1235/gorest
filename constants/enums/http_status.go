package enums

// HttpStatus represents the HTTP status codes.
// It is defined as an int type for better type safety.
type HttpStatus int

const (
	// Informational represents the 1xx status codes.
	Informational HttpStatus = 1
	// Successful represents the 2xx status codes.
	Successful    HttpStatus = 2
	Redirection   HttpStatus = 3
	ClientError   HttpStatus = 4
	ServerError   HttpStatus = 5
)

// Is1XXSeries checks if the status code is in the 1xx series.
func (status HttpStatus) Is1XXSeries() bool {
	return status/100 == Informational
}

// Is2XXSeries checks if the status code is in the 2xx series.
func (status HttpStatus) Is2XXSeries() bool {
	return status/100 == Successful
}

// Is3XXSeries checks if the status code is in the 3xx series.
func (status HttpStatus) Is3XXSeries() bool {
	return status/100 == Redirection
}

// Is4XXSeries checks if the status code is in the 4xx series.
func (status HttpStatus) Is4XXSeries() bool {
	return status/100 == ClientError
}

// Is5XXSeries checks if the status code is in the 5xx series.
func (status HttpStatus) Is5XXSeries() bool {
	return status/100 == ServerError
}

// SeriesType returns the series type of the status code.
func (status HttpStatus) SeriesType() HttpStatus {
	return status / 100
}
