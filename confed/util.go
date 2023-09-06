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

type RunCommandResult struct {
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func runCommand(captureStdout bool, stdin io.Reader, command string, args ...string) (res RunCommandResult, err error) {
	cmd := exec.Command(command, args...)
	cmd.Stdin = stdin
	if captureStdout {
		cmd.Stdout = &res.stdout
	}
	cmd.Stderr = &res.stderr
	if err := cmd.Run(); err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			status := -1 // FIXME
			ws, ok := exitErr.Sys().(syscall.WaitStatus)
			if ok {
				status = ws.ExitStatus()
			}
			err = fmt.Errorf("exit status %d from %s %s: %s",
				status, command, strings.Join(args, " "),
				string(res.stderr.Bytes()))
		}
	}

	return
}

func extPreprocess(commandAndArgs []string, in []byte) (RunCommandResult, error) {
	if len(commandAndArgs) < 1 {
		return RunCommandResult{}, errors.New("commandAndArgs must not be empty")
	}

	return runCommand(true, bytes.NewBuffer(in), commandAndArgs[0], commandAndArgs[1:]...)
}

type LoadConfigResult struct {
	content []byte
	preprocessorErrors string
}

func loadConfigBytes(path string, preprocessCmd []string) (res LoadConfigResult, err error) {
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
		var output RunCommandResult
		output, err = extPreprocess(preprocessCmd, tmpBs)
		if err != nil {
			return
		}
		jsonInput = &output.stdout
		if output.stderr.Len() != 0 {
			res.preprocessorErrors = output.stderr.String()
		}
	}

	reader := JsonConfigReader.New(jsonInput)
	res.content, err = ioutil.ReadAll(reader)
	return
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
