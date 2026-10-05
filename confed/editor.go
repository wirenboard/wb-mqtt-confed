// Package confed implements loading, validation and saving of JSON
// configuration files described by JSON schemas.
package confed

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/wirenboard/wbgong"
)

const (
	restartQueueLen = 100
)

func fixFormatProps(v any) any {
	switch v := v.(type) {
	case map[string]any:
		r := make(map[string]any)
		for k, item := range v {
			if k == "_format" {
				r["format"] = fixFormatProps(item)
			} else {
				r[k] = fixFormatProps(item)
			}
		}
		return r
	case []any:
		r := make([]any, len(v))
		for n, item := range v {
			r[n] = fixFormatProps(item)
		}
		return r
	default:
		return v
	}
}

func printPreprocessorErrors(configPath, errors string) {
	if errors != "" {
		for line := range strings.SplitSeq(strings.TrimSpace(errors), "\n") {
			wbgong.Warn.Printf("config preprocessor of %s printed in stderr: %s", configPath, line)
		}
	}
}

// RequestType is the kind of a request processed by RunRequestHandler.
type RequestType int64

// Request types processed by RunRequestHandler.
const (
	Sleep RequestType = iota
	Restart
)

// Request is a deferred action to be performed after a config is saved.
type Request struct {
	requestType RequestType
	properties  map[string]string
}

// Editor is an MQTT RPC service providing access to configs
// described by the loaded schemas.
type Editor struct {
	mtx                 sync.Mutex
	root                string
	schemasByConfigPath map[string][]*JSONSchema
	schemasBySchemaPath map[string]*JSONSchema
	RequestCh           chan Request
}

// EditorError is an error returned to RPC clients along with its code.
type EditorError struct {
	code    int32
	message string
}

func (err *EditorError) Error() string {
	return err.message
}

// ErrorCode returns the RPC error code.
func (err *EditorError) ErrorCode() int32 {
	return err.code
}

// RPC error codes.
// No iota here because these values may be used by external software.
const (
	EditorErrorWrite         = 1002
	EditorErrorFileNotFound  = 1003
	EditorErrorInvalidConfig = 1006
)

var (
	writeError         = &EditorError{EditorErrorWrite, "Error writing the file"}
	fileNotFoundError  = &EditorError{EditorErrorFileNotFound, "File not found"}
	invalidConfigError = &EditorError{EditorErrorInvalidConfig, "Invalid config file"}
	noContentError     = &EditorError{EditorErrorInvalidConfig, "No config content in the request"}
)

// NewEditor creates an Editor for configs located under the root directory.
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
		RequestCh:           make(chan Request, restartQueueLen),
	}
}

func (editor *Editor) loadSchema(path string) (err error) {
	editor.mtx.Lock()
	defer editor.mtx.Unlock()

	wbgong.Debug.Printf("Loading schema file: %s", path)
	schema, err := NewJSONSchemaWithRoot(path, editor.root)
	if err != nil {
		wbgong.Error.Printf("Error loading schema: %v", err)
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

	schema.StopWatchingDependentFiles()
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

// ByConfigThenSchemaPath sorts schema properties by config path, then by schema path.
type ByConfigThenSchemaPath []*JSONSchemaProps

func (b ByConfigThenSchemaPath) Len() int      { return len(b) }
func (b ByConfigThenSchemaPath) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b ByConfigThenSchemaPath) Less(i, j int) bool {
	if b[i].ConfigPath == b[j].ConfigPath {
		return b[i].SchemaPath < b[j].SchemaPath
	}
	return b[i].ConfigPath < b[j].ConfigPath
}

// List is an RPC method returning properties of all visible schemas.
func (editor *Editor) List(_ *struct{}, reply *[]*JSONSchemaProps) (err error) {
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

// EditorPathArgs are arguments of the Load RPC method.
type EditorPathArgs struct {
	Path string `json:"path"`
}

// EditorPathResponse is a reply of the Save RPC method.
type EditorPathResponse struct {
	Path string `json:"path"`
}

// EditorContentResponse is a reply of the Load RPC method.
type EditorContentResponse struct {
	ConfigPath string           `json:"configPath"`
	Content    *json.RawMessage `json:"content"`
	Schema     map[string]any   `json:"schema"`
	Editor     string           `json:"editor"`
}

func (editor *Editor) locateSchema(path string) (*JSONSchema, error) {
	schema, ok := editor.schemasBySchemaPath[path]
	if !ok {
		schemas, ok := editor.schemasByConfigPath[path]
		if !ok || len(schemas) == 0 {
			return nil, fileNotFoundError
		}
		schema = schemas[0]
	}
	return schema, nil
}

// Load is an RPC method returning a config with its preprocessed schema.
// The path may be either a config path or a schema path.
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
	} else if !json.Valid(bs.content) {
		return invalidConfigError
	}

	content := json.RawMessage(bs.content) // TBD: use parsed config
	reply.ConfigPath = schema.ConfigPath()
	reply.Content = &content
	reply.Schema = fixFormatProps(schema.GetPreprocessed()).(map[string]any)
	reply.Editor = schema.Editor()

	return nil
}

