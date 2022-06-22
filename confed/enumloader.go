package confed

import (
	"encoding/json"
	"errors"
	"github.com/contactless/wbgong"
	"github.com/xeipuuv/gojsonpointer"
	"reflect"
	"sort"
	"sync"
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

func (e *enumLoader) loadSubconf(key, path string, ptr gojsonpointer.JsonPointer) (err error) {
	wbgong.Debug.Printf("enumLoader.loadSubconf(): %s, %s", key, path)
	content, err := loadConfigBytes(path, nil)
	if err != nil {
		wbgong.Debug.Printf("enumLoader.loadSubconf(): %s load failed: %s", path, err)
		return
	}

	var parsed map[string]interface{}
	if err = json.Unmarshal(content, &parsed); err != nil {
		wbgong.Debug.Printf("enumLoader.loadSubconf(): %s unmarshal failed: %s", path, err)
		return
	}

	node, kind, err := ptr.Get(parsed)
	if err != nil {
		wbgong.Debug.Printf("enumLoader.loadSubconf(): %s JSON pointer deref failed: %s", path, err)
		return
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
	return
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
	watcher.Load(path)
	return
}

var invalidEnumSubconfError = errors.New("invalid enum subconf node")

func (e *enumLoader) subconfEnumValues(node map[string]interface{}) (r []interface{}, err error) {
	maybePaths, ok := node["directories"].([]interface{})
	if !ok || len(maybePaths) == 0 {
		return nil, invalidEnumSubconfError
	}
	paths := make([]string, len(maybePaths))
	for n, p := range maybePaths {
		path, ok := p.(string)
		if !ok {
			return nil, invalidEnumSubconfError
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
		return nil, invalidEnumSubconfError
	}

	pattern, ok := node["pattern"].(string)
	if !ok {
		pattern = DEFAULT_SUBCONF_PATTERN
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
	r = make([]interface{}, len(strs))
	for n, v := range strs {
		r[n] = v
	}
	wbgong.Debug.Printf("enumLoader.subconfEnumValues(): values=%v", r)
	return
}

func (e *enumLoader) preprocess(v interface{}) interface{} {
	switch v.(type) {
	case map[string]interface{}:
		m := v.(map[string]interface{})
		r := make(map[string]interface{})
		for k, item := range m {
			if k != "enum" {
				r[k] = e.preprocess(item)
				continue
			}
			msi, ok := item.(map[string]interface{})
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
				r[k] = []interface{}{}
				continue
			}
			r[k] = vals
		}
		return r
	case []interface{}:
		l := v.([]interface{})
		r := make([]interface{}, len(l))
		for n, item := range l {
			r[n] = e.preprocess(item)
		}
		return r
	default:
		return v
	}
}

func (e *enumLoader) Preprocess(v interface{}) (r interface{}) {
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
