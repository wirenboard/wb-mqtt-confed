package confed

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/wirenboard/wbgong"
	"github.com/xeipuuv/gojsonschema"
)

const (
	defaultSubconfPattern = `^.*\.conf$`
)

// JSONSchemaProps holds schema properties, some of which are reported by the List RPC method.
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

// JSONSchema is a config schema with its patches and enum subconfs.
type JSONSchema struct {
	path         string
	schema       *gojsonschema.Schema
	content      []byte
	parsed       map[string]any
	preprocessed map[string]any
	props        JSONSchemaProps
	enumLoader   *enumLoader
	patchLoader  *patchLoader
}

func subconfKey(path, pattern, ptrString string) string {
	return path + "\x00" + pattern + "\x00" + ptrString
}

func extractStringOrStringList(msi map[string]any, key string) ([]string, error) {
	cmd, found := msi[key]
	if !found {
		return nil, nil
	}

	s, ok := cmd.(string)
	if ok {
		return []string{s}, nil
	}

	parts, ok := cmd.([]any)
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

func addTranslation(strings map[string]any, lang, key string, dst map[string]string) {
	translated, ok := strings[key]
	if ok {
		res, ok := translated.(string)
		if ok {
			dst[lang] = res
		}
	}
}

// NewJSONSchemaWithRoot loads a schema with config paths relative to the root directory.
func NewJSONSchemaWithRoot(schemaPath, root string) (*JSONSchema, error) {
	bs, err := loadConfigBytes(schemaPath, nil)
	if err != nil {
		return nil, err
	}
	content := bs.content
	physicalSchemaPath, err := filepath.Abs(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path of %s: %w", schemaPath, err)
	}

	var parsed map[string]any
	if err = json.Unmarshal(content, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", schemaPath, err)
	}

	configFile, _ := parsed["configFile"].(map[string]any)
	if configFile == nil {
		return nil, errors.New("no configFile section in the schema")
	}

	physicalConfigPath, _ := configFile["path"].(string)
	if physicalConfigPath == "" {
		return nil, errors.New("bad config path or no config path in schema file")
	}
	physicalConfigPath, configPath, err := fakeRootPath(root, physicalConfigPath)
	if err != nil {
		return nil, err
	}

	fromJSONCommand, err := extractStringOrStringList(configFile, "fromJSON")
	if err != nil {
		return nil, err
	}

	toJSONCommand, err := extractStringOrStringList(configFile, "toJSON")
	if err != nil {
		return nil, err
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
		return nil, err
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
	translations, ok := parsed["translations"].(map[string]any)
	if ok {
		for lang, val := range translations {
			strings, ok := val.(map[string]any)
			if ok {
				addTranslation(strings, lang, title, titleTranslations)
				addTranslation(strings, lang, description, descriptionTranslations)
			}
		}
	}

	return &JSONSchema{
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
		patchLoader: newPatchLoader(physicalSchemaPath),
	}, nil
}

// GetPreprocessed returns the schema with patches applied and enum subconfs resolved.
func (s *JSONSchema) GetPreprocessed() map[string]any {
	if s.patchLoader.IsDirty() {
		err := json.Unmarshal(s.patchLoader.Patch(s.content), &s.parsed)
		if err != nil {
			wbgong.Warn.Printf("Failed to parse patched schema %s: %s", s.path, err)
		} else {
			s.preprocessed = nil
		}
	}
	if s.preprocessed == nil || s.enumLoader.IsDirty() {
		s.preprocessed = s.enumLoader.Preprocess(s.parsed).(map[string]any) // preprocessing of a map always yields a map
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

// ValidateContent validates the config content against the schema.
func (s *JSONSchema) ValidateContent(content []byte) (r *gojsonschema.Result, err error) {
	documentLoader := gojsonschema.NewStringLoader(string(content))
	schema, err := s.getSchema()
	if err != nil {
		return
	}
	r, err = schema.Validate(documentLoader)
	if err != nil {
		return nil, fmt.Errorf("failed to load document: %w", err)
	}
	return r, nil
}

// ValidateFile validates the config file against the schema.
func (s *JSONSchema) ValidateFile(path string) (result *gojsonschema.Result, err error) {
	bs, err := loadConfigBytes(path, nil)
	if err != nil {
		return
	}
	return s.ValidateContent(bs.content)
}

// Path returns the schema path relative to the root directory.
func (s *JSONSchema) Path() string {
	return s.path
}

// Content returns the original schema content.
func (s *JSONSchema) Content() []byte {
	return s.content
}

// ConfigPath returns the config path relative to the root directory.
func (s *JSONSchema) ConfigPath() string {
	return s.props.ConfigPath
}

// PhysicalConfigPath returns the config path in the filesystem.
func (s *JSONSchema) PhysicalConfigPath() string {
	return s.props.physicalConfigPath
}

// ToJSONCommand returns the command converting the config to JSON, if any.
func (s *JSONSchema) ToJSONCommand() []string {
	return s.props.toJSONCommand
}

// FromJSONCommand returns the command converting JSON to the config format, if any.
func (s *JSONSchema) FromJSONCommand() []string {
	return s.props.fromJSONCommand
}

// Title returns the schema title.
func (s *JSONSchema) Title() string {
	return s.props.Title
}

// Description returns the schema description.
func (s *JSONSchema) Description() string {
	return s.props.Description
}

// Services returns the services to restart after the config is saved.
func (s *JSONSchema) Services() []string {
	return s.props.services
}

// RestartDelayMS returns the delay before restarting the services.
func (s *JSONSchema) RestartDelayMS() int {
	return s.props.restartDelayMS
}

// ShouldValidate reports whether the config should be validated against the schema.
func (s *JSONSchema) ShouldValidate() bool {
	return s.props.shouldValidate
}

// HideFromList reports whether the schema should be hidden from the List RPC method.
func (s *JSONSchema) HideFromList() bool {
	return s.props.hideFromList
}

// Properties returns the schema properties.
func (s *JSONSchema) Properties() *JSONSchemaProps {
	return &s.props
}

// StopWatchingDependentFiles stops watching enum subconfs and schema patches.
func (s *JSONSchema) StopWatchingDependentFiles() {
	s.enumLoader.StopWatchingSubconfigs()
	s.patchLoader.StopWatchingPatches()
}

// Editor returns the name of a custom editor for the config.
func (s *JSONSchema) Editor() string {
	return s.props.Editor
}
