package confed

import (
	"github.com/contactless/wbgo"
	"github.com/xeipuuv/gojsonschema"
	"os"
	"testing"
)

type SchemaSuite struct {
	wbgo.Suite
	wd     string
	schema *JSONSchema
}

func (s *SchemaSuite) SetupTest() {
	s.Suite.SetupTest()
	var err error
	s.wd, err = os.Getwd()
	s.Ck("Getwd", err)
	s.schema, err = NewJSONSchemaWithRoot("sample.schema.json", s.wd)
	s.Ck("error loading schema", err)
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
	schema, err := NewJSONSchemaWithRoot(schemaPath, s.wd)
	s.Ck("error loading schema", err)
	_, validationError := schema.ValidateFile(docPath)
	s.NotNil(validationError)
}

func (s *SchemaSuite) TestValidation() {
	s.verifyValid("sample.json")
	s.verifyValid("sample-comments.json")
	s.verifyInvalid("sample-invalid.json")
	s.verifyError("sample-badsyntax.json", "sample.schema.json")
	s.verifyError("nosuchfile.json", "sample.schema.json")
	_, err := NewJSONSchemaWithRoot("nosuchfile.schema.json", s.wd)
	s.NotNil(err)
	_, err = NewJSONSchemaWithRoot("noconfig.schema.json", s.wd)
	s.NotNil(err)
}

func (s *SchemaSuite) TestSchemaProperties() {
	s.Equal("/sample.schema.json", s.schema.Path())
	s.Equal("/sample.json", s.schema.ConfigPath())
	s.Equal("Example Config", s.schema.Title())
	s.Equal("Just an example", s.schema.Description())
}

func TestSchemaSuite(t *testing.T) {
	wbgo.RunSuites(t, new(SchemaSuite))
}