// EditorSaveArgs are arguments of the Save RPC method.
type EditorSaveArgs struct {
	Path    string           `json:"path"`
	Content *json.RawMessage `json:"content"`
}

// Save is an RPC method validating and writing a config, then restarting
// the services specified in its schema.
func (editor *Editor) Save(args *EditorSaveArgs, reply *EditorPathResponse) error {
	if args.Content == nil {
		wbgong.Error.Printf("Save request for %s contains no content", args.Path)
		return noContentError
	}

	editor.mtx.Lock()
	defer editor.mtx.Unlock()

	schema, err := editor.locateSchema(args.Path)
	if err != nil {
		return err
	}
	if schema.ShouldValidate() {
		r, validateErr := schema.ValidateContent(*args.Content)
		if validateErr != nil {
			wbgong.Error.Printf("Failed to validate config file: %v", validateErr)
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

	if err = wbgong.WriteFileAtomic(schema.PhysicalConfigPath(), bytes.NewReader(bs), 0o777); err != nil {
		wbgong.Error.Printf("error writing %s: %s", schema.PhysicalConfigPath(), err)
		return writeError
	}

	if schema.RestartDelayMS() > 0 {
		editor.RequestCh <- Request{Sleep, map[string]string{"delay": strconv.Itoa(schema.RestartDelayMS())}}
	}

	reply.Path = args.Path
	if schema.Services() != nil {
		for _, service := range schema.Services() {
			editor.RequestCh <- Request{Restart, map[string]string{"service": service}}
		}
	}
	return nil
}

func (editor *Editor) stopWatchingDependentFiles() {
	for _, schema := range editor.schemasBySchemaPath {
		schema.StopWatchingDependentFiles()
	}
}

// We don't provide LoadFile / LiveLoadFile / LiveRemoveFile
// for *Editor itself in order to avoid RPC server warnings
// about improper methods.

// EditorDirWatcherClient loads and removes schemas of an Editor
// upon directory watcher events.
type EditorDirWatcherClient struct {
	editor *Editor
}

// NewEditorDirWatcherClient creates a directory watcher client for the editor.
func NewEditorDirWatcherClient(editor *Editor) wbgong.DirWatcherClient {
	return &EditorDirWatcherClient{editor}
}

// LoadFile loads a schema file.
func (c *EditorDirWatcherClient) LoadFile(path string) error {
	return c.editor.loadSchema(path)
}

// LiveLoadFile reloads a changed schema file.
func (c *EditorDirWatcherClient) LiveLoadFile(path string) error {
	return c.LoadFile(path)
}

// LiveRemoveFile removes a deleted schema file.
func (c *EditorDirWatcherClient) LiveRemoveFile(path string) error {
	return c.editor.removeSchema(path)
}
