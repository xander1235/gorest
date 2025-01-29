package network

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/xander1235/gorest/constants"
	"github.com/xander1235/gorest/constants/enums"
	"github.com/xander1235/gorest/exceptions"
	"github.com/xander1235/gorest/exceptions/errors"
	"github.com/xander1235/gorest/parsers"
	"github.com/xander1235/gorest/types"
	"go.uber.org/zap/buffer"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	NetworkClient = &networkClient{
		client: &http.Client{
			Timeout: time.Second * 100,
		},
		parser:      parsers.ParseResponse,
		errorParser: parsers.ParseError,
		requestType: enums.Json.ToString(),
	}
)

func NewApmWrapped(wrappedClient *http.Client) {
	NetworkClient = &networkClient{
		client:      wrappedClient,
		parser:      parsers.ParseResponse,
		errorParser: parsers.ParseError,
		requestType: enums.Json.ToString(),
	}
}

type networkClient struct {
	client      *http.Client
	headers     map[string]string
	params      map[string]string
	host        string
	body        any
	multipart   *types.MultipartBody
	parser      func(string, any) *errors.ErrorDetails
	errorParser func(string) *errors.ErrorDetails
	response    any
	requestType string
	ctx         context.Context
}

func (nc networkClient) Response(response any) networkClient {
	nc.response = response
	return nc
}

func (nc networkClient) Headers(headers map[string]string) networkClient {
	nc.headers = headers
	return nc
}

func (nc networkClient) Params(params map[string]string) networkClient {
	nc.params = params
	return nc
}

func (nc networkClient) Host(host string) networkClient {
	nc.host = host
	return nc
}

func (nc networkClient) Body(body any) networkClient {
	nc.body = body
	return nc
}

func (nc networkClient) MultipartBody(multipart *types.MultipartBody) networkClient {
	nc.multipart = multipart
	nc.requestType = enums.Multipart.ToString()
	return nc
}

func (nc networkClient) RequestType(requestType enums.RequestType) networkClient {
	nc.requestType = requestType.ToString()
	return nc
}

func (nc networkClient) Parser(parser func(string, any) *errors.ErrorDetails) networkClient {
	nc.parser = parser
	return nc
}

func (nc networkClient) ErrorParser(parser func(string) *errors.ErrorDetails) networkClient {
	nc.errorParser = parser
	return nc
}

func (nc networkClient) WithContext(ctx context.Context) networkClient {
	nc.ctx = ctx
	return nc
}

func (nc networkClient) Put(endpoint string) *errors.ErrorDetails {
	return nc.send(enums.PUT, endpoint)
}

func (nc networkClient) Delete(endpoint string) *errors.ErrorDetails {
	return nc.send(enums.DELETE, endpoint)
}

func (nc networkClient) Patch(endpoint string) *errors.ErrorDetails {
	return nc.send(enums.PATCH, endpoint)
}

func (nc networkClient) Post(endpoint string) *errors.ErrorDetails {
	return nc.send(enums.POST, endpoint)
}

func (nc networkClient) Get(endpoint string) *errors.ErrorDetails {
	return nc.send(enums.GET, endpoint)
}

func (nc networkClient) send(method enums.HttpMethods, endpoint string) *errors.ErrorDetails {
	switch nc.requestType {
	case enums.Json.ToString():
		return nc.sendJson(method, endpoint)
	case enums.Multipart.ToString():
		return nc.sendMultipart(method, endpoint)
	case enums.FormUrlEncoded.ToString():
		return nc.sendFormUrlEncoded(method, endpoint)
	default:
		return exceptions.GenericException(constants.InvalidRequestType, constants.InvalidRequestType, 500)
	}
}

func (nc networkClient) sendJson(method enums.HttpMethods, endpoint string) *errors.ErrorDetails {
	var jsonBytes buffer.Buffer
	var marshalErr error
	if nc.body != nil {
		marshalErr = json.NewEncoder(&jsonBytes).Encode(nc.body)
		if marshalErr != nil {
			return exceptions.GenericException(marshalErr.Error(), constants.SomethingWentWrong, 500)
		}
	}
	request, err := http.NewRequest(method.String(), nc.host+endpoint, bytes.NewBuffer(jsonBytes.Bytes()))

	if err != nil {
		return exceptions.GenericException(constants.SomethingWentWrong, err.Error(), 500)
	}

	return nc.sendRequest(request)

}

