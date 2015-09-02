package confed

import (
	"github.com/contactless/wbgo"
	"github.com/stretchr/objx"
	"io/ioutil"
	"testing"
)

type EditorSuite struct {
	wbgo.Suite
	*wbgo.DataFileFixture
	*wbgo.RpcFixture
}

func (s *EditorSuite) T() *testing.T {
	return s.Suite.T()
}

func (s *EditorSuite) SetupTest() {
	s.Suite.SetupTest()
	s.DataFileFixture = wbgo.NewDataFileFixture(s.T())
	s.addSampleFiles()
	editor := NewEditor()
	editor.setRoot(s.DataFileTempDir())
	err := editor.loadSchema(s.DataFilePath("sample.schema.json"))
	s.Ck("error creating the editor", err)
	s.RpcFixture = wbgo.NewRpcFixture(
		s.T(), "confed", "Editor", "confed",
		editor,
		"List", "Load", "Save")
}

func (s *EditorSuite) TearDownTest() {
	s.TearDownRPC()
	s.TearDownDataFiles()
	s.Suite.TearDownTest()
}

func (s *EditorSuite) addSampleFiles() {
	s.CopyDataFilesToTempDir("sample.json", "sample.schema.json")
}

func (s *EditorSuite) TestListFiles() {
	s.VerifyRpc("List", objx.Map{}, []objx.Map{
		{
			"configPath":  "/sample.json",
			"title":       "Example Config",
			"description": "Just an example",
		},
	})
}

func (s *EditorSuite) TestLoadFile() {
	s.VerifyRpc("Load", objx.Map{"path": "/sample.json"}, objx.Map{
		"content": objx.Map{
			"firstName": "foo",
			"lastName":  "bar",
			"age":       100,
		},
		"schema": objx.Map{
			"configPath":  "sample.json",
			"title":       "Example Config",
			"description": "Just an example",
			"type":        "object",
			"properties": objx.Map{
				"firstName": objx.Map{
					"type": "string",
				},
				"lastName": objx.Map{
					"type": "string",
				},
				"age": objx.Map{
					"description": "Age in years",
					"type":        "integer",
					"minimum":     0,
				},
			},
			"required": []interface{}{
				"firstName",
				"lastName",
			},
		},
	})
}

func (s *EditorSuite) verifyJSONFile(path string, expectedContent objx.Map) {
	bs, err := ioutil.ReadFile(s.DataFilePath(path))
	s.Ck("ReadFile()", err)
	s.Equal(expectedContent, objx.MustFromJSON(string(bs)))
}

func (s *EditorSuite) TestSaveFile() {
	newContent := objx.Map{
		"age":       float64(4242),
		"firstName": "qqq",
		"lastName":  "rrr",
	}
	s.VerifyRpc("Save", objx.Map{
		"path":    "/sample.json",
		"content": newContent,
	}, objx.Map{
		"path": "/sample.json",
	})
	s.verifyJSONFile("sample.json", newContent)
}

func (s *EditorSuite) TestSaveInvalidConfig() {
	s.VerifyRpcError("Save", objx.Map{
		"path":    "/sample.json",
		"content": objx.Map{"wtf": 100},
	}, EDITOR_ERROR_INVALID_CONFIG, "EditorError", "Invalid config file")
}

func TestEditorSuite(t *testing.T) {
	wbgo.RunSuites(t, new(EditorSuite))
}

// TBD: test load errors (including invalid config errors)
// TBD: test errors upon writing unlisted files
// TBD: test schema file removal (not via RPC API)
// TBD: test $ref
// TBD: test reloading schemas (possibly with other config path)
// TBD: test config path conflict between schemas
// TBD: use parsed json configs (MSIs), not byte slices
// TBD: rm unused error types
// TBD: modbus device_type handling (use json pointer)
