package confed

import (
	"encoding/json"
	"errors"

	"github.com/wirenboard/wbgong"
	"github.com/xeipuuv/gojsonschema"
)

const (
	DEFAULT_SUBCONF_PATTERN = `^.*\.conf$`
)

type JSONSchemaProps struct {
	Title                   string `json:"title"`
	Description             string `json:"description"`
	ConfigPath              string `json:"configPath"`
	SchemaPath              string `json:"schemaPath"`
	physicalConfigPath      string
	fromJSONCommand         []string
	toJSONCommand           []string
	services                []string
	restartDelayMS          int
	shouldValidate          bool
	hideFromList            bool
	TitleTranslations       map[string]string `json:"titleTranslations,omitempty"`
	DescriptionTranslations map[string]string `json:"descriptionTranslations,omitempty"`
	Editor                  string            `json:"editor"`
}

type JSONSchema struct {
	path         string
	schema       *gojsonschema.Schema
	content      []byte
	parsed       map[string]interface{}
	preprocessed map[string]interface{}
	props        JSONSchemaProps
	enumLoader   *enumLoader
	patchLoader  *patchLoader
}

func subconfKey(path, pattern, ptrString string) string {
	return path + "\x00" + pattern + "\x00" + ptrString
}

func extractStringOrStringList(msi map[string]interface{}, key string) ([]string, error) {
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

func addTranslation(strings map[string]interface{}, lang string, key string, dst map[string]string) {
	translated, ok := strings[key]
	if ok {
		res, ok := translated.(string)
		if ok {
			dst[lang] = res
		}
	}
}

func NewJSONSchemaWithRoot(schemaPath, root string) (s *JSONSchema, err error) {
	bs, err := loadConfigBytes(schemaPath, nil)
	content := bs.content
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

	fromJSONCommand, err := extractStringOrStringList(configFile, "fromJSON")
	if err != nil {
		return
	}

	toJSONCommand, err := extractStringOrStringList(configFile, "toJSON")
	if err != nil {
		return
	}

	shouldValidate, ok := configFile["validate"].(bool)
	if !ok {
		shouldValidate = true
	}

	hideFromList, ok := configFile["hide"].(bool)
	if !ok {
		hideFromList = false
	}

	services, _ := extractStringOrStringList(configFile, "service")
	restartDelayMS, _ := configFile["restartDelayMS"].(float64)
	editor, _ := configFile["editor"].(string)

	title, _ := parsed["title"].(string)
	description, _ := parsed["description"].(string)

	schemaPathFromRoot, err := pathFromRoot(root, schemaPath)
	if err != nil {
		return
	}

	// A schema could contain "translations" property
	// Expected structure of the property:
	// "translations": {
	//     "lang": {
	//         "english_string": "translated_string",
	//         ...
	//     }
	// }
	titleTranslations := map[string]string{}
	descriptionTranslations := map[string]string{}
	translations, ok := parsed["translations"].(map[string]interface{})
	if ok {
		for lang, val := range translations {
			strings, ok := val.(map[string]interface{})
			if ok {
				addTranslation(strings, lang, title, titleTranslations)
				addTranslation(strings, lang, description, descriptionTranslations)
			}
		}
	}

	s = &JSONSchema{
		path:    schemaPathFromRoot,
		schema:  nil,
		content: content,
		parsed:  parsed,
		props: JSONSchemaProps{
			ConfigPath:              configPath,
			SchemaPath:              schemaPathFromRoot,
			physicalConfigPath:      physicalConfigPath,
			Title:                   title,
			Description:             description,
			fromJSONCommand:         fromJSONCommand,
			toJSONCommand:           toJSONCommand,
			services:                services,
			restartDelayMS:          int(restartDelayMS),
			shouldValidate:          shouldValidate,
			hideFromList:            hideFromList,
			TitleTranslations:       titleTranslations,
			DescriptionTranslations: descriptionTranslations,
			Editor:                  editor,
		},
		enumLoader:  newEnumLoader(root),
		patchLoader: newPatchLoader(schemaPathFromRoot),
	}
	return
}

func NewJSONSchema(schemaPath string) (s *JSONSchema, err error) {
	return NewJSONSchemaWithRoot(schemaPath, "/")
}

func (s *JSONSchema) GetPreprocessed() map[string]interface{} {
	if s.patchLoader.IsDirty() {
		err := json.Unmarshal(s.patchLoader.Patch(s.content), &s.parsed)
		if err != nil {
			wbgong.Warn.Printf("Failed to parse patched schema %s: %s", s.path, err)
		} else {
			s.preprocessed = nil
		}
	}
	if s.preprocessed == nil || s.enumLoader.IsDirty() {
		s.preprocessed = s.enumLoader.Preprocess(s.parsed).(map[string]interface{}) // FIXME
	}
	return s.preprocessed
}

func (s *JSONSchema) getSchema() (schema *gojsonschema.Schema, err error) {
	if s.schema != nil && !s.enumLoader.IsDirty() && !s.patchLoader.IsDirty() {
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
	return s.ValidateContent(bs.content)
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

func (s *JSONSchema) Services() []string {
	return s.props.services
}

func (s *JSONSchema) RestartDelayMS() int {
	return s.props.restartDelayMS
}

func (s *JSONSchema) ShouldValidate() bool {
	return s.props.shouldValidate
}

func (s *JSONSchema) HideFromList() bool {
	return s.props.hideFromList
}

func (s *JSONSchema) Properties() *JSONSchemaProps {
	return &s.props
}

func (s *JSONSchema) StopWatchingDependentFiles() {
	s.enumLoader.StopWatchingSubconfigs()
	s.patchLoader.StopWatchingPatches()
}

func (s *JSONSchema) Editor() string {
	return s.props.Editor
}