func (nc networkClient) sendMultipart(method enums.HttpMethods, endpoint string) *errors.ErrorDetails {
	var jsonBytes *bytes.Buffer
	var err error
	if nc.multipart != nil {
		jsonBytes, nc.requestType, err = nc.multipart.CreateBuffer()
		if err != nil {
			return exceptions.GenericException(err.Error(), constants.SomethingWentWrong, 500)
		}
	}
	request, err := http.NewRequest(method.String(), nc.host+endpoint, bytes.NewBuffer(jsonBytes.Bytes()))
	if err != nil {
		//configs.Sugar.Error(constants.SomethingWentWrongDownstream + err.Error())
		return exceptions.GenericException(err.Error(), constants.SomethingWentWrong, 500)
	}

	return nc.sendRequest(request)
}

func (nc networkClient) sendRequest(request *http.Request) *errors.ErrorDetails {
	request.Header = http.Header{
		constants.ContentType: {nc.requestType},
		constants.XRequestId:  {uuid.New().String()},
	}
	for key, value := range nc.headers {
		request.Header.Add(key, value)
	}

	queryParams := request.URL.Query()
	for key, value := range nc.params {
		queryParams.Add(key, value)
	}
	request.URL.RawQuery = queryParams.Encode()

	if nc.ctx != nil {
		request = request.WithContext(nc.ctx)
	}

	res, err := nc.client.Do(request)

	if err != nil {
		//configs.Sugar.Error(constants.SomethingWentWrongDownstream + err.Error())
		return exceptions.GenericException(err.Error(), constants.SomethingWentWrong, 500)
	} else {
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				//configs.Sugar.Error(constants.SomethingWentWrongDownstream + err.Error())
			}
		}(res.Body)

		bodyBytes, err := io.ReadAll(res.Body)

		if err != nil {
			//configs.Sugar.Error(err.Error())
			return exceptions.GenericException(err.Error(), constants.SomethingWentWrong, res.StatusCode)
		}
		bodyString := string(bodyBytes)
		var resBody bytes.Buffer
		err = json.Indent(&resBody, bodyBytes, "", "\t")
		if err == nil {
			bodyString = resBody.String()
		}
		uri := request.URL.String()
		uri = "<-- " + strconv.Itoa(res.StatusCode) + " : " + request.Method + " " + uri
		switch enums.HttpStatus(res.StatusCode).SeriesType() {
		case enums.Successful:
			if nc.response != nil {
				appErr := nc.parser(bodyString, nc.response)
				//configs.Sugar.Infow(uri + " success, Response: \n" + bodyString)
				return appErr
			}
			return nil
		case enums.ClientError:
			//configs.Sugar.Infow(uri + " failure, Response: \n" + bodyString)
			return exceptions.GenericException(nc.errorParser(bodyString).Message, bodyString, res.StatusCode)
		case enums.ServerError:
			//configs.Sugar.Infow(uri + " failure, Response: \n" + bodyString)
			return exceptions.GenericException(constants.SomethingWentWrong, bodyString, res.StatusCode)
		}
	}
	return nil
}

func (nc networkClient) sendFormUrlEncoded(method enums.HttpMethods, endpoint string) *errors.ErrorDetails {
	data := url.Values{}

	for k, v := range nc.body.(map[string]string) {
		data.Set(k, v)
	}

	// Encode the form data into a URL-encoded string
	encodedData := data.Encode()

	// Create a new HTTP request with the encoded data as the body
	request, err := http.NewRequest(method.String(), nc.host+endpoint, strings.NewReader(encodedData))
	if err != nil {
		//configs.Sugar.Error(constants.SomethingWentWrongDownstream + err.Error())
		return exceptions.GenericException(err.Error(), constants.SomethingWentWrong, 500)
	}

	return nc.sendRequest(request)

}
