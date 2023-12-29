package confed

import (
	"io/ioutil"
	"os"
	"path"
	"regexp"
	"sort"
	"sync"

	"github.com/DisposaBoy/JsonConfigReader"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/wirenboard/wbgong"
)

type patchLoader struct {
	sync.Mutex
	baseSchemaPath string
	dirty          bool
	watcher        wbgong.DirWatcher
	patchPaths     []string
}

func newPatchLoader(baseSchemaPath string) *patchLoader {
	return &patchLoader{
		baseSchemaPath: baseSchemaPath,
		dirty:          true,
		watcher:        nil,
		patchPaths:     []string{},
	}
}

type patchWatcherClient struct {
	pl *patchLoader
}

func (c *patchWatcherClient) LoadFile(path string) error {
	c.pl.patchIsChanged(path)
	return nil
}

func (c *patchWatcherClient) LiveLoadFile(path string) error {
	c.pl.patchIsChanged(path)
	return nil
}

func (c *patchWatcherClient) LiveRemoveFile(path string) error {
	c.pl.removePatch(path)
	return nil
}

func (pl *patchLoader) patchIsChanged(path string) {
	wbgong.Debug.Printf("patchLoader.patchIsChanged: %s", path)
	pl.Lock()
	defer pl.Unlock()
	pl.dirty = true
	index := sort.SearchStrings(pl.patchPaths, path)
	if index == len(pl.patchPaths) {
		pl.patchPaths = append(pl.patchPaths, path)
	} else {
		if pl.patchPaths[index] != path {
			pl.patchPaths = append(pl.patchPaths, "")
			copy(pl.patchPaths[index+1:], pl.patchPaths[index:])
			pl.patchPaths[index] = path
		}
	}
}

func (pl *patchLoader) removePatch(path string) {
	wbgong.Debug.Printf("patchLoader.removePatch: %s", path)
	pl.Lock()
	defer pl.Unlock()
	index := sort.SearchStrings(pl.patchPaths, path)
	if index != len(pl.patchPaths) && pl.patchPaths[index] == path {
		pl.dirty = true
		pl.patchPaths = append(pl.patchPaths[:index], pl.patchPaths[index+1:]...)
	}
}

func (pl *patchLoader) Patch(schema []byte) []byte {
	if pl.watcher == nil {
		pattern := regexp.QuoteMeta(path.Base(pl.baseSchemaPath) + ".patch")
		client := &patchWatcherClient{pl: pl}
		pl.watcher = wbgong.NewDirWatcher(pattern, client)
		pl.watcher.Load(path.Dir(pl.baseSchemaPath))
	}
	pl.Lock()
	pl.dirty = false
	patchPaths := make([]string, len(pl.patchPaths))
	copy(patchPaths, pl.patchPaths)
	pl.Unlock()
	for _, patchPath := range patchPaths {
		in, err := os.Open(patchPath)
		if err != nil {
			wbgong.Warn.Printf("Failed to open patch file %s: %s", patchPath, err)
			continue
		}
		defer in.Close() // not writing the file, so we can ignore Close() errors here

		reader := JsonConfigReader.New(in)
		var patch []byte
		patch, err = ioutil.ReadAll(reader)
		if err != nil {
			wbgong.Warn.Printf("Failed to read patch file %s: %s", patchPath, err)
			continue
		}
		schema, err = jsonpatch.MergePatch(schema, patch)
		if err != nil {
			wbgong.Warn.Printf("Failed to apply patch file %s: %s", patchPath, err)
		}
	}
	return schema
}

func (pl *patchLoader) IsDirty() (dirty bool) {
	pl.Lock()
	defer pl.Unlock()
	return pl.dirty
}

func (pl *patchLoader) StopWatchingPatches() {
	pl.Lock()
	defer pl.Unlock()
	pl.watcher.Stop()
}
