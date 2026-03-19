package confed

import (
	"time"

	"strconv"

	"github.com/wirenboard/wbgong"
)

const (
	SERVICE_CMD = "systemctl"
)

func restartService(name string) (err error) {
	_, err = runCommand(false, nil, SERVICE_CMD, "reload-or-restart", name)
	return
}

func RunRequestHandler(ch chan Request) {
	go func() {
		for {
			req := <-ch
			switch req.requestType {
			case Sleep:
				delay, _ := strconv.Atoi(req.properties["delay"])
				wbgong.Debug.Printf("Delay %d ms before restarting services", delay)
				time.Sleep(time.Duration(delay) * time.Millisecond)
			case Sync:
				path := req.properties["path"]
				wbgong.Debug.Printf("File sync %s", path)
				if _, err := runCommand(false, nil, "sync", path); err != nil {
					wbgong.Error.Printf("Error sync file %s: %s", path, err)
				}
			case Restart:
				service := req.properties["service"]
				wbgong.Debug.Printf("Restarting service %s", service)
				if err := restartService(service); err != nil {
					wbgong.Error.Printf("Error restarting %s: %s", service, err)
				}
			default:
				wbgong.Error.Printf("Unknown request type %d", req.requestType)
			}
		}
	}()
}
