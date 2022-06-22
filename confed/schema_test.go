package confed

import (
	"github.com/wirenboard/wbgong/testutils"
	"github.com/xeipuuv/gojsonschema"
	"testing"
)

type SchemaSuite struct {
	testutils.Suite
	*ConfFixture
	schema *JSONSchema
}

func (s *SchemaSuite) SetupTest() {
	s.Suite.SetupTest()
	s.ConfFixture = NewConfFixture(s.T())
	var err error
	s.schema, err = NewJSONSchemaWithRoot("sample.schema.json", s.DataFileTempDir())
	s.Ck("error loading schema", err)
}

func (s *SchemaSuite) TearDownTest() {
	s.schema.StopWatchingSubconfigs()
	s.TearDownDataFiles()
	s.Suite.TearDownTest()
}

func (s *SchemaSuite) validate(path string) (r *gojsonschema.Result) {
	r, err := s.schema.ValidateFile(path)
	s.Ck("validation error", err)
	return
}

func (s *SchemaSuite) verifyValid(docPath string) {
	s.True(s.validate(docPath).Valid(), "%s must be valid", docPath)
}

func (s *SchemaSuite) verifyInvalid(docPath string) {
	s.False(s.validate(docPath).Valid(), "%s must be invalid", docPath)
}

func (s *SchemaSuite) verifyError(docPath, schemaPath string) {
	schema, err := NewJSONSchemaWithRoot(schemaPath, s.DataFileTempDir())
	s.Ck("error loading schema", err)
	defer schema.StopWatchingSubconfigs()
	_, validationError := schema.ValidateFile(docPath)
	s.NotNil(validationError)
}

func (s *SchemaSuite) TestValidation() {
	s.verifyValid("sample.json")
	s.verifyValid("sample-comments.json")
	s.verifyInvalid("sample-invalid.json")
	s.verifyError("sample-badsyntax.json", "sample.schema.json")
	s.verifyError("nosuchfile.json", "sample.schema.json")
	_, err := NewJSONSchemaWithRoot("nosuchfile.schema.json", s.DataFileTempDir())
	s.NotNil(err)
	_, err = NewJSONSchemaWithRoot("noconfig.schema.json", s.DataFileTempDir())
	s.NotNil(err)
}

func (s *SchemaSuite) TestSchemaProperties() {
	s.Equal("/sample.schema.json", s.schema.Path())
	s.Equal("/sample.json", s.schema.ConfigPath())
	s.Equal(s.DataFilePath("sample.json"), s.schema.PhysicalConfigPath())
	s.Equal("Example Config", s.schema.Title())
	s.Equal("Just an example", s.schema.Description())
}

func (s *SchemaSuite) TestAddingSubconf() {
	s.verifyValid("sample.json") // initialize schema to make sure it's updated properly later
	s.WriteDataFile("sample_devtypes/whatever.conf", `{"device_type": "Whatever"}`)
	s.WaitFor(func() bool { return s.schema.enumLoader.IsDirty() })
	s.verifyValid("sample-to-use-after-new-subconf.json")
}

func TestSchemaSuite(t *testing.T) {
	testutils.RunSuites(t, new(SchemaSuite))
}
