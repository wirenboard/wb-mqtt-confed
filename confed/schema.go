package confed

import (
	"encoding/json"
	"errors"
	"github.com/xeipuuv/gojsonschema"
	"path/filepath"
)

const (
	DEFAULT_SUBCONF_PATTERN = `^.*\.conf$`
)

type JSONSchemaProps struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	ConfigPath         string `json:"configPath"`
	physicalConfigPath string
}

type JSONSchema struct {
	path         string
	schema       *gojsonschema.Schema
	content      []byte
	parsed       map[string]interface{}
	preprocessed map[string]interface{}
	props        JSONSchemaProps
	enumLoader   *enumLoader
}

func pathFromRoot(root, path string) (r string, err error) {
	if len(root) == 0 || root[:len(root)-1] != "/" {
		root = root + "/"
	}
	path, err = filepath.Abs(path)
	if err == nil {
		r, err = filepath.Rel(root, path)
		if err == nil {
			r = "/" + r
		}
	}
	return
}

func subconfKey(path, pattern, ptrString string) string {
	return path + "\x00" + pattern + "\x00" + ptrString
}

func newJSONSchema(schemaPath, root string) (s *JSONSchema, err error) {
	content, err := loadConfigBytes(schemaPath)
	if err != nil {
		return
	}

	var parsed map[string]interface{}
	if err = json.Unmarshal(content, &parsed); err != nil {
		return
	}

	physicalConfigPath, _ := parsed["configPath"].(string)
	if physicalConfigPath == "" {
		return nil, errors.New("bad configPath or no configPath in schema file")
	}
	if physicalConfigPath[:1] != "/" {
		physicalConfigPath = "/" + physicalConfigPath
	} else {
		for physicalConfigPath[:1] == "/" {
			physicalConfigPath = physicalConfigPath[1:]
		}
	}
	physicalConfigPath = filepath.Join(root, physicalConfigPath)
	configPath, err := pathFromRoot(root, physicalConfigPath)
	if err != nil {
		return
	}

	title, _ := parsed["title"].(string)
	description, _ := parsed["description"].(string)

	schemaPathFromRoot, err := pathFromRoot(root, schemaPath)
	if err != nil {
		return
	}
	s = &JSONSchema{
		path:    schemaPathFromRoot,
		schema:  nil,
		content: content,
		parsed:  parsed,
		props: JSONSchemaProps{
			ConfigPath:         configPath,
			physicalConfigPath: physicalConfigPath,
			Title:              title,
			Description:        description,
		},
		enumLoader: newEnumLoader(),
	}
	return
}

func NewJSONSchema(schemaPath string) (s *JSONSchema, err error) {
	return newJSONSchema(schemaPath, "/")
}

func (s *JSONSchema) GetPreprocessed() map[string]interface{} {
	if s.preprocessed == nil || s.enumLoader.IsDirty() {
		s.preprocessed = s.enumLoader.Preprocess(s.parsed).(map[string]interface{}) // FIXME
	}
	return s.preprocessed
}

func (s *JSONSchema) getSchema() (schema *gojsonschema.Schema, err error) {
	if s.schema != nil && !s.enumLoader.IsDirty() {
		return s.schema, nil
	}

	loader := gojsonschema.NewGoLoader(s.GetPreprocessed())
	s.schema, err = gojsonschema.NewSchema(loader)
	if err != nil {
		return
	}

	return s.schema, nil
}

func (s *JSONSchema) ValidateContent(content []byte) (r *gojsonschema.Result, err error) {
	documentLoader := gojsonschema.NewStringLoader(string(content))
	schema, err := s.getSchema()
	if err != nil {
		return
	}
	return schema.Validate(documentLoader)
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

func (s *JSONSchema) PhysicalConfigPath() string {
	return s.props.physicalConfigPath
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

func (s *JSONSchema) StopWatchingSubconfigs() {
	s.enumLoader.StopWatchingSubconfigs()
}
