# Gorest

## Overview
Gorest is a Go library for making HTTP requests with support for JSON, multipart, and form URL-encoded request types.

## Features
- Supports JSON, multipart, and form URL-encoded request types
- Easy-to-use client with customizable headers, parameters, and body
- Context support for request cancellation and timeouts
- Ability to add APM wrappers for the client

## Installation
To install the project, use the following command:

```sh
go get -u github.com/xander1235/gorest
```

## Usage

### JSON request example:

```go
package main

import (
    "context"
    "github.com/xander1235/gorest/networks"
)

func main() {
    client := networks.NetworkClient.
        Host("https://api.example.com").
        Headers(map[string]string{"Authorization": "Bearer token"}).
        WithContext(context.Background())

    client.Body(map[string]string{"key": "value"}).Post("/json-endpoint")
}
```

### Multipart request example:

```go
package main

import (
    "context"
    "github.com/xander1235/gorest/networks"
    "github.com/xander1235/gorest/types"
)

func main() {
    client := networks.NetworkClient.
        Host("https://api.example.com").
        Headers(map[string]string{"Authorization": "Bearer token"}).
        WithContext(context.Background())

    multipartBody := &types.MultipartBody{}
    multipartBody.Add("field", "value")
    client.MultipartBody(multipartBody).Post("/multipart-endpoint")
}
```

### Form URL-encoded request example:

```go
package main

import (
    "context"
    "github.com/xander1235/gorest/networks"
    "github.com/xander1235/gorest/constants/enums"
)

func main() {
    client := networks.NetworkClient.
        Host("https://api.example.com").
        Headers(map[string]string{"Authorization": "Bearer token"}).
        WithContext(context.Background())

    client.Body(map[string]string{"key": "value"}).RequestType(enums.FormUrlEncoded).Post("/form-endpoint")
}
```

## Contributing

If you would like to contribute, please fork the repository and use a feature branch. Pull requests are warmly welcome.

## License
This project is released under the [MIT License](https://opensource.org/licenses/MIT).

## Contact
If you have any questions or feedback, please contact [xander1235](https://github.com/xander1235).
