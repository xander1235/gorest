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

// NewApmWrapped initializes the networkClient with a wrapped HTTP client.
// This allows for custom HTTP client configurations.
//
// Parameters:
// - wrappedClient: A custom HTTP client to use.
func NewApmWrapped(wrappedClient *http.Client) {
	NetworkClient = &networkClient{
		client:      wrappedClient,
		parser:      parsers.ParseResponse,
		errorParser: parsers.ParseError,
		requestType: enums.Json.ToString(),
	}
}

// networkClient is a custom HTTP client that provides methods for making HTTP requests with configurable headers, parameters, and body.
// It supports JSON, multipart, and form URL encoded request types.
//
// Example:
// client := &networkClient{}
// client.Headers(map[string]string{"Authorization": "Bearer token"})
// response := client.Get("/api/resource")
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

// Response sets the response for the networkClient.
// This method is chainable and returns the updated networkClient.
//
// Parameters:
// - response: The response object to set.
func (nc networkClient) Response(response any) networkClient {
	nc.response = response
	return nc
}

// Headers sets custom headers for the networkClient.
// This method is chainable and returns the updated networkClient.
//
// Parameters:
// - headers: A map of header key-value pairs.
func (nc networkClient) Headers(headers map[string]string) networkClient {
	nc.headers = headers
	return nc
}

// Params sets query parameters for the networkClient.
// This method is chainable and returns the updated networkClient.
//
// Parameters:
// - params: A map of query parameter key-value pairs.
func (nc networkClient) Params(params map[string]string) networkClient {
	nc.params = params
	return nc
}

// Host sets the base URL for the networkClient.
// This method is chainable and returns the updated networkClient.
//
// Parameters:
// - host: The base URL for the requests.
func (nc networkClient) Host(host string) networkClient {
	nc.host = host
	return nc
}

// Body sets the body of the request for the networkClient.
// This method is chainable and returns the updated networkClient.
//
// Parameters:
// - body: The body of the request.
func (nc networkClient) Body(body any) networkClient {
	nc.body = body
	return nc
}

// MultipartBody sets the multipart body for the networkClient.
// This method is chainable and updates the request type to multipart.
//
// Parameters:
// - multipart: The multipart body to set.
func (nc networkClient) MultipartBody(multipart *types.MultipartBody) networkClient {
	nc.multipart = multipart
	nc.requestType = enums.Multipart.ToString()
	return nc
}

// RequestType sets the type of request (e.g., JSON, multipart).
// This method is chainable and returns the updated networkClient.
//
// Parameters:
// - requestType: The type of request to set.
func (nc networkClient) RequestType(requestType enums.RequestType) networkClient {
	nc.requestType = requestType.ToString()
	return nc
}

// Parser sets the function used to parse responses.
// This method is chainable and returns the updated networkClient.
//
// Parameters:
// - parser: The function to parse responses.
func (nc networkClient) Parser(parser func(string, any) *errors.ErrorDetails) networkClient {
	nc.parser = parser
	return nc
}

// ErrorParser sets the function used to parse errors.
// This method is chainable and returns the updated networkClient.
//
// Parameters:
// - parser: The function to parse errors.
func (nc networkClient) ErrorParser(parser func(string) *errors.ErrorDetails) networkClient {
	nc.errorParser = parser
	return nc
}

// WithContext sets the context for the networkClient.
// This method is chainable and returns the updated networkClient.
//
// Parameters:
// - ctx: The context to set.
func (nc networkClient) WithContext(ctx context.Context) networkClient {
	nc.ctx = ctx
	return nc
}

// Put sends a PUT request to the specified endpoint.
// This method is a convenience wrapper around the Send method.
//
// Parameters:
// - endpoint: The endpoint to send the request to.
func (nc networkClient) Put(endpoint string) *errors.ErrorDetails {
	return nc.send(enums.PUT, endpoint)
}

// Delete sends a DELETE request to the specified endpoint.
// This method is a convenience wrapper around the Send method.
//
// Parameters:
// - endpoint: The endpoint to send the request to.
func (nc networkClient) Delete(endpoint string) *errors.ErrorDetails {
	return nc.send(enums.DELETE, endpoint)
}

// Patch sends a PATCH request to the specified endpoint.
// This method is a convenience wrapper around the Send method.
//
// Parameters:
// - endpoint: The endpoint to send the request to.
func (nc networkClient) Patch(endpoint string) *errors.ErrorDetails {
	return nc.send(enums.PATCH, endpoint)
}

// Post sends a POST request to the specified endpoint.
// This method is a convenience wrapper around the Send method.
//
// Parameters:
// - endpoint: The endpoint to send the request to.
func (nc networkClient) Post(endpoint string) *errors.ErrorDetails {
	return nc.send(enums.POST, endpoint)
}

// Get sends a GET request to the specified endpoint.
// This method is a convenience wrapper around the Send method.
//
// Parameters:
// - endpoint: The endpoint to send the request to.
func (nc networkClient) Get(endpoint string) *errors.ErrorDetails {
	return nc.send(enums.GET, endpoint)
}

// Send sends an HTTP request based on the configured parameters and body.
// It determines the request type and calls the appropriate method to send the request.
//
// Parameters:
// - method: The HTTP method to use (GET, POST, etc.).
// - endpoint: The endpoint to send the request to.
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

// SendJson sends a JSON request to the specified endpoint.
// This method is called by the send method when the request type is JSON.
//
// Parameters:
// - method: The HTTP method to use.
// - endpoint: The endpoint to send the request to.
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

// SendMultipart sends a multipart request to the specified endpoint.
// This method is called by the send method when the request type is multipart.
//
// Parameters:
// - method: The HTTP method to use.
// - endpoint: The endpoint to send the request to.
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

// SendRequest sends the actual HTTP request and handles the response.
// It sets the headers and query parameters before executing the request.
//
// Parameters:
// - request: The HTTP request to send.
func (nc networkClient) sendRequest(request *http.Request) *errors.ErrorDetails {
	request.Header = http.Header{
		constants.ContentType: []string{nc.requestType},
		constants.XRequestId:  []string{uuid.New().String()},
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
			_ = Body.Close() // Intentionally ignoring error as we can't do much with it during deferred close
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
		// Build request information string for potential logging
		_ = "<-- " + strconv.Itoa(res.StatusCode) + " : " + request.Method + " " + request.URL.String()
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

// SendFormUrlEncoded sends a form URL encoded request to the specified endpoint.
// This method is called by the send method when the request type is form URL encoded.
//
// Parameters:
// - method: The HTTP method to use.
// - endpoint: The endpoint to send the request to.
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
