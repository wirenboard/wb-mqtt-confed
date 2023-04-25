package confed

import (
	"github.com/wirenboard/wbgong"
	"time"
)

const (
	SERVICE_CMD = "systemctl"
)

func restartService(name string) (err error) {
	_, err = runCommand(false, nil, SERVICE_CMD, "reload-or-restart", name)
	return
}

func RunRestarter(ch chan RestartRequest) {
	go func() {
		for {
			req := <-ch
			wbgong.Debug.Printf("Restarting service %s (delay %d ms)",
				req.Name, req.DelayMS)
			if req.DelayMS > 0 {
				time.Sleep(time.Duration(req.DelayMS) * time.Millisecond)
			}
			if err := restartService(req.Name); err != nil {
				wbgong.Error.Printf("Error restarting %s: %s", req.Name, err)
			}
		}
	}()
}
