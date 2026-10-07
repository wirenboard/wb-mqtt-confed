package confed

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/wirenboard/wbgong"
	"github.com/xeipuuv/gojsonpointer"
)

type enumLoader struct {
	sync.Mutex
	root       string
	dirty      bool
	watchers   map[string]wbgong.DirWatcher
	enumValues map[string]map[string]string
}

func newEnumLoader(root string) *enumLoader {
	return &enumLoader{
		root:       root,
		dirty:      true,
		watchers:   make(map[string]wbgong.DirWatcher),
		enumValues: make(map[string]map[string]string),
	}
}

type subconfWatcherClient struct {
	e   *enumLoader
	key string
	ptr gojsonpointer.JsonPointer
}

func (c *subconfWatcherClient) LoadFile(path string) error {
	return c.e.loadSubconf(c.key, path, c.ptr)
}

func (c *subconfWatcherClient) LiveLoadFile(path string) error {
	return c.e.liveLoadSubconf(c.key, path, c.ptr)
}

func (c *subconfWatcherClient) LiveRemoveFile(path string) error {
	c.e.removeSubconf(c.key, path)
	return nil
}

func (e *enumLoader) loadSubconf(key, path string, ptr gojsonpointer.JsonPointer) error {
	wbgong.Debug.Printf("enumLoader.loadSubconf(): %s, %s", key, path)
	bs, err := loadConfigBytes(path, nil)
	if err != nil {
		wbgong.Debug.Printf("enumLoader.loadSubconf(): %s load failed: %s", path, err)
		return err
	}

	var parsed map[string]any
	if err = json.Unmarshal(bs.content, &parsed); err != nil {
		wbgong.Debug.Printf("enumLoader.loadSubconf(): %s unmarshal failed: %s", path, err)
		return fmt.Errorf("failed to parse %s: %w", path, err)
	}

	node, kind, err := ptr.Get(parsed)
	if err != nil {
		wbgong.Debug.Printf("enumLoader.loadSubconf(): %s JSON pointer deref failed: %s", path, err)
		return fmt.Errorf("failed to dereference JSON pointer in %s: %w", path, err)
	}
	if kind != reflect.String {
		wbgong.Debug.Printf("enumLoader.loadSubconf(): %s: JSON Pointer enum target is not a string", path)
		return errors.New("JSON Pointer enum target is not a string")
	}

	vals := e.enumValues[key]
	if vals == nil {
		vals = make(map[string]string)
		e.enumValues[key] = vals
	}
	vals[path] = node.(string)
	return nil
}

func (e *enumLoader) liveLoadSubconf(key, path string, ptr gojsonpointer.JsonPointer) error {
	e.Lock()
	defer e.Unlock()
	e.dirty = true
	return e.loadSubconf(key, path, ptr)
}

func (e *enumLoader) removeSubconf(key, path string) {
	e.Lock()
	defer e.Unlock()

	vals := e.enumValues[key]
	if vals == nil {
		return
	}

	_, found := vals[path]
	if found {
		delete(vals, path)
		e.dirty = true
	}
}

func (e *enumLoader) ensureSubconfDirLoaded(path, pattern, ptrString string) (err error) {
	ptr, err := gojsonpointer.NewJsonPointer(ptrString)
	if err != nil {
		return
	}

	key := subconfKey(ptrString, path, pattern)
	if e.watchers[key] != nil {
		return
	}
	client := &subconfWatcherClient{e: e, key: key, ptr: ptr}
	watcher := wbgong.NewDirWatcher(pattern, client)
	e.watchers[key] = watcher
	if loadErr := watcher.Load(path); loadErr != nil {
		wbgong.Debug.Printf("enumLoader.ensureSubconfDirLoaded(): failed to load %s: %s", path, loadErr)
	}
	return
}

var errInvalidEnumSubconf = errors.New("invalid enum subconf node")

func (e *enumLoader) subconfEnumValues(node map[string]any) (r []any, err error) {
	maybePaths, ok := node["directories"].([]any)
	if !ok || len(maybePaths) == 0 {
		return nil, errInvalidEnumSubconf
	}
	paths := make([]string, len(maybePaths))
	for n, p := range maybePaths {
		path, isString := p.(string)
		if !isString {
			return nil, errInvalidEnumSubconf
		}
		paths[n], _, err = fakeRootPath(e.root, path)
		if err != nil {
			wbgong.Warn.Printf("pathFromRoot failed for %s", path)
			paths[n] = path
		}
		wbgong.Debug.Printf("pathFromRoot: %s, %s -> %s", e.root, path, paths[n])
	}

	ptrString, ok := node["pointer"].(string)
	if !ok {
		return nil, errInvalidEnumSubconf
	}

	pattern, ok := node["pattern"].(string)
	if !ok {
		pattern = defaultSubconfPattern
	}

	seen := make(map[string]bool)
	strs := make([]string, 0, 32)
	for _, path := range paths {
		wbgong.Debug.Printf("enumLoader.subconfEnumValues(): loading subconf path %s", path)
		curErr := e.ensureSubconfDirLoaded(path, pattern, ptrString)
		if curErr != nil {
			wbgong.Debug.Printf("enumLoader.subconfEnumValues(): subconf load error: %s", curErr)
		}
		if err == nil {
			err = curErr
		}
		key := subconfKey(ptrString, path, pattern)
		if vals := e.enumValues[key]; vals != nil {
			for _, v := range vals {
				if !seen[v] {
					strs = append(strs, v)
					seen[v] = true
				}
			}
		}
	}

	sort.Strings(strs)
	r = make([]any, len(strs))
	for n, v := range strs {
		r[n] = v
	}
	wbgong.Debug.Printf("enumLoader.subconfEnumValues(): values=%v", r)
	return r, err
}

func (e *enumLoader) preprocess(v any) any {
	switch v := v.(type) {
	case map[string]any:
		r := make(map[string]any)
		for k, item := range v {
			if k != "enum" {
				r[k] = e.preprocess(item)
				continue
			}
			msi, ok := item.(map[string]any)
			if !ok {
				r[k] = e.preprocess(item)
				continue
			}
			_, found := msi["directories"]
			if !found {
				r[k] = e.preprocess(item)
				continue
			}
			vals, err := e.subconfEnumValues(msi)
			if err != nil {
				wbgong.Error.Printf(
					"failed to load subconf values for %v: %s",
					vals, err)
				r[k] = []any{}
				continue
			}
			r[k] = vals
		}
		return r
	case []any:
		r := make([]any, len(v))
		for n, item := range v {
			r[n] = e.preprocess(item)
		}
		return r
	default:
		return v
	}
}

func (e *enumLoader) Preprocess(v any) (r any) {
	e.Lock()
	defer e.Unlock()

	r = e.preprocess(v)
	// all necessary subconfs are loaded at this point
	e.dirty = false
	return
}

func (e *enumLoader) IsDirty() (dirty bool) {
	e.Lock()
	defer e.Unlock()
	return e.dirty
}

func (e *enumLoader) StopWatchingSubconfigs() {
	e.Lock()
	defer e.Unlock()
	for _, watcher := range e.watchers {
		watcher.Stop()
	}
}
