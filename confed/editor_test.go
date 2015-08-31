package confed

import (
	"github.com/contactless/wbgo"
	"github.com/stretchr/objx"
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
	editor, err := NewEditor(s.DataFilePath("wb-configs.json"))
	s.Ck("error creating the editor", err)
	s.RpcFixture = wbgo.NewRpcFixture(
		s.T(), "confed", "Editor", "confed",
		editor,
		"List", "Load")
}

func (s *EditorSuite) TearDownTest() {
	s.TearDownRPC()
	s.TearDownDataFiles()
	s.Suite.TearDownTest()
}

func (s *EditorSuite) addSampleFiles() {
	s.CopyDataFilesToTempDir("sample.json", "sample.schema.json", "wb-configs.json")
}

func (s *EditorSuite) TestListFiles() {
	s.VerifyRpc("List", objx.Map{}, []objx.Map{
		{
			"path":        "sample.json",
			"description": "Sample config file",
		},
	})
}

func (s *EditorSuite) TestLoadFile() {
	s.VerifyRpc("Load", objx.Map{"path": "sample.json"}, objx.Map{
		"content": objx.Map{
			"firstName": "foo",
			"lastName":  "bar",
			"age":       100,
		},
		"schema": objx.Map{
			"title": "Example Schema",
			"type":  "object",
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

func TestEditorSuite(t *testing.T) {
	wbgo.RunSuites(t, new(EditorSuite))
}

// TBD: list multiple configs in the catalog
// TBD: load errors
// TBD: validate configs when loading, don't list invalid configs
