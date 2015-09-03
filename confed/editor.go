package confed

import (
	"encoding/json"
	"github.com/contactless/wbgo"
	"io/ioutil"
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
	EDITOR_ERROR_INVALID_PATH   = 1000
	EDITOR_ERROR_LISTDIR        = 1001
	EDITOR_ERROR_WRITE          = 1002
	EDITOR_ERROR_FILE_NOT_FOUND = 1003
	EDITOR_ERROR_REMOVE         = 1004
	EDITOR_ERROR_READ           = 1005
	EDITOR_ERROR_INVALID_CONFIG = 1006
	EDITOR_ERROR_INVALID_SCHEMA = 1007
)

var invalidPathError = &EditorError{EDITOR_ERROR_INVALID_PATH, "Invalid path"}
var listDirError = &EditorError{EDITOR_ERROR_LISTDIR, "Error listing the directory"}
var writeError = &EditorError{EDITOR_ERROR_WRITE, "Error writing the file"}
var fileNotFoundError = &EditorError{EDITOR_ERROR_FILE_NOT_FOUND, "File not found"}
var invalidConfigError = &EditorError{EDITOR_ERROR_INVALID_CONFIG, "Invalid config file"}
var invalidConfigSchemaError = &EditorError{EDITOR_ERROR_INVALID_SCHEMA, "Invalid config schema"}
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

	wbgo.Debug.Printf("Loading schema file: %s", path)
	schema, err := newJSONSchema(path, editor.root)
	if err != nil {
		wbgo.Error.Printf("Error loading schema: %s", err)
		return
	}

	editor.doRemoveSchema(schema.Path())
	editor.schemasBySchemaPath[schema.Path()] = schema
	editor.schemasByConfigPath[schema.ConfigPath()] = schema
	return
}

func (editor *Editor) doRemoveSchema(path string) {
	schema, found := editor.schemasBySchemaPath[path]
	if !found {
		return
	}

	schema.StopWatchingSubconfigs()
	delete(editor.schemasBySchemaPath, schema.Path())
	delete(editor.schemasByConfigPath, schema.ConfigPath())
}

func (editor *Editor) removeSchema(path string) (err error) {
	editor.mtx.Lock()
	defer editor.mtx.Unlock()

	path, err = pathFromRoot(editor.root, path)
	if err != nil {
		return
	}

	editor.doRemoveSchema(path)
	return nil
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

type EditorPathResponse struct {
	Path string `json:"path"`
}

type EditorContentResponse struct {
	Content *json.RawMessage       `json:"content"`
	Schema  map[string]interface{} `json:"schema"`
}

func (editor *Editor) locateConfig(path string) (*JSONSchema, error) {
	if conf, ok := editor.schemasByConfigPath[path]; !ok {
		return nil, fileNotFoundError
	} else {
		return conf, nil
	}
}

func (editor *Editor) configPath(schema *JSONSchema) string {
	return filepath.Join(editor.root, schema.ConfigPath())
}

func (editor *Editor) Load(args *EditorPathArgs, reply *EditorContentResponse) error {
	schema, err := editor.locateConfig(args.Path)
	if err != nil {
		return err
	}

	bs, err := loadConfigBytes(editor.configPath(schema))
	if err != nil {
		wbgo.Error.Printf("Failed to read config file %s: %s", args.Path, err)
		return invalidConfigError
	}

	r, err := schema.ValidateContent(bs)
	if err != nil {
		wbgo.Error.Printf("Failed to validate config file %s: %s", args.Path, err)
		return invalidConfigError
	}
	if !r.Valid() {
		wbgo.Error.Printf("Invalid config file %s", args.Path)
		for _, desc := range r.Errors() {
			wbgo.Error.Printf("- %s\n", desc)
		}
		return invalidConfigError
	}

	content := json.RawMessage(bs) // TBD: use parsed config
	reply.Content = &content
	reply.Schema = schema.GetPreprocessed()

	return nil
}

type EditorSaveArgs struct {
	Path    string           `json:"path"`
	Content *json.RawMessage `json:"content"`
}

func (editor *Editor) Save(args *EditorSaveArgs, reply *EditorPathResponse) error {
	editor.mtx.Lock()
	defer editor.mtx.Unlock()

	schema, err := editor.locateConfig(args.Path)
	if err != nil {
		return err
	}
	r, err := schema.ValidateContent(*args.Content)
	if err != nil || !r.Valid() {
		return invalidConfigError
	}

	if err = ioutil.WriteFile(editor.configPath(schema), *args.Content, 0777); err != nil {
		wbgo.Error.Printf("error writing %s: %s", editor.configPath(schema), err)
		return writeError
	}

	reply.Path = args.Path
	return nil
}

func (editor *Editor) stopWatchingSubconfigs() {
	for _, schema := range editor.schemasBySchemaPath {
		schema.StopWatchingSubconfigs()
	}
}

// We don't provide LoadFile / LiveLoadFile / LiveRemoveFile
// for *Editor itself in order to avoid RPC server warnings
// about improper methods.

type EditorDirWatcherClient struct {
	editor *Editor
}

func NewEditorDirWatcherClient(editor *Editor) wbgo.DirWatcherClient {
	return &EditorDirWatcherClient{editor}
}

func (c *EditorDirWatcherClient) LoadFile(path string) error {
	return c.editor.loadSchema(path)
}

func (c *EditorDirWatcherClient) LiveLoadFile(path string) error {
	return c.LoadFile(path)
}

func (c *EditorDirWatcherClient) LiveRemoveFile(path string) error {
	return c.editor.removeSchema(path)
}
