// parsers package contains the parsing logic.
package parsers

import (
	"encoding/json"
	"github.com/xander1235/gorest/exceptions"
	"github.com/xander1235/gorest/exceptions/errors"
	"strings"
)

// ParseResponse parses the response from the server.
// It decodes the JSON response and returns an ErrorDetails instance.
// If the decoding fails, it returns a generic error.
func ParseResponse(value string, res any) *errors.ErrorDetails {
	unMarshalError := json.NewDecoder(strings.NewReader(value)).Decode(res)
	if unMarshalError != nil {
		return exceptions.GenericException(unMarshalError.Error(), unMarshalError, 500)
	}
	return nil
}
