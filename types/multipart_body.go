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

type MultipartBody struct {
	Parts []Part
}

type Part struct {
	Name               string
	ContentType        string
	Value              any
	IncludeContentType bool
}

func (b *MultipartBody) AddMultipartFile(name string, value multipart.FileHeader) {
	b.Parts = append(b.Parts, Part{
		Name:               name,
		ContentType:        "file",
		Value:              value,
		IncludeContentType: false,
	})
}

func (b *MultipartBody) Add(name string, value string) {
	b.Parts = append(b.Parts, Part{
		Name:               name,
		ContentType:        "text/plain",
		Value:              value,
		IncludeContentType: false,
	})
}

func (b *MultipartBody) AddWithContentType(name string, value any, contentType string) {
	b.Parts = append(b.Parts, Part{
		Name:               name,
		ContentType:        contentType,
		Value:              value,
		IncludeContentType: true,
	})
}

func (b *MultipartBody) AddFile(name string, value os.File) {
	b.Parts = append(b.Parts, Part{
		Name:               name,
		ContentType:        "osFile",
		Value:              value,
		IncludeContentType: false,
	})
}

func (b *MultipartBody) CreateBuffer() (*bytes.Buffer, string, error) {
	var buf = &bytes.Buffer{}
	writer := multipart.NewWriter(buf)

	for _, part := range b.Parts {

		h := make(textproto.MIMEHeader)
		h.Set(constants.ContentDisposition, fmt.Sprintf(`form-data; name="%s"`, part.Name))
		if part.IncludeContentType {
			h.Set(constants.ContentType, part.ContentType)
		}

		switch part.ContentType {
		case "text/plain":
			partWriter, err := writer.CreatePart(h)
			if err != nil {
				return nil, "", err
			}
			_, err = partWriter.Write([]byte(part.Value.(string)))
			if err != nil {
				return nil, "", err
			}
		case "application/json":
			partWriter, err := writer.CreatePart(h)
			if err != nil {
				return nil, "", err
			}
			//var jsonBytes bytes.Buffer
			jsonBytes, marshalErr := json.Marshal(part.Value)
			if marshalErr != nil {
				return nil, "", marshalErr
			}
			_, err = partWriter.Write(jsonBytes)
			if err != nil {
				return nil, "", err
			}
		case "osFile":
			v := part.Value.(os.File)
			fileInfo, err := v.Stat()
			if err != nil {
				return nil, "", err
			}
			h.Set(constants.ContentDisposition, fmt.Sprintf(`form-data; name="%s"; filename="%s"`, part.Name, fileInfo.Name()))
			partWriter, err := writer.CreateFormFile(part.Name, fileInfo.Name())
			if err != nil {
				return nil, "", err
			}
			_, err = io.Copy(partWriter, &v)
			if err != nil {
				return nil, "", err
			}
			err = v.Close()
			if err != nil {
				return nil, "", err
			} // Manually close the file here
		case "file":
			v := part.Value.(multipart.FileHeader)
			h.Set(constants.ContentDisposition, fmt.Sprintf(`form-data; name="%s"; filename="%s"`, part.Name, v.Filename))
			partWriter, err := writer.CreateFormFile(part.Name, v.Filename)
			if err != nil {
				return nil, "", err
			}
			file, err := v.Open()
			if err != nil {
				return nil, "", err
			}
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

	err := writer.Close()
	if err != nil {
		return nil, "", err
	}

	return buf, writer.FormDataContentType(), nil
}
