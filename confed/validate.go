package confed

import (
	"github.com/xeipuuv/gojsonschema"
	"path/filepath"
)

func LoadSchema(schemaFile string) (schema *gojsonschema.Schema, err error) {
	schemaPath, err := filepath.Abs(schemaFile)
	if err != nil {
		return
	}
	schemaLoader := gojsonschema.NewReferenceLoader("file://" + schemaPath)
	return gojsonschema.NewSchema(schemaLoader)
}

func ValidateJSON(jsonFile, schemaFile string) (result *gojsonschema.Result, err error) {
	bs, err := loadConfigBytes(jsonFile)
	if err != nil {
		return
	}
	documentLoader := gojsonschema.NewStringLoader(string(bs))
	schema, err := LoadSchema(schemaFile)
	if err != nil {
		return
	}
	return schema.Validate(documentLoader)
}
