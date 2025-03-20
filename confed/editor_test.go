package confed

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/objx"
	"github.com/wirenboard/wbgong/testutils"
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
      "enum": ["MSU21", "WB-MRM2"],
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
	EXPECTED_ALT_SCHEMA_CONTENT = `
{
  "type": "object",
  "title": "Example Config (alt)",
  "description": "Just an example (alt)",
  "properties": {
    "device_type": {
      "type": "string",
      "enum": ["MSU21", "WB-MRM2"],
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
	// Note that "_format" property name in another.schema.json
	// gets replaced with "format". That's necessary because
	// 'format' values intended for json-editor like 'checkbox'
	// for boolean may confuse gojsonschema
	EXPECTED_ANOTHER_SCHEMA_CONTENT = `
{
  "type": "object",
  "title": "Another Example Config",
  "properties": {
    "name": {
      "type": "string",
      "title": "Device name",
      "description": "Device name to be displayed in UI"
    },
    "active": {
      "type": "boolean",
      "title": "Active",
      "format": "checkbox"
    }
  },
  "required": ["name"],
  "configFile": {
    "path": "/another.json"
  }
}
`

	EXPECTED_INTERFACES_JSON = `
{
  "interfaces": [
    {
      "auto": true,
      "method": "static",
      "name": "eth0",
      "options": {
        "address": "172.16.200.77",
        "broadcast": "172.16.200.255",
        "gateway": "172.16.200.10",
        "netmask": "255.255.255.0"
      }
    }
  ]
}`
)

type EditorSuite struct {
	testutils.Suite
	*ConfFixture
	*testutils.RpcFixture
	editor *Editor
}

func (s *EditorSuite) T() *testing.T {
	return s.Suite.T()
}

func (s *EditorSuite) SetupTest() {
	s.Suite.SetupTest()
	s.ConfFixture = NewConfFixture(s.T())
	s.setupPathEnvVar()
	s.editor = NewEditor(s.DataFileTempDir())
	s.Ck("s.editor.loadSchema()", s.editor.loadSchema(s.DataFilePath("sample.schema.json")))
	s.RpcFixture = testutils.NewRpcFixture(
		s.T(), "confed", "Editor", "confed",
		s.editor,
		"List", "Load", "Save")
}

func (s *EditorSuite) TearDownTest() {
	s.editor.stopWatchingDependentFiles()
	s.TearDownRPC()
	s.TearDownDataFiles()
	s.Suite.TearDownTest()
}

func (s *EditorSuite) setupPathEnvVar() {
	// add directory with 'networkparser' to the front of $PATH
	path := os.Getenv("PATH")
	if path == "" {
		os.Setenv("PATH", s.SourceDir()+"/..")
	} else {
		os.Setenv("PATH", s.SourceDir()+"/..:"+path)
	}

}

func (s *EditorSuite) verifyInitialSchemaList() {
	s.VerifyRpc("List", objx.Map{}, []objx.Map{
		{
			"configPath":  "/sample.json",
			"schemaPath":  "/sample.schema.json",
			"title":       "Example Config",
			"description": "Just an example",
		},
	})
}

func (s *EditorSuite) SkipTestListFiles() {
	s.verifyInitialSchemaList()
}

func (s *EditorSuite) verifyLoadSampleJson() {
	s.VerifyRpc("Load", objx.Map{"path": "/sample.json"}, objx.Map{
		"configPath": "/sample.json",
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

func (s *EditorSuite) SkipTestLoadFile() {
	s.verifyLoadSampleJson()
	s.CopyDataFilesToTempDir("another.schema.json", "another.json")
	s.Ck("loadSchema()", s.editor.loadSchema("another.schema.json"))
	for _, path := range []string{"/another.json", "/another.schema.json"} {
		s.VerifyRpc("Load", objx.Map{"path": path}, objx.Map{
			"configPath": "/another.json",
			"content": objx.Map{
				"name": "foobar",
			},
			"schema": objx.MustFromJSON(EXPECTED_ANOTHER_SCHEMA_CONTENT),
		})
	}
}

func (s *EditorSuite) verifyJSONFile(path string, expectedContent objx.Map) {
	bs, err := os.ReadFile(s.DataFilePath(path))
	s.Ck("ReadFile()", err)
	s.Equal(expectedContent, objx.MustFromJSON(string(bs)))
}

func (s *EditorSuite) verifyTextFile(path string, expectedContent string) {
	bs, err := os.ReadFile(s.DataFilePath(path))
	s.Ck("ReadFile()", err)
	s.Equal(expectedContent, string(bs))
}

func (s *EditorSuite) SkipTestSaveFile() {
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

	newContent["id"] = "msu21xxx"
	s.VerifyRpc("Save", objx.Map{
		"path":    "/sample.schema.json",
		"content": newContent,
	}, objx.Map{
		"path": "/sample.schema.json",
	})
	s.verifyJSONFile("sample.json", newContent)
}

func (s *EditorSuite) SkipTestSaveInvalidConfig() {
	s.VerifyRpcError("Save", objx.Map{
		"path":    "/sample.json",
		"content": objx.Map{"wtf": 100},
	}, EDITOR_ERROR_INVALID_CONFIG, "EditorError", "Invalid config file")
}

func (s *EditorSuite) SkipTestAddSchema() {
	s.verifyInitialSchemaList()
	s.CopyDataFilesToTempDir("another.schema.json", "another.json")
	dwc := NewEditorDirWatcherClient(s.editor)
	s.Ck("LiveLoadFile()", dwc.LiveLoadFile(s.DataFilePath("another.schema.json")))
	s.VerifyRpc("List", objx.Map{}, []objx.Map{
		{
			"configPath":  "/another.json",
			"schemaPath":  "/another.schema.json",
			"title":       "Another Example Config",
			"description": "",
		},
		{
			"configPath":  "/sample.json",
			"schemaPath":  "/sample.schema.json",
			"title":       "Example Config",
			"description": "Just an example",
		},
	})
	s.verifyLoadSampleJson()
	s.VerifyRpc("Load", objx.Map{"path": "/another.json"}, objx.Map{
		"configPath": "/another.json",
		"content": objx.Map{
			"name": "foobar",
		},
		"schema": objx.MustFromJSON(EXPECTED_ANOTHER_SCHEMA_CONTENT),
	})
}

func (s *EditorSuite) SkipTestRemoveSchema() {
	s.verifyInitialSchemaList()
	s.CopyDataFilesToTempDir("another.schema.json", "another.json")
	s.RmFile("sample.schema.json")
	dwc := NewEditorDirWatcherClient(s.editor)
	s.Ck("LiveLoadFile()", dwc.LiveLoadFile(s.DataFilePath("another.schema.json")))
	s.Ck("LiveRemoveFile()", dwc.LiveRemoveFile(s.DataFilePath("sample.schema.json")))
	s.VerifyRpc("List", objx.Map{}, []objx.Map{
		{
			"configPath":  "/another.json",
			"schemaPath":  "/another.schema.json",
			"title":       "Another Example Config",
			"description": "",
		},
	})
	s.VerifyRpcError("Load", objx.Map{"path": "/sample.json"},
		EDITOR_ERROR_FILE_NOT_FOUND, "EditorError", "File not found")
	s.VerifyRpc("Load", objx.Map{"path": "/another.json"}, objx.Map{
		"configPath": "/another.json",
		"content": objx.Map{
			"name": "foobar",
		},
		"schema": objx.MustFromJSON(EXPECTED_ANOTHER_SCHEMA_CONTENT),
	})
}

func (s *EditorSuite) loadInterfacesConf() {
	s.CopyDataFilesToTempDir(
		"interfaces.schema.json",
		"interfaces:etc/network/interfaces")
	s.Ck("s.editor.loadSchema()", s.editor.loadSchema(s.DataFilePath("interfaces.schema.json")))
}

func (s *EditorSuite) SkipTestListPreprocessed() {
	s.loadInterfacesConf()
	s.VerifyRpc("List", objx.Map{}, []objx.Map{
		{
			"configPath":  "/etc/network/interfaces",
			"schemaPath":  "/interfaces.schema.json",
			"title":       "Network Interface Configuration",
			"description": "Specifies network configuration of the system",
		},
		{
			"configPath":  "/sample.json",
			"schemaPath":  "/sample.schema.json",
			"title":       "Example Config",
			"description": "Just an example",
		},
	})
}

func (s *EditorSuite) SkipTestLoadPreprocessed() {
	s.loadInterfacesConf()
	s.VerifyRpc("Load", objx.Map{"path": "/etc/network/interfaces"}, objx.Map{
		"configPath": "/etc/network/interfaces",
		"content":    objx.MustFromJSON(EXPECTED_INTERFACES_JSON),
		"schema": objx.MustFromJSON(
			strings.Replace(
				s.ReadSourceDataFile("interfaces.schema.json"),
				"_format", "format", -1)),
	})
}

var newIfacesContent = objx.Map{
	"interfaces": []interface{}{
		map[string]interface{}{
			"name":   "eth0",
			"auto":   true,
			"method": "dhcp",
			"options": map[string]interface{}{
				"hostname": "WirenBoard",
			},
		},
	},
}

func (s *EditorSuite) SkipTestSavePreprocessed() {
	s.loadInterfacesConf()
	s.VerifyRpc("Save", objx.Map{
		"path":    "/etc/network/interfaces",
		"content": newIfacesContent,
	}, objx.Map{
		"path": "/etc/network/interfaces",
	})
	s.verifyTextFile("etc/network/interfaces", `auto eth0
