package confed

import (
	"encoding/json"
	"errors"
	"github.com/xeipuuv/gojsonschema"
)

const (
	DEFAULT_SUBCONF_PATTERN = `^.*\.conf$`
)

type JSONSchemaProps struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	ConfigPath         string `json:"configPath"`
	SchemaPath         string `json:"schemaPath"`
	physicalConfigPath string
	fromJSONCommand    []string
	toJSONCommand      []string
	service            string
	restartDelayMS     int
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

func subconfKey(path, pattern, ptrString string) string {
	return path + "\x00" + pattern + "\x00" + ptrString
}

func extractCommand(msi map[string]interface{}, key string) ([]string, error) {
	cmd, found := msi[key]
	if !found {
		return nil, nil
	}

	s, ok := cmd.(string)
	if ok {
		return []string{s}, nil
	}

	parts, ok := cmd.([]interface{})
	if !ok {
		return nil, errors.New("bad command spec")
	}

	r := make([]string, len(parts))
	for n, p := range parts {
		r[n], ok = p.(string)
		if !ok {
			return nil, errors.New("bad command spec")
		}
	}
	return r, nil
}

func NewJSONSchemaWithRoot(schemaPath, root string) (s *JSONSchema, err error) {
	content, err := loadConfigBytes(schemaPath, nil)
	if err != nil {
		return
	}

	var parsed map[string]interface{}
	if err = json.Unmarshal(content, &parsed); err != nil {
		return
	}

	configFile, _ := parsed["configFile"].(map[string]interface{})
	if configFile == nil {
		return nil, errors.New("no configFile section in the schema")
	}

	physicalConfigPath, _ := configFile["path"].(string)
	if physicalConfigPath == "" {
		return nil, errors.New("bad config path or no config path in schema file")
	}
	physicalConfigPath, configPath, err := fakeRootPath(root, physicalConfigPath)
	if err != nil {
		return
	}

	fromJSONCommand, err := extractCommand(configFile, "fromJSON")
	if err != nil {
		return
	}

	toJSONCommand, err := extractCommand(configFile, "toJSON")
	if err != nil {
		return
	}

	service, _ := configFile["service"].(string)
	restartDelayMS, _ := configFile["restartDelayMS"].(float64)

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
			SchemaPath:         schemaPathFromRoot,
			physicalConfigPath: physicalConfigPath,
			Title:              title,
			Description:        description,
			fromJSONCommand:    fromJSONCommand,
			toJSONCommand:      toJSONCommand,
			service:            service,
			restartDelayMS:     int(restartDelayMS),
		},
		enumLoader: newEnumLoader(root),
	}
	return
}

func NewJSONSchema(schemaPath string) (s *JSONSchema, err error) {
	return NewJSONSchemaWithRoot(schemaPath, "/")
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
	bs, err := loadConfigBytes(path, nil)
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

func (s *JSONSchema) ToJSONCommand() []string {
	return s.props.toJSONCommand
}

func (s *JSONSchema) FromJSONCommand() []string {
	return s.props.fromJSONCommand
}

func (s *JSONSchema) Title() string {
	return s.props.Title
}

func (s *JSONSchema) Description() string {
	return s.props.Description
}

func (s *JSONSchema) Service() string {
	return s.props.service
}

func (s *JSONSchema) RestartDelayMS() int {
	return s.props.restartDelayMS
}

func (s *JSONSchema) Properties() *JSONSchemaProps {
	return &s.props
}

func (s *JSONSchema) StopWatchingSubconfigs() {
	s.enumLoader.StopWatchingSubconfigs()
}
