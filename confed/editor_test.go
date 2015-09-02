package confed

import (
	"github.com/contactless/wbgo"
	"github.com/stretchr/objx"
	"io/ioutil"
	"testing"
)

const (
	EXPECTED_SCHEMA_CONTENT = `
{
  "type": "object",
  "title": "Example Config",
  "description": "Just an example",
  "properties": {
    "device_type": {
      "type": "string",
      "enum": ["MSU21"],
      "title": "Device type",
      "description": "Modbus device template to use"
    },
    "name": {
      "type": "string",
      "title": "Device name",
      "description": "Device name to be displayed in UI"
    },
    "id": {
      "type": "string",
      "title": "Device ID",
      "description": "Device identifier to be used as a part of MQTT topic"
    },
    "enabled": {
      "type": "boolean",
      "title": "Enabled",
      "description": "Check to enable device polling"
    },
    "slave_id": {
      "type": "integer",
      "title": "Slave ID",
      "description": "Modbus Slave ID",
      "minimum": 0
    }
  },
  "required": ["device_type", "slave_id"],
  "configPath": "sample.json"
}
`
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
			"device_type": "MSU21",
			"name":        "MSU21",
			"id":          "msu21",
			"slave_id":    float64(24),
			"enabled":     true,
		},
		"schema": objx.MustFromJSON(EXPECTED_SCHEMA_CONTENT),
	})
}

func (s *EditorSuite) verifyJSONFile(path string, expectedContent objx.Map) {
	bs, err := ioutil.ReadFile(s.DataFilePath(path))
	s.Ck("ReadFile()", err)
	s.Equal(expectedContent, objx.MustFromJSON(string(bs)))
}

func (s *EditorSuite) TestSaveFile() {
	newContent := objx.Map{
		"device_type": "MSU21",
		"name":        "MSU21 (updated)",
		"id":          "msu21",
		"slave_id":    float64(42),
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

// TBD: test multiple configs
// TBD: test load errors (including invalid config errors)
// TBD: test errors upon writing unlisted files
// TBD: test schema file removal (not via RPC API)
// TBD: test reloading schemas (possibly with other config path)
// TBD: test config path conflict between schemas
// TBD: rm unused error types
// TBD: modbus device_type handling (use json pointer)
// Later: resolve $ref when loading config
// so as to avoid using complicated loading mechanism
// on the client
