package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/wirenboard/wb-mqtt-confed/confed"
	"github.com/wirenboard/wbgong"
)

const (
	DRIVER_CLIENT_ID    = "confed"
	MOSQUITTO_SOCK_FILE = "/var/run/mosquitto/mosquitto.sock"
	DEFAULT_BROKER_URL  = "tcp://localhost:1883"
	WBGO_FILE           = "/usr/lib/wb-mqtt-confed/wbgo.so"
)

func isSocket(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.Mode()&os.ModeSocket != 0
}

var version = "unknown"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		os.Exit(0)
	}

	brokerAddress := flag.String("broker", DEFAULT_BROKER_URL, "MQTT broker url")
	root := flag.String("root", "/", "Config root path")
	debug := flag.Bool("debug", false, "Enable debugging")
	useSyslog := flag.Bool("syslog", false, "Use syslog for logging")
	validate := flag.Bool("validate", false, "Validate specified config file and exit")
	dump := flag.Bool("dump", false, "Dump preprocessed schema and exit")
	wbgoso := flag.String("wbgo", WBGO_FILE, "Location to wbgo.so file")
	profile := flag.String("profile", "", "Run pprof server")
	flag.Parse()

	if *profile != "" {
		go func() {
			log.Println(http.ListenAndServe(*profile, nil))
		}()
	}

	errInit := wbgong.Init(*wbgoso)
	if errInit != nil {
		log.Fatalf("ERROR: wbgo.so init failed: '%s'", errInit)
	}
	if flag.NArg() < 1 {
		wbgong.Error.Fatal("must specify schema(s) / schema directory(ies)")
	}
	if *useSyslog {
		wbgong.UseSyslog()
	}
	if *debug {
		wbgong.SetDebuggingEnabled(true)
		wbgong.EnableMQTTDebugLog(*useSyslog)
	}
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		wbgong.Error.Fatal("failed to get absolute path for root")
	}

	// TBD: don't watch subconfs while validating/dumping
	if *validate {
		if flag.NArg() != 2 {
			// TBD: don't require config path, it should
			// be taken from schema if it's not specified
			wbgong.Error.Fatal("must specify schema and config files")
		}
		schemaPath, configPath := flag.Arg(0), flag.Arg(1)
		schema, err := confed.NewJSONSchemaWithRoot(schemaPath, absRoot)
		if err != nil {
			wbgong.Error.Fatalf("failed to load schema %s: %s", schemaPath, err)
		}
		r, err := schema.ValidateFile(configPath)
		if err != nil {
			wbgong.Error.Fatalf("failed to validate %s: %s", configPath, err)
		}
		if !r.Valid() {
			wbgong.Error.Printf("Validation failed for %s", configPath)
			for _, desc := range r.Errors() {
				wbgong.Error.Printf("- %s\n", desc)
			}
			os.Exit(1)
		}
		os.Exit(0)
	}
	if *dump {
		if flag.NArg() != 1 {
			wbgong.Error.Fatal("must specify schema file")
		}
		schemaPath := flag.Arg(0)
		schema, err := confed.NewJSONSchemaWithRoot(schemaPath, absRoot)
		if err != nil {
			wbgong.Error.Fatalf("failed to load schema %s: %s", schemaPath, err)
		}
		content, err := json.MarshalIndent(schema.GetPreprocessed(), "", "  ")
		if err != nil {
			wbgong.Error.Fatalf("failed to serialize schema %s: %s", schemaPath, err)
		}
		os.Stdout.Write(content)
		os.Exit(0)
	}

	editor := confed.NewEditor(absRoot)
	watcher := wbgong.NewDirWatcher("\\.schema.json$", confed.NewEditorDirWatcherClient(editor))

	gotSome := false
	for _, path := range flag.Args() {
		if err := watcher.Load(path); err != nil {
			wbgong.Error.Printf("error loading schema file/dir %s: %s", path, err)
		} else {
			gotSome = true
		}
	}
	if !gotSome {
		wbgong.Error.Fatalf("no valid schemas found")
	}
	confed.RunRequestHandler(editor.RequestCh)

	// prepare exit signal channel
	exitCh := make(chan os.Signal, 1)
	signal.Notify(exitCh, syscall.SIGINT, syscall.SIGTERM)

	if *brokerAddress == DEFAULT_BROKER_URL && isSocket(MOSQUITTO_SOCK_FILE) {
		wbgong.Info.Println("broker URL is default and mosquitto socket detected, trying to connect via it")
		*brokerAddress = "unix://" + MOSQUITTO_SOCK_FILE
	}

	mqttClient := wbgong.NewPahoMQTTClient(*brokerAddress, DRIVER_CLIENT_ID)
	rpc := wbgong.NewMQTTRPCServer("confed", mqttClient)
	rpc.Register(editor)
	rpc.Start()
	defer rpc.Stop()

	// wait for quit signal
	<-exitCh
}
