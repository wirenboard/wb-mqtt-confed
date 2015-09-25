package confed

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/DisposaBoy/JsonConfigReader"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

func extPreprocess(commandAndArgs []string, in []byte) (*bytes.Buffer, error) {
	if len(commandAndArgs) < 1 {
		return nil, errors.New("commandAndArgs must not be empty")
	}

	cmd := exec.Command(commandAndArgs[0], commandAndArgs[1:]...)
	cmd.Stdin = bytes.NewBuffer(in)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			status := -1 // FIXME
			ws, ok := exitErr.Sys().(syscall.WaitStatus)
			if ok {
				status = ws.ExitStatus()
			}
			return nil, fmt.Errorf("exit status %d from %s: %s",
				status, strings.Join(commandAndArgs, " "),
				string(stderr.Bytes()))
		}
		return nil, err
	}

	return &stdout, nil
}

func loadConfigBytes(path string, preprocessCmd []string) (bs []byte, err error) {
	in, err := os.Open(path)
	if err != nil {
		return
	}
	defer in.Close() // not writing the file, so we can ignore Close() errors here

	var jsonInput io.Reader = in
	if preprocessCmd != nil {
		var tmpBs []byte
		tmpBs, err = ioutil.ReadAll(in)
		if err != nil {
			return
		}
		jsonInput, err = extPreprocess(preprocessCmd, tmpBs)
		if err != nil {
			return
		}
	}

	reader := JsonConfigReader.New(jsonInput)
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
