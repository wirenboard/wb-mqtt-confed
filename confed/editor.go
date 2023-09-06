package confed

import (
	"bytes"
	"encoding/json"
	"github.com/wirenboard/wbgong"
	"io/ioutil"
	"path/filepath"
	"sort"
	"sync"
	"strconv"
	"strings"
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

func printPreprocessorErrors(configPath string, errors string) {
	if len(errors) > 0 {
		for _, err := range strings.Split(strings.TrimRight(errors, "\n"), "\n") {
			wbgong.Warn.Printf("config preprocessor of %s printed in stderr: %s", configPath, err)
		}
	}
}

type RequestType int64
const (
	Sleep RequestType = iota
	Sync
	Restart
)

type Request struct {
	requestType RequestType
	properties map[string]string
}

type Editor struct {
	mtx                 sync.Mutex
	root                string
	schemasByConfigPath map[string][]*JSONSchema
	schemasBySchemaPath map[string]*JSONSchema
	RequestCh           chan Request
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
		wbgong.Error.Printf("invalid root path %s, using /", root)
		confRoot = root
	}
	return &Editor{
		root:                confRoot,
		schemasByConfigPath: make(map[string][]*JSONSchema),
		schemasBySchemaPath: make(map[string]*JSONSchema),
		RequestCh:           make(chan Request, RESTART_QUEUE_LEN),
	}
}

func (editor *Editor) loadSchema(path string) (err error) {
	editor.mtx.Lock()
	defer editor.mtx.Unlock()

	wbgong.Debug.Printf("Loading schema file: %s", path)
	schema, err := NewJSONSchemaWithRoot(path, editor.root)
	if err != nil {
		wbgong.Error.Printf("Error loading schema: %s", err)
		return
	}

	editor.doRemoveSchema(schema.Path())
	editor.schemasBySchemaPath[schema.Path()] = schema
	if l, ok := editor.schemasByConfigPath[schema.ConfigPath()]; ok {
		editor.schemasByConfigPath[schema.ConfigPath()] = append(l, schema)
	} else {
		editor.schemasByConfigPath[schema.ConfigPath()] = []*JSONSchema{schema}
	}
	return
}

func (editor *Editor) doRemoveSchema(path string) {
	schema, found := editor.schemasBySchemaPath[path]
	if !found {
		return
	}

	schema.StopWatchingSubconfigs()
	delete(editor.schemasBySchemaPath, schema.Path())
	l := editor.schemasByConfigPath[schema.ConfigPath()]
	if l == nil {
		panic("schema not registered by config path")
	}
	newList := make([]*JSONSchema, 0, len(l)-1)
	for _, s := range l {
		if s == schema {
			continue
		}
		newList = append(newList, s)
	}
	editor.schemasByConfigPath[schema.ConfigPath()] = newList
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

type ByConfigThenSchemaPath []*JSONSchemaProps

func (b ByConfigThenSchemaPath) Len() int      { return len(b) }
func (b ByConfigThenSchemaPath) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b ByConfigThenSchemaPath) Less(i, j int) bool {
	if b[i].ConfigPath == b[j].ConfigPath {
		return b[i].SchemaPath < b[j].SchemaPath
	}
	return b[i].ConfigPath < b[j].ConfigPath
}

func (editor *Editor) List(args *struct{}, reply *[]*JSONSchemaProps) (err error) {
	editor.mtx.Lock()
	defer editor.mtx.Unlock()

	*reply = make([]*JSONSchemaProps, 0, len(editor.schemasBySchemaPath))
	for _, schema := range editor.schemasBySchemaPath {
		if !schema.HideFromList() {
			*reply = append(*reply, schema.Properties())
		}
	}
	sort.Sort(ByConfigThenSchemaPath(*reply))
	return
}

type EditorPathArgs struct {
	Path string `json:"path"`
}

type EditorPathResponse struct {
	Path string `json:"path"`
}

type EditorContentResponse struct {
	ConfigPath string                 `json:"configPath"`
	Content    *json.RawMessage       `json:"content"`
	Schema     map[string]interface{} `json:"schema"`
	Editor     string                 `json:"editor"`
}

