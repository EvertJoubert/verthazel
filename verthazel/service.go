package verthazel

import (
	"os"
	"strconv"
	"strings"

	"github.com/kardianos/osext"
	"github.com/kardianos/service"
)

var logger service.Logger

type Service struct {
	localpath   string
	serviceName string
}

//var svr *Service

func InvokeService() {
	localExecDir, err := osext.ExecutableFolder()
	if err == nil {
		//if svr == nil {
		svr := &Service{localpath: localExecDir}
		svr.setup()
		//}
	}
}

func (svr *Service) setup() {
	//args := os.Args[:]

}

func (svr *Service) Start(s service.Service) error {
	go svr.run()
	return nil
}

func (svr *Service) run() {
	RegisterWebActiveExtensions()
	MappedResources()
	ExecuteGlobalRequest(svr.serviceName + ".js")
	ports := []int{}
	for _, arg := range os.Args {
		if strings.HasPrefix(arg, "ports=") {
			arg = arg[len("ports="):]
			for _, sport := range strings.Split(arg, "|") {
				p, _ := strconv.ParseInt(sport, 0, 64)
				ports = append(ports, int(p))
			}
			ActivatePort(ports...)
		}
	}
	MonitorActiveEnvironment()
}

func (svr *Service) Stop(s service.Service) error {
	// Stop should not block. Return with a few seconds.
	return nil
}
