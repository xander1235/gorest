// types package contains the type definitions.
package types

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/xander1235/gorest/constants"
	"io"
	"mime/multipart"
	"net/textproto"
	"os"
)

// MultipartBody represents a multipart form data body.
type MultipartBody struct {
	Parts []Part
}

// Part represents a part of the multipart form data.
type Part struct {
	Name               string
	ContentType        string
	Value              any
	IncludeContentType bool
}

// AddMultipartFile adds a file to the multipart body.
//
// Parameters:
// - name: The name of the file.
// - value: The file to add.
func (b *MultipartBody) AddMultipartFile(name string, value multipart.FileHeader) {
	b.Parts = append(b.Parts, Part{
		Name:               name,
		ContentType:        "file",
		Value:              value,
		IncludeContentType: false,
	})
}

// Add adds a string value to the multipart body.
//
// Parameters:
// - name: The name of the value.
// - value: The string value to add.
func (b *MultipartBody) Add(name string, value string) {
	b.Parts = append(b.Parts, Part{
		Name:               name,
		ContentType:        "text/plain",
		Value:              value,
		IncludeContentType: false,
	})
}

// AddWithContentType adds a value with a custom content type to the multipart body.
//
// Parameters:
// - name: The name of the value.
// - value: The value to add.
// - contentType: The content type of the value.
func (b *MultipartBody) AddWithContentType(name string, value any, contentType string) {
	b.Parts = append(b.Parts, Part{
		Name:               name,
		ContentType:        contentType,
		Value:              value,
		IncludeContentType: true,
	})
}

// AddFile adds a file to the multipart body.
//
// Parameters:
// - name: The name of the file.
// - value: The file to add.
func (b *MultipartBody) AddFile(name string, value os.File) {
	b.Parts = append(b.Parts, Part{
		Name:               name,
		ContentType:        "osFile",
		Value:              value,
		IncludeContentType: false,
	})
}

// CreateBuffer creates a buffer for the multipart body.
//
// Returns:
// - A pointer to the buffer.
// - The content type of the body.
// - An error if the creation fails.
func (b *MultipartBody) CreateBuffer() (*bytes.Buffer, string, error) {
	// Create a buffer for the multipart body
	var buf = &bytes.Buffer{}
	// Create a new writer for the multipart body
	writer := multipart.NewWriter(buf)

	// Iterate over the parts
	for _, part := range b.Parts {
		// Create a new header for the part
		h := make(textproto.MIMEHeader)
		h.Set(constants.ContentDisposition, fmt.Sprintf(`form-data; name="%s"`, part.Name))
		
		// Set the content type if required
		if part.IncludeContentType {
			h.Set(constants.ContentType, part.ContentType)
		}

		// Create a new writer for the part
		switch part.ContentType {
		case "text/plain":
			// Create a new writer for the part 
			partWriter, err := writer.CreatePart(h)
			if err != nil {
				return nil, "", err
			}

			// Write the value to the part
			_, err = partWriter.Write([]byte(part.Value.(string)))
			if err != nil {
				return nil, "", err
			}
		case "application/json":
			// Create a new writer for the part
			partWriter, err := writer.CreatePart(h)
			if err != nil {
				return nil, "", err
			}
			//var jsonBytes bytes.Buffer
			jsonBytes, marshalErr := json.Marshal(part.Value)
			if marshalErr != nil {
				return nil, "", marshalErr
			}

			// Write the value to the part
			_, err = partWriter.Write(jsonBytes)
			if err != nil {
				return nil, "", err
			}
		case "osFile":
			// Create a new writer for the part
			v := part.Value.(os.File)
			fileInfo, err := v.Stat()
			if err != nil {
				return nil, "", err
			}
			h.Set(constants.ContentDisposition, fmt.Sprintf(`form-data; name="%s"; filename="%s"`, part.Name, fileInfo.Name()))
			
			// Create a new writer for the part
			partWriter, err := writer.CreateFormFile(part.Name, fileInfo.Name())
			if err != nil {
				return nil, "", err
			}
			
			// Write the value to the part
			_, err = io.Copy(partWriter, &v)
			if err != nil {
				return nil, "", err
			}
			err = v.Close()
			if err != nil {
				return nil, "", err
			} // Manually close the file here
		case "file":
			// Create a new writer for the part
			v := part.Value.(multipart.FileHeader)
			h.Set(constants.ContentDisposition, fmt.Sprintf(`form-data; name="%s"; filename="%s"`, part.Name, v.Filename))
			
			// Create a new writer for the part
			partWriter, err := writer.CreateFormFile(part.Name, v.Filename)
			if err != nil {
				return nil, "", err
			}
			
			// Open the file
			file, err := v.Open()
			if err != nil {
				return nil, "", err
			}
			
			// Write the file to the part
			_, err = io.Copy(partWriter, file)
			if err != nil {
				err = file.Close()
				if err != nil {
					return nil, "", err
				} // Ensure file is closed before returning
				return nil, "", err
			}
			err = file.Close()
			if err != nil {
				return nil, "", err
			} // Manually close the file here
		default:
			return nil, "", fmt.Errorf("unsupported part type: %T", part.ContentType)
		}
	}

	// Close the writer
	err := writer.Close()
	if err != nil {
		return nil, "", err
	}

	// Return the buffer and content type
	return buf, writer.FormDataContentType(), nil
}