func (editor *Editor) locateSchema(path string) (*JSONSchema, error) {
	schema, ok := editor.schemasBySchemaPath[path]
	if !ok {
		if schemas, ok := editor.schemasByConfigPath[path]; !ok || len(schemas) == 0 {
			return nil, fileNotFoundError
		} else {
			schema = schemas[0]
		}
	}
	return schema, nil
}

func (editor *Editor) Load(args *EditorPathArgs, reply *EditorContentResponse) error {
	schema, err := editor.locateSchema(args.Path)
	if err != nil {
		return err
	}

	bs, err := loadConfigBytes(schema.PhysicalConfigPath(), schema.ToJSONCommand())
	if err != nil {
		wbgong.Error.Printf("Failed to read config file %s: %s", schema.PhysicalConfigPath(), err)
		return invalidConfigError
	}
	printPreprocessorErrors(schema.PhysicalConfigPath(), bs.preprocessorErrors)

	if schema.ShouldValidate() {
		r, err := schema.ValidateContent(bs.content)
		if err != nil {
			wbgong.Error.Printf("Failed to validate config file %s: %s", schema.PhysicalConfigPath(), err)
			return invalidConfigError
		}
		if !r.Valid() {
			wbgong.Error.Printf("Invalid config file %s", schema.PhysicalConfigPath())
			for _, desc := range r.Errors() {
				wbgong.Error.Printf("- %s\n", desc)
			}
			return invalidConfigError
		}
	} else {
		if !json.Valid(bs.content) {
			return invalidConfigError
		}
	}

	content := json.RawMessage(bs.content) // TBD: use parsed config
	reply.ConfigPath = schema.ConfigPath()
	reply.Content = &content
	reply.Schema = fixFormatProps(schema.GetPreprocessed()).(map[string]interface{})
	reply.Editor = schema.Editor()

	return nil
}

type EditorSaveArgs struct {
	Path    string           `json:"path"`
	Content *json.RawMessage `json:"content"`
}

func (editor *Editor) Save(args *EditorSaveArgs, reply *EditorPathResponse) error {
	editor.mtx.Lock()
	defer editor.mtx.Unlock()

	schema, err := editor.locateSchema(args.Path)
	if err != nil {
		return err
	}
	if schema.ShouldValidate() {
		r, err := schema.ValidateContent(*args.Content)
		if err != nil {
			wbgong.Error.Printf("Failed to validate config file: %s", err)
			return invalidConfigError
		}
		if !r.Valid() {
			wbgong.Error.Printf("Invalid config file")
			for _, desc := range r.Errors() {
				wbgong.Error.Printf("- %s\n", desc)
			}
			return invalidConfigError
		}
	}

	var bs []byte
	var output RunCommandResult
	if schema.FromJSONCommand() != nil {
		output, err = extPreprocess(schema.FromJSONCommand(), *args.Content)
		if err != nil {
			wbgong.Error.Printf("external command error, %s: %s", schema.PhysicalConfigPath(), err)
			return writeError
		}
		bs = output.stdout.Bytes()
		if output.stderr.Len() != 0 {
			printPreprocessorErrors(schema.PhysicalConfigPath(), output.stderr.String())
		}
	} else {
		var indented bytes.Buffer
		if err = json.Indent(&indented, *args.Content, "", "    "); err != nil {
			wbgong.Error.Printf("json.Indent() error, %s: %s", schema.PhysicalConfigPath(), err)
			return writeError
		}
		bs = indented.Bytes()
	}

	if err = ioutil.WriteFile(schema.PhysicalConfigPath(), bs, 0777); err != nil {
		wbgong.Error.Printf("error writing %s: %s", schema.PhysicalConfigPath(), err)
		return writeError
	}

	if schema.RestartDelayMS() > 0 {
		editor.RequestCh <- Request{Sleep,map[string]string{"delay":strconv.Itoa(schema.RestartDelayMS())}}
	} else {
		editor.RequestCh <- Request{Sync,map[string]string{"path":schema.PhysicalConfigPath()}}
	}	

	reply.Path = args.Path
	if schema.Services() != nil {
		for _, service := range schema.Services() {
			editor.RequestCh <- Request{Restart,map[string]string{"service":service}}
		}
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

func NewEditorDirWatcherClient(editor *Editor) wbgong.DirWatcherClient {
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
