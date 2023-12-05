package enums

type HttpMethods string

const (
	POST   HttpMethods = "POST"
	PUT    HttpMethods = "PUT"
	DELETE HttpMethods = "DELETE"
	PATCH  HttpMethods = "PATCH"
	GET    HttpMethods = "GET"
)

func (httpMethod HttpMethods) String() string {
	return string(httpMethod)
}
