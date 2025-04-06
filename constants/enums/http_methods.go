package enums

// HttpMethods represents the HTTP methods used in requests.
// It is defined as a string type for better type safety.
type HttpMethods string

const (
	// POST represents the HTTP POST method.
	POST HttpMethods = "POST"

	// PUT represents the HTTP PUT method.
	PUT HttpMethods = "PUT"

	// DELETE represents the HTTP DELETE method.
	DELETE HttpMethods = "DELETE"

	// PATCH represents the HTTP PATCH method.
	PATCH HttpMethods = "PATCH"

	// GET represents the HTTP GET method.
	GET HttpMethods = "GET"
)

// String returns the string representation of the HTTP method.
func (httpMethod HttpMethods) String() string {
	return string(httpMethod)
}