iface eth0 inet dhcp
  hostname WirenBoard

`)
	// FIXME: link-local section is disabled for now
	//
	// auto eth0:42
	// iface eth0:42 inet static
	//   address 169.254.42.42
	//   netmask 255.255.0.0
	// `)
}

func (s *EditorSuite) SkipTestRestart() {
	s.loadInterfacesConf()
	s.VerifyRpc("Save", objx.Map{
		"path":    "/etc/network/interfaces",
		"content": newIfacesContent,
	}, objx.Map{
		"path": "/etc/network/interfaces",
	})
	restart := <-s.editor.RequestCh
	s.Equal(Request{Sleep, map[string]string{"delay": "4000"}}, restart)
	restart = <-s.editor.RequestCh
	s.Equal(Request{Restart, map[string]string{"service": "networking"}}, restart)
}

func (s *EditorSuite) SkipTestMultipleSchemasPerConfig() {
	s.CopyDataFilesToTempDir("sample-extra.schema.json")
	s.Ck("loadSchema()", s.editor.loadSchema("sample-extra.schema.json"))
	s.VerifyRpc("List", objx.Map{}, []objx.Map{
		{
			"configPath":  "/sample.json",
			"schemaPath":  "/sample-extra.schema.json",
			"title":       "Example Config (alt)",
			"description": "Just an example (alt)",
		},
		{
			"configPath":  "/sample.json",
			"schemaPath":  "/sample.schema.json",
			"title":       "Example Config",
			"description": "Just an example",
		},
	})
	content := objx.Map{
		"device_type": "MSU21",
		"name":        "MSU21",
		"id":          "msu21",
		"slave_id":    float64(24),
		"enabled":     true,
	}
	s.VerifyRpc("Load", objx.Map{"path": "/sample.schema.json"}, objx.Map{
		"configPath": "/sample.json",
		"content":    content,
		"schema":     objx.MustFromJSON(EXPECTED_SCHEMA_CONTENT),
	})
	s.VerifyRpc("Load", objx.Map{"path": "/sample-extra.schema.json"}, objx.Map{
		"configPath": "/sample.json",
		"content":    content,
		"schema":     objx.MustFromJSON(EXPECTED_ALT_SCHEMA_CONTENT),
	})

	content["id"] = "msu21xxx"
	s.VerifyRpc("Save", objx.Map{
		"path":    "/sample.schema.json",
		"content": content,
	}, objx.Map{
		"path": "/sample.schema.json",
	})
	s.verifyJSONFile("sample.json", content)

	content["id"] = "msu21yyy"
	s.VerifyRpc("Save", objx.Map{
		"path":    "/sample-extra.schema.json",
		"content": content,
	}, objx.Map{
		"path": "/sample-extra.schema.json",
	})
	s.verifyJSONFile("sample.json", content)
}

func TestEditorSuite(t *testing.T) {
	testutils.RunSuites(t, new(EditorSuite))
}

// TBD: test schema removal
// TBD: test multiple configs
// TBD: test load errors (including invalid config errors)
// TBD: test errors upon writing unlisted files
// TBD: test reloading schemas (possibly with other config path)
// TBD: test config path conflict between schemas
// TBD: rm unused error types
// TBD: handle relative paths (incl. enum subconf paths) properly:
//      they should be relative to the schema file, not the current
//      directory
// TBD: add propertyOrder to schema properties
//      (it works without it, but that's not good to rely on this behavior)
//      (write Emacs func for it)
// TBD: for schema editor: disable_properties, no_additional_properties
// TBD: always provide absolute paths to configs
//      (this helps with URLs on the client side)
// Later: resolve $ref when loading config
// so as to avoid using complicated loading mechanism
// on the client
