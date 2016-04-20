package confed

import (
	"encoding/json"
	"fmt"
	"github.com/contactless/wbgo/testutils"
	"os"
	"testing"
)

const (
	EXPECTED_SCHEMA_CONTENT_TMPL = `
{
  "type": "object",
  "title": "Example Config",
  "description": "Just an example",
  "properties": {
    "device_type": {
      "type": "string",
      "enum": %s,
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
  "configFile": {
    "path": "/sample.json"
  }
}
`
)

type EnumLoaderSuite struct {
	testutils.Suite
	*ConfFixture
	enumLoader *enumLoader
}

func (s *EnumLoaderSuite) T() *testing.T {
	return s.Suite.T()
}

func (s *EnumLoaderSuite) SetupTest() {
	s.Suite.SetupTest()
	s.ConfFixture = NewConfFixture(s.T())
	s.enumLoader = newEnumLoader(s.DataFileTempDir())
}

func (s *EnumLoaderSuite) TearDownTest() {
	s.enumLoader.StopWatchingSubconfigs()
	s.TearDownDataFiles()
	s.Suite.TearDownTest()
}

func (s *EnumLoaderSuite) expectedContent(enumSubst string) (r map[string]interface{}) {
	bs := []byte(fmt.Sprintf(EXPECTED_SCHEMA_CONTENT_TMPL, enumSubst))
	s.Ck("Unmarshal expected JSON", json.Unmarshal(bs, &r))
	return
}

func (s *EnumLoaderSuite) verifyEnum(enumSubst string) {
	bs, err := loadConfigBytes(s.DataFilePath("sample.schema.json"), nil)
	s.Ck("loadConfigBytes()", err)
	var m map[string]interface{}
	s.Ck("Unmarshal JSON", json.Unmarshal(bs, &m))
	// TBD: specify the base directory for Preprocess
	s.Equal(s.expectedContent(enumSubst), s.enumLoader.Preprocess(m))
	s.False(s.enumLoader.IsDirty())
}

func (s *EnumLoaderSuite) verifyInitial() {
	s.True(s.enumLoader.IsDirty())
	s.verifyEnum(`["MSU21", "WB-MRM2"]`)
}

func (s *EnumLoaderSuite) TestLoading() {
	s.verifyInitial()
}

func (s *EnumLoaderSuite) TestChangeSubconf() {
	s.verifyInitial()
	s.WriteDataFile("sample_devtypes/msu21.conf", `{"device_type": "m.s.u.21"}`)
	s.WaitFor(func() bool { return s.enumLoader.IsDirty() })
	s.verifyEnum(`["WB-MRM2", "m.s.u.21"]`)
}

func (s *EnumLoaderSuite) TestNewSubconf() {
	s.verifyInitial()
	s.WriteDataFile("sample_devtypes/whatever.conf", `{"device_type": "Whatever"}`)
	s.WaitFor(func() bool { return s.enumLoader.IsDirty() })
	s.verifyEnum(`["MSU21", "WB-MRM2", "Whatever"]`)
}

func (s *EnumLoaderSuite) TestRemoveSubconf() {
	s.verifyInitial()
	s.Ck("os.Remove()", os.Remove(s.DataFilePath("sample_devtypes/msu21.conf")))
	s.WaitFor(func() bool { return s.enumLoader.IsDirty() })
	s.verifyEnum(`["WB-MRM2"]`)
}

func TestEnumLoaderSuite(t *testing.T) {
	testutils.RunSuites(t, new(EnumLoaderSuite))
}

// TBD: test malformed files
