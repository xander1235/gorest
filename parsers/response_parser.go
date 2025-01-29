package parsers

import (
	"encoding/json"
	"github.com/xander1235/gorest/exceptions"
	"github.com/xander1235/gorest/exceptions/errors"
	"strings"
)

func ParseResponse(value string, res any) *errors.ErrorDetails {
	unMarshalError := json.NewDecoder(strings.NewReader(value)).Decode(res)
	if unMarshalError != nil {
		return exceptions.GenericException(unMarshalError.Error(), unMarshalError, 500)
	}
	return nil
}
