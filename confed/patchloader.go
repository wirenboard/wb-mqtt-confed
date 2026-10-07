package confed

import (
	"fmt"
	"io"
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
	baseSchemaPath   string
	dirty            bool
	watcher          wbgong.DirWatcher
	sortedPatchPaths []string
}

func newPatchLoader(baseSchemaPath string) *patchLoader {
	return &patchLoader{
		baseSchemaPath:   baseSchemaPath,
		dirty:            true,
		watcher:          nil,
		sortedPatchPaths: []string{},
	}
}

type patchWatcherClient struct {
	pl *patchLoader
}

func (c *patchWatcherClient) LoadFile(patchPath string) error {
	c.pl.patchIsChanged(patchPath)
	return nil
}

func (c *patchWatcherClient) LiveLoadFile(patchPath string) error {
	c.pl.patchIsChanged(patchPath)
	return nil
}

func (c *patchWatcherClient) LiveRemoveFile(patchPath string) error {
	c.pl.removePatch(patchPath)
	return nil
}

func (pl *patchLoader) patchIsChanged(patchPath string) {
	wbgong.Debug.Printf("patchLoader.patchIsChanged: %s", patchPath)
	pl.Lock()
	defer pl.Unlock()
	pl.dirty = true
	index := sort.SearchStrings(pl.sortedPatchPaths, patchPath)
	if index == len(pl.sortedPatchPaths) {
		pl.sortedPatchPaths = append(pl.sortedPatchPaths, patchPath)
	} else if pl.sortedPatchPaths[index] != patchPath {
		pl.sortedPatchPaths = append(pl.sortedPatchPaths, "")
		copy(pl.sortedPatchPaths[index+1:], pl.sortedPatchPaths[index:])
		pl.sortedPatchPaths[index] = patchPath
	}
}

func (pl *patchLoader) removePatch(patchPath string) {
	wbgong.Debug.Printf("patchLoader.removePatch: %s", patchPath)
	pl.Lock()
	defer pl.Unlock()
	index := sort.SearchStrings(pl.sortedPatchPaths, patchPath)
	if index != len(pl.sortedPatchPaths) && pl.sortedPatchPaths[index] == patchPath {
		pl.dirty = true
		pl.sortedPatchPaths = append(pl.sortedPatchPaths[:index], pl.sortedPatchPaths[index+1:]...)
	}
}

func (pl *patchLoader) Patch(schema []byte) []byte {
	if pl.watcher == nil {
		pattern := regexp.QuoteMeta(path.Base(pl.baseSchemaPath) + ".patch")
		client := &patchWatcherClient{pl: pl}
		pl.watcher = wbgong.NewDirWatcher(pattern, client)
		if err := pl.watcher.Load(path.Dir(pl.baseSchemaPath)); err != nil {
			wbgong.Warn.Printf("Failed to load patches for %s: %s", pl.baseSchemaPath, err)
		}
	}
	pl.Lock()
	pl.dirty = false
	patchPaths := make([]string, len(pl.sortedPatchPaths))
	copy(patchPaths, pl.sortedPatchPaths)
	pl.Unlock()
	for _, patchPath := range patchPaths {
		patch, err := readPatch(patchPath)
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

func readPatch(patchPath string) ([]byte, error) {
	in, err := os.Open(patchPath) //nolint:gosec // patch paths come from the schema directory watcher
	if err != nil {
		return nil, fmt.Errorf("failed to open: %w", err)
	}
	defer in.Close() // not writing the file, so we can ignore Close() errors here

	patch, err := io.ReadAll(JsonConfigReader.New(in))
	if err != nil {
		return nil, fmt.Errorf("failed to read: %w", err)
	}
	return patch, nil
}

func (pl *patchLoader) IsDirty() (dirty bool) {
	pl.Lock()
	defer pl.Unlock()
	return pl.dirty
}

func (pl *patchLoader) StopWatchingPatches() {
	pl.Lock()
	defer pl.Unlock()
	if pl.watcher != nil {
		pl.watcher.Stop()
	}
}
