package confed

import (
	"encoding/json"
	"errors"
	"github.com/contactless/wbgo"
	"github.com/xeipuuv/gojsonpointer"
	"reflect"
	"sort"
	"sync"
)

type deviceDefinition struct {
	deviceType string
	setupSchema map[string]interface{}
	channels []interface{}
}

type byDeviceType []*deviceDefinition

func (a byDeviceType) Len() int           { return len(a) }
func (a byDeviceType) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a byDeviceType) Less(i, j int) bool { return a[i].deviceType < a[j].deviceType }

type enumLoader struct {
	sync.Mutex
	root                   string
	dirty                  bool
	watchers               map[string]*wbgo.DirWatcher
	enumValues             map[string]map[string]string
	deviceDefinitionValues map[string]map[string]*deviceDefinition
}

func newEnumLoader(root string) *enumLoader {
	return &enumLoader{
		root:                   root,
		dirty:                  true,
		watchers:               make(map[string]*wbgo.DirWatcher),
		enumValues:             make(map[string]map[string]string),
		deviceDefinitionValues: make(map[string]map[string]*deviceDefinition),
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
	wbgo.Debug.Printf("enumLoader.loadSubconf(): %s, %s", key, path)
	content, err := loadConfigBytes(path, nil)
	if err != nil {
		wbgo.Debug.Printf("enumLoader.loadSubconf(): %s load failed: %s", path, err)
		return
	}

	var parsed map[string]interface{}
	if err = json.Unmarshal(content, &parsed); err != nil {
		wbgo.Debug.Printf("enumLoader.loadSubconf(): %s unmarshal failed: %s", path, err)
		return
	}

	node, kind, err := ptr.Get(parsed)
	if err != nil {
		wbgo.Debug.Printf("enumLoader.loadSubconf(): %s JSON pointer deref failed: %s", path, err)
		return
	}
	if kind != reflect.String {
		wbgo.Debug.Printf("enumLoader.loadSubconf(): %s: JSON Pointer enum target is not a string", path)
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
	watcher := wbgo.NewDirWatcher(pattern, client)
	e.watchers[key] = watcher
	watcher.Load(path)
	return
}

func (e *enumLoader) getPaths(maybePaths []interface{}) (paths []string) {
	paths = make([]string, len(maybePaths))
	for n, p := range maybePaths {
		path, ok := p.(string)
		if !ok {
			wbgo.Warn.Printf("deviceDefinitions invalid object in directories array")
		} else {
			var err error
			paths[n], _, err = fakeRootPath(e.root, path)
			if err != nil {
				wbgo.Warn.Printf("pathFromRoot failed for %s", path)
				paths[n] = path
			}
			wbgo.Debug.Printf("pathFromRoot: %s, %s -> %s", e.root, path, paths[n])
		}
	}
	return
}

var invalidEnumSubconfError = errors.New("invalid enum subconf node")

func (e *enumLoader) subconfEnumValues(node map[string]interface{}) (r []interface{}, err error) {
	maybePaths, ok := node["directories"].([]interface{})
	if !ok || len(maybePaths) == 0 {
		return nil, invalidEnumSubconfError
	}
	paths := e.getPaths(maybePaths)

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
		wbgo.Debug.Printf("enumLoader.subconfEnumValues(): loading subconf path %s", path)
		curErr := e.ensureSubconfDirLoaded(path, pattern, ptrString)
		if curErr != nil {
			wbgo.Debug.Printf("enumLoader.subconfEnumValues(): subconf load error: %s", curErr)
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
	wbgo.Debug.Printf("enumLoader.subconfEnumValues(): values=%v", r)
	return
}

type deviceDefinitionPointers struct {
	deviceType  gojsonpointer.JsonPointer
	setupSchema gojsonpointer.JsonPointer
	channels    gojsonpointer.JsonPointer
}

type deviceDefinitionsWatcherClient struct {
	e   *enumLoader
	key string
	ptr deviceDefinitionPointers
}

func (c *deviceDefinitionsWatcherClient) LoadFile(path string) error {
	return c.e.loadDeviceDefinitions(c.key, path, c.ptr)
}

func (c *deviceDefinitionsWatcherClient) LiveLoadFile(path string) error {
	return c.e.liveLoadDeviceDefinitions(c.key, path, c.ptr)
}

func (c *deviceDefinitionsWatcherClient) LiveRemoveFile(path string) error {
	c.e.removeDeviceDefinitions(c.key, path)
	return nil
}

func (e *enumLoader) loadDeviceDefinitions(key, path string, ptr deviceDefinitionPointers) (err error) {
	content, err := loadConfigBytes(path, nil)
	if err != nil {
		wbgo.Debug.Printf("enumLoader.loadDeviceDefinitions(): %s load failed: %s", path, err)
		return
	}

	var parsed map[string]interface{}
	if err = json.Unmarshal(content, &parsed); err != nil {
		wbgo.Debug.Printf("enumLoader.loadDeviceDefinitions(): %s unmarshal failed: %s", path, err)
		return
	}

	deviceTypeNode, _, err := ptr.deviceType.Get(parsed)
	if err != nil {
		wbgo.Debug.Printf("enumLoader.loadDeviceDefinitions(): %s device type JSON pointer deref failed: %s", path, err)
		return
	}
	deviceType, ok := deviceTypeNode.(string)
	if !ok {
		return errors.New(path + " device type JSON Pointer target is not a string")
	}

	var setupSchema map[string]interface{} = nil
	setupSchemaNode, _, err := ptr.setupSchema.Get(parsed)
	if err == nil {
		setupSchema, ok = setupSchemaNode.(map[string]interface{})
		if !ok {
			return errors.New(path + " setup schema JSON Pointer target is not an object")
		}
	} else {
		err = nil // the template hasn't setup_schema field, it is ok, continue
	}

	channelsNode, _, err := ptr.channels.Get(parsed)
	if err != nil {
		wbgo.Debug.Printf("enumLoader.loadDeviceDefinitions(): %s channels JSON pointer deref failed: %s", path, err)
		return
	}
	channels, ok := channelsNode.([]interface{})
	if !ok {
		return errors.New(path + " channels JSON Pointer target is not an array")
	}

	vals := e.deviceDefinitionValues[key]
	if vals == nil {
		vals = make(map[string]*deviceDefinition)
		e.deviceDefinitionValues[key] = vals
	}
	vals[path] = &deviceDefinition{deviceType, setupSchema, channels}
	return
}

func (e *enumLoader) liveLoadDeviceDefinitions(key, path string, ptr deviceDefinitionPointers) error {
	e.Lock()
	defer e.Unlock()
	e.dirty = true
	return e.loadDeviceDefinitions(key, path, ptr)
}

func (e *enumLoader) removeDeviceDefinitions(key, path string) {
	e.Lock()
	defer e.Unlock()

	vals := e.deviceDefinitionValues[key]
	if vals == nil {
		return
	}

	_, found := vals[path]
	if found {
		delete(vals, path)
		e.dirty = true
	}
}

func (e *enumLoader) ensureDeviceDefinitionsDirLoaded(path,
													  pattern,
													  deviceTypePtrString  string,
													  setupSchemaPtrString string,
													  channelsPtrString    string) (err error) {
	var ptr deviceDefinitionPointers
	ptr.deviceType, err = gojsonpointer.NewJsonPointer(deviceTypePtrString)
	if err != nil {
		return
	}

	ptr.setupSchema, err = gojsonpointer.NewJsonPointer(setupSchemaPtrString)
	if err != nil {
		return
	}

	ptr.channels, err = gojsonpointer.NewJsonPointer(channelsPtrString)
	if err != nil {
		return
	}

	key := subconfKey(setupSchemaPtrString, path, pattern)
	if e.watchers[key] != nil {
		return
	}
	client := &deviceDefinitionsWatcherClient{e, key, ptr}
	watcher := wbgo.NewDirWatcher(pattern, client)
	e.watchers[key] = watcher
	watcher.Load(path)
	return
}

//	{ "$_devicesDefinitions": {
//			"directories": ["/usr/share/wb-mqtt-serial/templates"],
//			"pointer": [ "/device_type", "/setup_schema", "/device/channels"]
//			"pattern": "^.*\\.json$"
//		}
//	}
func (e *enumLoader) deviceDefinitions(node map[string]interface{}) (r []*deviceDefinition, err error) {
	maybePaths, ok := node["directories"].([]interface{})
	if !ok || len(maybePaths) == 0 {
		return nil, errors.New("enumLoader.deviceDefinitions(): directories field is not an array")
	}

	ptrArray, ok := node["pointer"].([]interface{})
	if !ok || (len(ptrArray) < 3) {
		return nil, errors.New("enumLoader.deviceDefinitions(): pointer field is not an array")
	}

	deviceTypePtrString, ok := ptrArray[0].(string)
	if !ok {
		return nil, errors.New("enumLoader.deviceDefinitions(): pointers first element is not a string")
	}

	setupSchemaPtrString, ok := ptrArray[1].(string)
	if !ok {
		return nil, errors.New("enumLoader.deviceDefinitions(): pointers second element is not a string")
	}

	channelsPtrString, ok := ptrArray[2].(string)
	if !ok {
		return nil, errors.New("enumLoader.deviceDefinitions(): pointers third element is not a string")
	}

	pattern, ok := node["pattern"].(string)
	if !ok {
		pattern = DEFAULT_SUBCONF_PATTERN
	}

	paths := e.getPaths(maybePaths)

	seen := make(map[string]bool)
	r = make([]*deviceDefinition, 0, 100)

	for _, path := range paths {
		wbgo.Debug.Printf("enumLoader.deviceDefinitions(): loading path %s", path)
		curErr := e.ensureDeviceDefinitionsDirLoaded(path, pattern, deviceTypePtrString, setupSchemaPtrString, channelsPtrString)
		if curErr != nil {
			wbgo.Debug.Printf("enumLoader.deviceDefinitions(): path load error: %s", curErr)
		}
		if err == nil {
			err = curErr
		}
		key := subconfKey(setupSchemaPtrString, path, pattern)

		if vals := e.deviceDefinitionValues[key]; vals != nil {
			for _, v := range vals {
				if !seen[v.deviceType] {
					r = append(r, v)
					seen[v.deviceType] = true
				} else {
					wbgo.Info.Printf("enumLoader.deviceDefinitions(): Device type %s is already defined", v.deviceType)
				}
			}
		}
	}

	sort.Sort(byDeviceType(r))

	return
}

//	{
//		"type": "object"
//		"title": deviceType
//		"properties": {
//			"device_type": {
//				"type": "string",
//				"enum": [ DEVICE_TYPE ],
//				"options": {
//					"hidden": true
//				},
//				"propertyOrder": 5
//			},
//			"setup": setupSchema,
//			"channels": {
//				"type": "array",
//				"_format": "table",
//				"title": "List of standard channels",
//				"minItems": CHANNELS_COUNT,
//				"description": "Lists device registers and their corresponding controls",
//				"items": { "$ref": "#/definitions/channelSettings" },
//				"default": [
//					{
//						"hidden_name": CHANNEL_NAME,
//						"poll_interval": POLL_INTERVAL
//					},
//					....
//				],
//				"options": {
//					"disable_array_delete": true,
//					"disable_array_reorder": true,
//					"disable_array_add": true
//				},
//				"propertyOrder": 9
//			},
//			"custom_channels": {
//				"type": "array",
//				"title": "List of custom channels",
//				"description": "Lists nonstandard device registers and their corresponding controls",
//				"items": { "$ref": "#/definitions/channel" },
//				"propertyOrder": 10
//			}
//		},
//		"required": ["device_type"]
//	}
func (e *enumLoader) makeDeviceDefinitionProperties(deviceType  string,
													setupSchema map[string]interface{},
													channels    []interface{}) map[string]interface{} {
	r := map[string]interface{} {
			"device_type": map[string]interface{} {
				"type": "string",
				"options": map[string]interface{} { "hidden": true },
				"enum": []interface{} { deviceType },
				"propertyOrder": 5,
			},
			"channels": map[string]interface{} {
				"type": "array",
				"title": "List of standard channels",
				"_format": "table",
				"minItems": len(channels),
				"description": "Lists device registers and their corresponding controls",
				"items": map[string]interface{} { "$ref": "#/definitions/channelSettings" },
				"default": e.makeDefaultChannels(channels),
				"options": map[string]interface{} {
					"disable_array_delete": true,
					"disable_array_reorder": true,
					"disable_array_add": true,
				},
				"propertyOrder": 9,
			},
			"custom_channels": map[string]interface{} {
				"type": "array",
				"title": "List of custom channels",
				"description": "Lists nonstandard device registers and their corresponding controls",
				"items": map[string]interface{} { "$ref": "#/definitions/channel" },
				"propertyOrder": 10,
			},
		}

	if setupSchema != nil {
		r["setup"] = setupSchema
	}
	return r
}

func (e *enumLoader) makeDefaultChannels(channels []interface{}) []interface{} {
	r := make([]interface{}, 0, len(channels))
	for _, v := range channels {
		t, ok := v.(map[string]interface{})
		if ok {
			ch := map[string]interface{} {
					"hidden_name": t["name"],
				}
			pi, exists := t["poll_interval"]
			if exists {
				ch["poll_interval"] = pi
			}
			r = append(r, ch)
		}
	}
	return r
}

func (e *enumLoader) makeDeviceDefinition(deviceType string, setupSchema map[string]interface{}, channels []interface{}) map[string]interface{} {
	return map[string]interface{} {
				"type": "object",
				"title": deviceType,
				"properties": e.makeDeviceDefinitionProperties(deviceType, setupSchema, channels),
				"required": []interface{} { "device_type" },
			}
}

func (e *enumLoader) tryToLoadDeviceDefinitions(item interface{}) ([]interface{}, bool) {
	msi, ok := item.(map[string]interface{})
	if !ok {
		return nil, false
	}

	defsNode, exists := msi["$_devicesDefinitions"]
	if !exists {
		return nil, false
	}

	r := make([]interface{}, 0, 100)
	defs, ok := defsNode.(map[string]interface{})
	if !ok {
		wbgo.Error.Printf( "$_devicesDefinitions is not an object")
		return r, true // it is a deviceDefinitions node, so don't copy it to resulting JSON
	}

	vals, err := e.deviceDefinitions(defs)
	if err != nil {
		wbgo.Error.Printf( "failed to load device definitions %v: %s", vals, err)
		return r, true // it is a deviceDefinitions node, but with some external problems, so don't copy it to resulting JSON
	}

	for _, v := range vals {
		r = append(r, e.makeDeviceDefinition(v.deviceType, v.setupSchema, v.channels))
	}

	return r, true
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
				wbgo.Error.Printf(
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
		r := make([]interface{}, 0, len(l))
		for _, item := range l {
			vals, ok := e.tryToLoadDeviceDefinitions(item)
			if ok {
				r = append(r, vals...)
			} else {
				r = append(r, e.preprocess(item))
			}
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
