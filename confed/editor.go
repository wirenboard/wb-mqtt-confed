package confed

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"sync"
)

type Editor struct {
	mtx                 sync.Mutex
	root                string
	schemasByConfigPath map[string]*JSONSchema
	schemasBySchemaPath map[string]*JSONSchema
}

type EditorError struct {
	code    int32
	message string
}

func (err *EditorError) Error() string {
	return err.message
}

func (err *EditorError) ErrorCode() int32 {
	return err.code
}

const (
	// no iota here because these values may be used
	// by external software
	EDITOR_ERROR_INVALID_PATH         = 1000
	EDITOR_ERROR_LISTDIR              = 1001
	EDITOR_ERROR_WRITE                = 1002
	EDITOR_ERROR_FILE_NOT_FOUND       = 1003
	EDITOR_ERROR_REMOVE               = 1004
	EDITOR_ERROR_READ                 = 1005
	EDITOR_ERROR_INVALID_CONFIG_ERROR = 1006
	EDITOR_ERROR_INVALID_SCHEMA_ERROR = 1007
)

var invalidPathError = &EditorError{EDITOR_ERROR_INVALID_PATH, "Invalid path"}
var listDirError = &EditorError{EDITOR_ERROR_LISTDIR, "Error listing the directory"}
var writeError = &EditorError{EDITOR_ERROR_WRITE, "Error writing the file"}
var fileNotFoundError = &EditorError{EDITOR_ERROR_FILE_NOT_FOUND, "File not found"}
var invalidConfigError = &EditorError{EDITOR_ERROR_INVALID_CONFIG_ERROR, "Invalid config file"}
var invalidConfigSchemaError = &EditorError{EDITOR_ERROR_INVALID_SCHEMA_ERROR, "Invalid config schema"}
var rmError = &EditorError{EDITOR_ERROR_REMOVE, "Error removing the file"}
var readError = &EditorError{EDITOR_ERROR_READ, "Error reading the file"}

func NewEditor() *Editor {
	return &Editor{
		root:                "/",
		schemasByConfigPath: make(map[string]*JSONSchema),
		schemasBySchemaPath: make(map[string]*JSONSchema),
	}
}

func (editor *Editor) setRoot(path string) {
	if len(editor.schemasBySchemaPath) > 0 {
		panic("cannot set root for non-empty editor")
	}
	editor.root = path
}

func (editor *Editor) loadSchema(path string) (err error) {
	editor.mtx.Lock()
	defer editor.mtx.Unlock()

	schema, err := NewJSONSchemaWithRoot(path, editor.root)
	if err != nil {
		return
	}

	if err != nil {
		return
	}
	oldSchema, found := editor.schemasBySchemaPath[schema.Path()]
	if found {
		delete(editor.schemasBySchemaPath, oldSchema.Path())
		delete(editor.schemasByConfigPath, oldSchema.ConfigPath())
	}
	editor.schemasBySchemaPath[schema.Path()] = schema
	editor.schemasByConfigPath[schema.ConfigPath()] = schema

	return
}

func (editor *Editor) List(args *struct{}, reply *[]*JSONSchemaProps) (err error) {
	editor.mtx.Lock()
	defer editor.mtx.Unlock()

	paths := make([]string, 0, len(editor.schemasByConfigPath))
	for path := range editor.schemasByConfigPath {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	*reply = make([]*JSONSchemaProps, len(editor.schemasByConfigPath))
	for i, path := range paths {
		(*reply)[i] = editor.schemasByConfigPath[path].Properties()
	}
	return
}

type EditorPathArgs struct {
	Path string `json:"path"`
}

type EditorContentResponse struct {
	Content *json.RawMessage `json:"content"`
	Schema  *json.RawMessage `json:"schema"`
}

func (editor *Editor) locateConfig(path string) (*JSONSchema, error) {
	if conf, ok := editor.schemasByConfigPath[path]; !ok {
		return nil, fileNotFoundError
	} else {
		return conf, nil
	}
}

func (editor *Editor) Load(args *EditorPathArgs, reply *EditorContentResponse) error {
	schema, err := editor.locateConfig(args.Path)
	if err != nil {
		return err
	}

	bs, err := loadConfigBytes(filepath.Join(editor.root, schema.ConfigPath()))
	if err != nil {
		return invalidConfigError
	}

	r, err := schema.ValidateContent(bs)
	if err != nil || !r.Valid() {
		return invalidConfigError
	}

	content := json.RawMessage(bs)
	schemaContent := json.RawMessage(schema.Content())
	reply.Content = &content
	reply.Schema = &schemaContent

	return nil
}
