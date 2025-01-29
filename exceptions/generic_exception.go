package exceptions

import (
	"github.com/xander1235/gorest/exceptions/errors"
	"time"
)

func GenericException(message string, error any, httpStatus int) *errors.ErrorDetails {
	return &errors.ErrorDetails{
		ErrorTimestamp: time.Now().UnixMilli(),
		Message:        message,
		Error:          error,
		ResponseCode:   httpStatus,
	}
}
