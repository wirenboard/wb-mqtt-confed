package confed

import (
	"github.com/xeipuuv/gojsonschema"
)

type JSONSchema struct {
	path    string
	schema  *gojsonschema.Schema
	content []byte
}

func configLoader(path string) (loader gojsonschema.JSONLoader, content []byte, err error) {
	content, err = loadConfigBytes(path)
	if err != nil {
		return
	}
	loader = gojsonschema.NewStringLoader(string(content))
	return
}

func NewJSONSchema(schemaPath string) (s *JSONSchema, err error) {
	loader, content, err := configLoader(schemaPath)
	if err != nil {
		return
	}
	schema, err := gojsonschema.NewSchema(loader)
	if err != nil {
		return
	}
	return &JSONSchema{
		path:    schemaPath,
		schema:  schema,
		content: content,
	}, nil
}

func (s *JSONSchema) ValidateContent(content []byte) (*gojsonschema.Result, error) {
	documentLoader := gojsonschema.NewStringLoader(string(content))
	return s.schema.Validate(documentLoader)
}

func (s *JSONSchema) ValidateFile(path string) (result *gojsonschema.Result, err error) {
	bs, err := loadConfigBytes(path)
	if err != nil {
		return
	}
	return s.ValidateContent(bs)
}

func (s *JSONSchema) Path() string {
	return s.path
}

func (s *JSONSchema) Content() []byte {
	return s.content
}
