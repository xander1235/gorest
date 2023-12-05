package networks

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/samber/lo"
	"github.com/xander1235/gorest/constants"
	"github.com/xander1235/gorest/constants/enums"
	"github.com/xander1235/gorest/exceptions"
	"github.com/xander1235/gorest/pojos"
	"io"
	"net/http"
	"strconv"
	"time"
)

var (
	IClient = &client{}
	Client  = &http.Client{
		Timeout: time.Second * 100,
	}
)

func NewApmWrappedClient(wrappedClient *http.Client) {
	Client = wrappedClient
}

type client struct {
	headers     map[string]any
	params      map[string]any
	host        string
	body        []byte
	parser      func(string, any) *any
	errorParser func(string) *any
	response    any
	ctx         context.Context
}

func (networkClient client) Response(response any) client {
	networkClient.response = response
	return networkClient
}

func (networkClient client) Headers(headers map[string]any) client {
	networkClient.headers = headers
	return networkClient
}

func (networkClient client) Params(params map[string]any) client {
	networkClient.params = params
	return networkClient
}

func (networkClient client) Host(host string) client {
	networkClient.host = host
	return networkClient
}

func (networkClient client) Body(body []byte) client {
	networkClient.body = body
	return networkClient
}

func (networkClient client) Parser(parser func(string, any) *any) client {
	networkClient.parser = parser
	return networkClient
}

func (networkClient client) ErrorParser(parser func(string) *any) client {
	networkClient.errorParser = parser
	return networkClient
}

func (networkClient client) WithContext(ctx context.Context) client {
	networkClient.ctx = ctx
	return networkClient
}

func (networkClient client) Put(endpoint string, token string) *pojos.ResponseData {
	return networkClient.send(enums.PUT, endpoint, token)
}

func (networkClient client) Delete(endpoint string, token string) *pojos.ResponseData {
	return networkClient.send(enums.DELETE, endpoint, token)
}

func (networkClient client) Patch(endpoint string, token string) *pojos.ResponseData {
	return networkClient.send(enums.PATCH, endpoint, token)
}

func (networkClient client) Post(endpoint string, token string) *pojos.ResponseData {
	return networkClient.send(enums.POST, endpoint, token)
}

func (networkClient client) Get(endpoint string, token string) *pojos.ResponseData {
	return networkClient.send(enums.GET, endpoint, token)
}

func (networkClient client) send(method enums.HttpMethods, endpoint string, token string) *pojos.ResponseData {
	request, err := http.NewRequest(method.String(), networkClient.host+endpoint, bytes.NewBuffer(networkClient.body))
	request.Header = http.Header{
		"Content-Type": {"application/json"},
		"X-AUTH-TOKEN": {token},
		"X-CLIENT-ID":  {"aether"},
	}
	if networkClient.headers != nil {
		for _, key := range lo.Keys(networkClient.headers) {
			if value, ok := networkClient.headers[key]; ok {
				request.Header.Add(key, value.(string))
			}
		}
	}

	queryParams := request.URL.Query()

	if networkClient.params != nil {
		for _, key := range lo.Keys(networkClient.params) {
			if value, ok := networkClient.params[key]; ok {
				queryParams.Add(key, value.(string))
			}
		}
	}

	request.URL.RawQuery = queryParams.Encode()

	res, err := Client.Do(request.WithContext(networkClient.ctx))
	code := 500
	if err != nil {
		//utils.Sugar.Error("Something went wrong while calling downstream service: " + err.Error())
		return &pojos.ResponseData{
			Error:        exceptions.GenericException("Something went wrong while calling downstream service: "+err.Error(), nil, code),
			ResponseCode: &code,
		}
	} else {
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				//utils.Sugar.Error("Something went wrong while calling downstream service: " + err.Error())
			}
		}(res.Body)

		bodyBytes, err := io.ReadAll(res.Body)

		if err != nil {
			//utils.Sugar.Error(err.Error())
			return &pojos.ResponseData{
				Error:        exceptions.GenericException("Something went wrong while calling downstream service: "+err.Error(), nil, code),
				ResponseCode: &code,
			}
		}
		bodyString := string(bodyBytes)
		var resBody bytes.Buffer
		err = json.Indent(&resBody, bodyBytes, "", "\t")
		if err == nil {
			bodyString = string(resBody.Bytes())
		}
		uri := request.URL.String()
		uri = "<-- " + strconv.Itoa(res.StatusCode) + " : " + method.String() + " " + uri
		switch enums.HttpStatus(res.StatusCode).SeriesType() {
		case enums.Successful:
			if networkClient.response != nil && networkClient.parser != nil {
				appErr := networkClient.parser(bodyString, networkClient.response)
				//utils.Sugar.Infow(uri + " success, Response: \n" + bodyString)
				return &pojos.ResponseData{
					ResponseCode: &res.StatusCode,
					Error:        appErr,
				}
			}
			if networkClient.response != nil {
				//utils.Sugar.Infow(uri + " success, Response: \n" + bodyString)
				networkClient.response = bodyString
				return &pojos.ResponseData{
					ResponseCode: &res.StatusCode,
					Response:     bodyString,
				}
			}
			return nil
		case enums.ClientError:
			//utils.Sugar.Infow(uri + " failure, Response: \n" + bodyString)
			networkClient.response = bodyString
			if networkClient.errorParser != nil {
				return &pojos.ResponseData{
					ResponseCode: &res.StatusCode,
					Error:        exceptions.GenericException("", networkClient.errorParser(bodyString), res.StatusCode),
				}
			}
			return &pojos.ResponseData{
				ResponseCode: &res.StatusCode,
				Error:        exceptions.GenericException(bodyString, nil, res.StatusCode),
				Response:     bodyString,
			}

		case enums.ServerError:
			//utils.Sugar.Infow(uri + " failure, Response: \n" + bodyString)
			networkClient.response = bodyString
			return &pojos.ResponseData{
				ResponseCode: &res.StatusCode,
				Error:        exceptions.GenericException(constants.SomethingWentWrong, nil, res.StatusCode),
				Response:     bodyString,
			}
		}
	}
	return nil
}
