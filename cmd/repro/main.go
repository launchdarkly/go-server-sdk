package main

import (
	"github.com/launchdarkly/go-sdk-common/v3/ldlog"
	ldclient "github.com/launchdarkly/go-server-sdk/v6"
	"github.com/launchdarkly/go-server-sdk/v6/interfaces"
	"github.com/launchdarkly/go-server-sdk/v6/ldcomponents"
	"log"
	_ "net/http/pprof"
	"os"
	"time"
)

func main() {
	var config ldclient.Config
	config.Logging = ldcomponents.Logging().MinLevel(ldlog.Debug)
	then := time.Now()
	cl, err := ldclient.MakeCustomClient(os.Getenv("SDK_KEY"), config, 0)
	if err != nil {
		panic(err)
	}
	status := cl.GetDataSourceStatusProvider().AddStatusListener()

	timer := time.NewTimer(30 * time.Second)
	for {
		select {
		case s := <-status:
			if s.State == interfaces.DataSourceStateValid {
				log.Println("initialized in", time.Since(then))
			} else {
				log.Println("couldn't initialize:", s.LastError)
			}
			return
		case <-timer.C:
			log.Println("timeout in", time.Since(then))
			return
		}
	}
}
