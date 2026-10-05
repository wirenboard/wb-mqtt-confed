// Package main implements wb-mqtt-confed, an MQTT RPC service for
// editing JSON configuration files validated against JSON schemas.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" //nolint:gosec // pprof is served only when -profile is specified
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/wirenboard/wb-mqtt-confed/confed"
	"github.com/wirenboard/wbgong"
)

const (
	driverClientID    = "confed"
	mosquittoSockFile = "/var/run/mosquitto/mosquitto.sock"
	defaultBrokerURL  = "tcp://localhost:1883"
	wbgoFile          = "/usr/lib/wb-mqtt-confed/wbgo.so"

	profileReadHeaderTimeout = 10 * time.Second
)

func isSocket(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.Mode()&os.ModeSocket != 0
}

func runValidation(schemaPath, configPath, absRoot string) error {
	schema, err := confed.NewJSONSchemaWithRoot(schemaPath, absRoot)
	if err != nil {
		return fmt.Errorf("failed to load schema %s: %w", schemaPath, err)
	}

	r, err := schema.ValidateFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to validate %s: %w", configPath, err)
	}

	if !r.Valid() {
		var b strings.Builder

		fmt.Fprintf(&b, "Validation failed for %s\n", configPath)

		for _, desc := range r.Errors() {
			fmt.Fprintf(&b, "- %s\n", desc)
		}

		return fmt.Errorf("%s", b.String())
	}

	return nil
}

func runDump(schemaPath, absRoot string) error {
	schema, err := confed.NewJSONSchemaWithRoot(schemaPath, absRoot)
	if err != nil {
		return fmt.Errorf("failed to load schema %s: %w", schemaPath, err)
	}

	content, err := json.MarshalIndent(schema.GetPreprocessed(), "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize schema %s: %w", schemaPath, err)
	}

	if _, err = os.Stdout.Write(content); err != nil {
		return fmt.Errorf("failed to write schema %s: %w", schemaPath, err)
	}

	return nil
}

var version = "unknown"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(version)
		os.Exit(0)
	}

	brokerAddress := flag.String("broker", defaultBrokerURL, "MQTT broker url")
	root := flag.String("root", "/", "Config root path")
	debug := flag.Bool("debug", false, "Enable debugging")
	useSyslog := flag.Bool("syslog", false, "Use syslog for logging")
	validate := flag.Bool("validate", false, "Validate specified config file and exit")
	dump := flag.Bool("dump", false, "Dump preprocessed schema and exit")
	wbgoso := flag.String("wbgo", wbgoFile, "Location to wbgo.so file")
	profile := flag.String("profile", "", "Run pprof server")
	flag.Parse()

	if *profile != "" {
		go func() {
			server := &http.Server{
				Addr:              *profile,
				ReadHeaderTimeout: profileReadHeaderTimeout,
			}
			log.Println(server.ListenAndServe())
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

		if err = runValidation(schemaPath, configPath, absRoot); err != nil {
			wbgong.Error.Fatal(err)
		}

		os.Exit(0)
	}
	if *dump {
		if flag.NArg() != 1 {
			wbgong.Error.Fatal("must specify schema file")
		}
		if err = runDump(flag.Arg(0), absRoot); err != nil {
			wbgong.Error.Fatal(err)
		}
		os.Exit(0)
	}

	editor := confed.NewEditor(absRoot)
	watcher := wbgong.NewDirWatcher("\\.schema.json$", confed.NewEditorDirWatcherClient(editor))

	gotSome := false
	for _, path := range flag.Args() {
		if err = watcher.Load(path); err != nil {
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

	if *brokerAddress == defaultBrokerURL && isSocket(mosquittoSockFile) {
		wbgong.Info.Println("broker URL is default and mosquitto socket detected, trying to connect via it")
		*brokerAddress = "unix://" + mosquittoSockFile
	}

	mqttClient := wbgong.NewPahoMQTTClient(*brokerAddress, driverClientID)
	rpc := wbgong.NewMQTTRPCServer("confed", mqttClient)
	err = rpc.Register(editor)
	if err != nil {
		wbgong.Error.Fatalf("failed to register editor: %v", err)
	}
	rpc.Start()
	defer rpc.Stop()

	// wait for quit signal
	<-exitCh
}
