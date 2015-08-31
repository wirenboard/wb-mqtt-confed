package confed

import (
	"github.com/contactless/wbgo"
	"github.com/stretchr/objx"
	"testing"
)

const (
	SAMPLE_CLIENT_ID = "11111111"
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
		"List")
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

func TestEditorSuite(t *testing.T) {
	wbgo.RunSuites(t, new(EditorSuite))
}

// TBD: list multiple configs in the catalog
