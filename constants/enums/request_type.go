package enums

// RequestType represents the type of HTTP request.
// It is defined as an int type for better type safety.
type RequestType int

const (
	// RequestTypeType represents the type of HTTP request.
	RequestTypeType RequestType = iota
	Json
	Multipart
	FormUrlEncoded
)

// Values returns the values of the RequestType.
func (s RequestType) Values() []string {
	return []string{"application/json", "multipart/form-data", "application/x-www-form-urlencoded"}
}

// ToString returns the string representation of the RequestType.
func (s RequestType) ToString() string {
	return s.Values()[s-1]
}

// ValueOf returns the RequestType value for the given string.
func (s RequestType) ValueOf(value string) RequestType {
	for i, g := range s.Values() {
		if g == value {
			return RequestType(i + 1)
		}
	}
	return RequestType(-1)
}
