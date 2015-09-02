package confed

import (
	"encoding/json"
	"errors"
	"github.com/xeipuuv/gojsonschema"
	"path/filepath"
)

type JSONSchemaProps struct {
	ConfigPath  string `json:"configPath"`
	Description string `json:"description"`
	Title       string `json:"title"`
}

type JSONSchema struct {
	path    string
	schema  *gojsonschema.Schema
	content []byte
	props   JSONSchemaProps
}

func configLoader(path string) (loader gojsonschema.JSONLoader, content []byte, err error) {
	content, err = loadConfigBytes(path)
	if err != nil {
		return
	}
	loader = gojsonschema.NewStringLoader(string(content))
	return
}

func pathFromRoot(root, path string) (r string, err error) {
	path, err = filepath.Abs(path)
	if err == nil {
		r, err = filepath.Rel(root, path)
		if err == nil {
			r = "/" + r
		}
	}
	return
}

func NewJSONSchemaWithRoot(schemaPath, root string) (s *JSONSchema, err error) {
	loader, content, err := configLoader(schemaPath)
	if err != nil {
		return
	}
	schema, err := gojsonschema.NewSchema(loader)
	if err != nil {
		return
	}
	schemaPathFromRoot, err := pathFromRoot(root, schemaPath)
	if err != nil {
		return
	}
	s = &JSONSchema{
		path:    schemaPathFromRoot,
		schema:  schema,
		content: content,
	}
	err = json.Unmarshal(content, &s.props)
	if err == nil && s.props.ConfigPath == "" {
		return nil, errors.New("no config path in schema file")
	}
	s.props.ConfigPath, err = pathFromRoot(root, s.props.ConfigPath)
	return
}

func NewJSONSchema(schemaPath string) (s *JSONSchema, err error) {
	return NewJSONSchemaWithRoot(schemaPath, "/")
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

func (s *JSONSchema) ConfigPath() string {
	return s.props.ConfigPath
}

func (s *JSONSchema) Title() string {
	return s.props.Title
}

func (s *JSONSchema) Description() string {
	return s.props.Description
}

func (s *JSONSchema) Properties() *JSONSchemaProps {
	return &s.props
}

// TBD: rename to ConfigSchema
