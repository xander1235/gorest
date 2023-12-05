package parsers

import (
	"aether_go/constants"
	"aether_go/exceptions"
	"aether_go/exceptions/errors"
	"aether_go/pojos"
	"encoding/json"
	"go.uber.org/zap/buffer"
	"strings"
	"time"
)

func ParseThreedotsBaseResponseError(value string) *errors.ThreedotsError {
	var baseResponse pojos.BaseResponse
	unmarshalErr := json.NewDecoder(strings.NewReader(value)).Decode(&baseResponse)
	if unmarshalErr != nil {
		return exceptions.InternalServerException(constants.SomethingWentWrong)
	}
	var jsonBytes buffer.Buffer
	encodingErr := json.NewEncoder(&jsonBytes).Encode(baseResponse.Data)
	if encodingErr != nil {
		return exceptions.InternalServerException(constants.SomethingWentWrong)
	}
	var threedotsError errors.ErrorDetails
	unmarshalErr2 := json.NewDecoder(strings.NewReader(jsonBytes.String())).Decode(&threedotsError)
	if unmarshalErr2 != nil {
		return exceptions.InternalServerException(constants.SomethingWentWrong)
	}
	return &errors.ThreedotsError{
		ErrorTimestamp: time.Now().UTC().Unix(),
		Message:        threedotsError.Message,
		ResponseCode:   threedotsError.ResponseCode,
	}
}

func ParseThreedotsError(value string) *errors.ThreedotsError {
	var threedotsError errors.ErrorDetails
	unmarshalErr := json.NewDecoder(strings.NewReader(value)).Decode(&threedotsError)
	if unmarshalErr != nil {
		return exceptions.InternalServerException(constants.SomethingWentWrong)
	}
	return &errors.ThreedotsError{
		ErrorTimestamp: time.Now().UTC().Unix(),
		Message:        threedotsError.Message,
		ResponseCode:   threedotsError.ResponseCode,
	}
}
