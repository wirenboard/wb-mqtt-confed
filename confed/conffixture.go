//go:build test

package confed

import (
	"testing"

	"github.com/wirenboard/wbgong/testutils"
)

// ConfFixture provides sample config and schema files for tests.
type ConfFixture struct {
	*testutils.DataFileFixture
}

// NewConfFixture creates a fixture with sample files copied to a temporary directory.
func NewConfFixture(t *testing.T) (f *ConfFixture) {
	f = &ConfFixture{testutils.NewDataFileFixture(t)}
	f.addSampleFiles()
	return
}

func (f *ConfFixture) addSampleFiles() {
	f.CopyDataFilesToTempDir(
		"sample.json",
		"sample.schema.json",
		"sample-comments.json",
		"sample-badsyntax.json",
		"sample-invalid.json",
		"noconfig.schema.json",
		"sample-to-use-after-new-subconf.json",
		"sample_devtypes/msu21.conf",
		"sample_devtypes/wb-mrm2.conf")
}
