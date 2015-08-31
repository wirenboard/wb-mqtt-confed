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
	reader := JsonConfigReader.New(in)
	return ioutil.ReadAll(reader)
}
