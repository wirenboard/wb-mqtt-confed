package confed

import (
	"github.com/contactless/wbgo"
	"github.com/xeipuuv/gojsonschema"
	"testing"
)

type ValidationSuite struct {
	wbgo.Suite
	schema *JSONSchema
}

func (s *ValidationSuite) SetupTest() {
	s.Suite.SetupTest()
	var err error
	s.schema, err = NewJSONSchema("sample.schema.json")
	s.Ck("error loading schema", err)
}

func (s *ValidationSuite) validate(path string) (r *gojsonschema.Result) {
	r, err := s.schema.ValidateFile(path)
	s.Ck("validation error", err)
	return
}

func (s *ValidationSuite) verifyValid(docPath string) {
	s.True(s.validate(docPath).Valid(), "%s must be valid", docPath)
}

func (s *ValidationSuite) verifyInvalid(docPath string) {
	s.False(s.validate(docPath).Valid(), "%s must be invalid", docPath)
}

func (s *ValidationSuite) verifyError(docPath, schemaPath string) {
	schema, err := NewJSONSchema(schemaPath)
	s.Ck("error loading schema", err)
	_, validationError := schema.ValidateFile(docPath)
	s.NotNil(validationError)
}

func (s *ValidationSuite) TestValidation() {
	s.verifyValid("sample.json")
	s.verifyValid("sample-comments.json")
	s.verifyInvalid("sample-invalid.json")
	s.verifyError("sample-badsyntax.json", "sample.schema.json")
	s.verifyError("nosuchfile.json", "sample.schema.json")
	_, err := NewJSONSchema("nosuchfile.schema.json")
	s.NotNil(err)
}

func TestValidationSuite(t *testing.T) {
	wbgo.RunSuites(t, new(ValidationSuite))
}
