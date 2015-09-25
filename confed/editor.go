package confed

import (
	"bytes"
	"encoding/json"
	"github.com/contactless/wbgo"
	"io/ioutil"
	"path/filepath"
	"sort"
	"sync"
)

const (
	RESTART_QUEUE_LEN = 100
)

func fixFormatProps(v interface{}) interface{} {
	switch v.(type) {
	case map[string]interface{}:
		m := v.(map[string]interface{})
		r := make(map[string]interface{})
		for k, item := range m {
			if k == "_format" {
				r["format"] = fixFormatProps(item)
			} else {
				r[k] = fixFormatProps(item)
			}
		}
		return r
	case []interface{}:
		l := v.([]interface{})
		r := make([]interface{}, len(l))
		for n, item := range l {
			r[n] = fixFormatProps(item)
		}
		return r
	default:
		return v
	}
}

type RestartRequest struct {
	Name    string
	DelayMS int
}

type Editor struct {
	mtx                 sync.Mutex
	root                string
	schemasByConfigPath map[string]*JSONSchema
	schemasBySchemaPath map[string]*JSONSchema
	RestartCh           chan RestartRequest
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

func NewEditor(root string) *Editor {
	confRoot, err := filepath.Abs(root)
	if err != nil {
		wbgo.Error.Printf("invalid root path %s, using /", root)
		confRoot = root
	}
	return &Editor{
		root:                confRoot,
		schemasByConfigPath: make(map[string]*JSONSchema),
		schemasBySchemaPath: make(map[string]*JSONSchema),
		RestartCh:           make(chan RestartRequest, RESTART_QUEUE_LEN),
	}
}

func (editor *Editor) loadSchema(path string) (err error) {
	editor.mtx.Lock()
	defer editor.mtx.Unlock()

	wbgo.Debug.Printf("Loading schema file: %s", path)
	schema, err := NewJSONSchemaWithRoot(path, editor.root)
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

func (editor *Editor) Load(args *EditorPathArgs, reply *EditorContentResponse) error {
	schema, err := editor.locateConfig(args.Path)
	if err != nil {
		return err
	}

	bs, err := loadConfigBytes(schema.PhysicalConfigPath(), schema.ToJSONCommand())
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
	reply.Schema = fixFormatProps(schema.GetPreprocessed()).(map[string]interface{})

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

	var bs []byte
	if schema.FromJSONCommand() != nil {
		var buf *bytes.Buffer
		buf, err = extPreprocess(schema.FromJSONCommand(), *args.Content)
		if err != nil {
			wbgo.Error.Printf("external command error, %s: %s", schema.PhysicalConfigPath(), err)
			return writeError
		}
		bs = buf.Bytes()
	} else {
		var indented bytes.Buffer
		if err = json.Indent(&indented, *args.Content, "", "    "); err != nil {
			wbgo.Error.Printf("json.Indent() error, %s: %s", schema.PhysicalConfigPath(), err)
			return writeError
		}
		bs = indented.Bytes()
	}

	if err = ioutil.WriteFile(schema.PhysicalConfigPath(), bs, 0777); err != nil {
		wbgo.Error.Printf("error writing %s: %s", schema.PhysicalConfigPath(), err)
		return writeError
	}

	reply.Path = args.Path
	if schema.Service() != "" {
		editor.RestartCh <- RestartRequest{schema.Service(), schema.RestartDelayMS()}
	}
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
