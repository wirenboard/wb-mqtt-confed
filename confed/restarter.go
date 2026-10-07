package confed

import (
	"time"

	"strconv"

	"github.com/wirenboard/wbgong"
)

const (
	serviceCmd = "systemctl"
)

func restartService(name string) (err error) {
	_, err = runCommand(false, nil, serviceCmd, "reload-or-restart", name)
	return
}

// RunRequestHandler starts a goroutine processing requests from the channel.
func RunRequestHandler(ch chan Request) {
	go func() {
		for {
			req := <-ch
			switch req.requestType {
			case Sleep:
				delay, _ := strconv.Atoi(req.properties["delay"])
				wbgong.Debug.Printf("Delay %d ms before restarting services", delay)
				time.Sleep(time.Duration(delay) * time.Millisecond)
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
