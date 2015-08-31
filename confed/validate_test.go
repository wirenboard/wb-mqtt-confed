package confed

import (
	"github.com/contactless/wbgo"
	"github.com/xeipuuv/gojsonschema"
	"testing"
)

type ValidationSuite struct {
	wbgo.Suite
}

func (s *ValidationSuite) validate(path string) (r *gojsonschema.Result) {
	r, err := ValidateJSON(path, "sample.schema.json")
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
	_, err := ValidateJSON(docPath, schemaPath)
	s.NotNil(err)
}

func (s *ValidationSuite) TestValidation() {
	s.verifyValid("sample.json")
	s.verifyValid("sample-comments.json")
	s.verifyInvalid("sample-invalid.json")
	s.verifyError("sample-badsyntax.json", "sample.schema.json")
	s.verifyError("nosuchfile.json", "sample.schema.json")
	s.verifyError("sample.json", "nosuchschema.json")
}

func TestValidationSuite(t *testing.T) {
	wbgo.RunSuites(t, new(ValidationSuite))
}
