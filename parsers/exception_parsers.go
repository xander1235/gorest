package parsers

import (
	"encoding/json"
	"github.com/xander1235/gorest/constants"
	"github.com/xander1235/gorest/exceptions"
	"github.com/xander1235/gorest/exceptions/errors"
	"strings"
	"time"
)

func ParseError(value string) *errors.ErrorDetails {
	var genError errors.ErrorDetails
	unmarshalErr := json.NewDecoder(strings.NewReader(value)).Decode(&genError)
	if unmarshalErr != nil {
		return exceptions.GenericException(constants.SomethingWentWrong, nil, 500)
	}
	return &errors.ErrorDetails{
		ErrorTimestamp: time.Now().UTC().Unix(),
		Message:        genError.Message,
		ResponseCode:   genError.ResponseCode,
		Error:          value,
	}
}
