// exceptions package contains the exception handling logic.
package exceptions

import (
	"github.com/xander1235/gorest/v2/exceptions/errors"
	"time"
)

// GenericException creates a new ErrorDetails instance with the given message, error, and HTTP status code.
func GenericException(message string, error error, httpStatus int) *errors.ErrorDetails {
	return &errors.ErrorDetails{
		ErrorTimestamp: time.Now().UnixMilli(),
		Message:        message,
		Error:          error,
		ResponseCode:   httpStatus,
	}
}
