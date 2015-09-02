package confed

import (
	"github.com/DisposaBoy/JsonConfigReader"
	"io/ioutil"
	"os"
)

func loadConfigBytes(path string) (bs []byte, err error) {
	in, err := os.Open(path)
	if err != nil {
		return
	}
	defer in.Close() // not writing the file, so we can ignore Close() errors here
	reader := JsonConfigReader.New(in)
	return ioutil.ReadAll(reader)
}
