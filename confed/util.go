package confed

import (
	"github.com/DisposaBoy/JsonConfigReader"
	"io/ioutil"
	"os"
	"path/filepath"
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

func pathFromRoot(root, path string) (r string, err error) {
	if len(root) == 0 || root[:len(root)-1] != "/" {
		root = root + "/"
	}
	path, err = filepath.Abs(path)
	if err == nil {
		r, err = filepath.Rel(root, path)
		if err == nil {
			r = "/" + r
		}
	}
	return
}

func fakeRootPath(root, path string) (physicalPath, virtualPath string, err error) {
	for path[:1] == "/" {
		path = path[1:]
	}
	physicalPath = filepath.Join(root, path)
	virtualPath, err = pathFromRoot(root, physicalPath)
	return
}
