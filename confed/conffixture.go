package confed

import (
	"github.com/contactless/wbgo/testutils"
	"testing"
)

type ConfFixture struct {
	*testutils.DataFileFixture
}

func NewConfFixture(t *testing.T) (f *ConfFixture) {
	f = &ConfFixture{testutils.NewDataFileFixture(t)}
	f.addSampleFiles()
	return
}

func (f *ConfFixture) addSampleFiles() {
	f.CopyDataFilesToTempDir(
		"sample.json",
		"sample.schema.json",
		"device_descriptions.schema.json",
		"device_descriptions_expected.schema.json",
		"sample-comments.json",
		"sample-badsyntax.json",
		"sample-invalid.json",
		"noconfig.schema.json",
		"sample-to-use-after-new-subconf.json",
		"sample_devtypes/msu21.conf",
		"sample_devtypes/wb-mrm2.conf",
		"sample_devtypes2/msu21.json",
		"sample_devtypes2/wb-mrm2.json",
		"sample_devtypes3/msu21.json",
		"sample_devtypes3/wb-mrm2.json")
}
