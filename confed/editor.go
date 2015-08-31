package confed

import (
	"encoding/json"
	"github.com/xeipuuv/gojsonschema"
	"path/filepath"
	"sort"
)

type confCatalog struct {
	Configs []confCatalogItem `json:"configs"`
}

type confCatalogItem struct {
	Description string `json:"description"`
	Path        string `json:"path"`
	Schema      string `json:"schema"`
}

type confFile struct {
	Description string `json:"description"`
	Path        string `json:"path"`
	schema      *gojsonschema.Schema
}

type Editor struct {
	basePath    string
	catalogPath string
	configs     map[string]*confFile
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
)

var invalidPathError = &EditorError{EDITOR_ERROR_INVALID_PATH, "Invalid path"}
var listDirError = &EditorError{EDITOR_ERROR_LISTDIR, "Error listing the directory"}
var writeError = &EditorError{EDITOR_ERROR_WRITE, "Error writing the file"}
var fileNotFoundError = &EditorError{EDITOR_ERROR_FILE_NOT_FOUND, "File not found"}
var rmError = &EditorError{EDITOR_ERROR_REMOVE, "Error removing the file"}
var readError = &EditorError{EDITOR_ERROR_READ, "Error reading the file"}

func NewEditor(catalogPath string) (editor *Editor, err error) {
	catalogPath, err = filepath.Abs(catalogPath)
	if err != nil {
		return
	}
	editor = &Editor{
		catalogPath: catalogPath,
		basePath:    filepath.Dir(catalogPath),
		configs:     make(map[string]*confFile),
	}
	err = editor.loadCatalog()
	return
}

func (editor *Editor) loadCatalog() (err error) {
	bs, err := loadConfigBytes(editor.catalogPath)
	if err != nil {
		return
	}
	var catalog confCatalog
	if err = json.Unmarshal(bs, &catalog); err != nil {
		return
	}
	for _, item := range catalog.Configs {
		schema, err := LoadSchema(filepath.Join(editor.basePath, item.Schema))
		if err != nil {
			return err
		}
		editor.configs[item.Path] = &confFile{
			Path:        item.Path,
			Description: item.Description,
			schema:      schema,
		}
	}
	return
}

func (editor *Editor) List(args *struct{}, reply *[]*confFile) (err error) {
	paths := make([]string, 0, len(editor.configs))
	for path := range editor.configs {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	*reply = make([]*confFile, len(editor.configs))
	for i, path := range paths {
		(*reply)[i] = editor.configs[path]
	}
	return
}
