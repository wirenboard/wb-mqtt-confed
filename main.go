package main

import (
	"./confed"
	"flag"
	"github.com/contactless/wbgo"
	"time"
)

const DRIVER_CLIENT_ID = "confed"

func main() {
	brokerAddress := flag.String("broker", "tcp://localhost:1883", "MQTT broker url")
	debug := flag.Bool("debug", false, "Enable debugging")
	useSyslog := flag.Bool("syslog", false, "Use syslog for logging")
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

	editor := confed.NewEditor()
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

	mqttClient := wbgo.NewPahoMQTTClient(*brokerAddress, DRIVER_CLIENT_ID, true)
	rpc := wbgo.NewMQTTRPCServer("confed", mqttClient)
	rpc.Register(editor)
	rpc.Start()

	for {
		time.Sleep(1 * time.Second)
	}
}
