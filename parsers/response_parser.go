package parsers

import (
	"aether_go/exceptions"
	"aether_go/exceptions/errors"
	"aether_go/pojos"
	"encoding/json"
	"go.uber.org/zap/buffer"
	"strings"
)

func ParseBaseResponse(value string, res any) *errors.ThreedotsError {
	var baseResponse pojos.BaseResponse
	unMarshalError := json.NewDecoder(strings.NewReader(value)).Decode(&baseResponse)
	if unMarshalError != nil {
		return exceptions.InternalServerException(unMarshalError.Error())
	}

	var jsonBytes buffer.Buffer
	encodingError := json.NewEncoder(&jsonBytes).Encode(baseResponse.Data)
	if encodingError != nil {
		return exceptions.InternalServerException(encodingError.Error())
	}
	unMarshalError = json.NewDecoder(strings.NewReader(jsonBytes.String())).Decode(res)
	if unMarshalError != nil {
		return exceptions.InternalServerException(unMarshalError.Error())
	}
	return nil
}

func ParseResponse(value string, res any) *errors.ThreedotsError {
	unMarshalError := json.NewDecoder(strings.NewReader(value)).Decode(res)
	if unMarshalError != nil {
		return exceptions.InternalServerException(unMarshalError.Error())
	}
	return nil
}
