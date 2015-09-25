package main

import (
	"./confed"
	"encoding/json"
	"flag"
	"github.com/contactless/wbgo"
	"os"
	"path/filepath"
	"time"
)

const DRIVER_CLIENT_ID = "confed"

func main() {
	brokerAddress := flag.String("broker", "tcp://localhost:1883", "MQTT broker url")
	root := flag.String("root", "/", "Config root path")
	debug := flag.Bool("debug", false, "Enable debugging")
	useSyslog := flag.Bool("syslog", false, "Use syslog for logging")
	validate := flag.Bool("validate", false, "Validate specified config file and exit")
	dump := flag.Bool("dump", false, "Dump preprocessed schema and exit")
	flag.Parse()
	if flag.NArg() < 1 {
		wbgo.Error.Fatal("must specify schema(s) / schema directory(ies)")
	}
	if *useSyslog {
		wbgo.UseSyslog()
	}
	if *debug {
		wbgo.SetDebuggingEnabled(true)
	}
	absRoot, err := filepath.Abs(*root)
	if err != nil {
		wbgo.Error.Fatal("failed to get absolute path for root")
	}

	// TBD: don't watch subconfs while validating/dumping
	if *validate {
		if flag.NArg() != 2 {
			// TBD: don't require config path, it should
			// be taken from schema if it's not specified
			wbgo.Error.Fatal("must specify schema and config files")
		}
		schemaPath, configPath := flag.Arg(0), flag.Arg(1)
		schema, err := confed.NewJSONSchemaWithRoot(schemaPath, absRoot)
		if err != nil {
			wbgo.Error.Fatal("failed to load schema %s: %s", schemaPath, err)
		}
		r, err := schema.ValidateFile(configPath)
		if err != nil {
			wbgo.Error.Fatal("failed to validate %s: %s", configPath, err)
		}
		if !r.Valid() {
			wbgo.Error.Printf("Validation failed for %s", configPath)
			for _, desc := range r.Errors() {
				wbgo.Error.Printf("- %s\n", desc)
			}
			os.Exit(1)
		}
		os.Exit(0)
	}
	if *dump {
		if flag.NArg() != 1 {
			wbgo.Error.Fatal("must specify schema file")
		}
		schemaPath := flag.Arg(0)
		schema, err := confed.NewJSONSchemaWithRoot(schemaPath, absRoot)
		if err != nil {
			wbgo.Error.Fatal("failed to load schema %s: %s", schemaPath, err)
		}
		content, err := json.MarshalIndent(schema.GetPreprocessed(), "", "  ")
		if err != nil {
			wbgo.Error.Fatal("failed to serialize schema %s: %s", schemaPath, err)
		}
		os.Stdout.Write(content)
		os.Exit(0)
	}

	editor := confed.NewEditor(absRoot)
	watcher := wbgo.NewDirWatcher("\\.schema.json$", confed.NewEditorDirWatcherClient(editor))

	gotSome := false
	for _, path := range flag.Args() {
		if err := watcher.Load(path); err != nil {
			wbgo.Error.Printf("error loading schema file/dir %s: %s", path, err)
		} else {
			gotSome = true
		}
	}
	if !gotSome {
		wbgo.Error.Fatalf("no valid schemas found")
	}
	confed.RunRestarter(editor.RestartCh)

	mqttClient := wbgo.NewPahoMQTTClient(*brokerAddress, DRIVER_CLIENT_ID, true)
	rpc := wbgo.NewMQTTRPCServer("confed", mqttClient)
	rpc.Register(editor)
	rpc.Start()

	for {
		time.Sleep(1 * time.Second)
	}
}
