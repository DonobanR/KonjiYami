package daemon

/*
	KONJIYAMI Implant Framework
	Copyright (C) 2019  Bishop Fox

	This program is free software: you can redistribute it and/or modify
	it under the terms of the GNU General Public License as published by
	the Free Software Foundation, either version 3 of the License, or
	(at your option) any later version.

	This program is distributed in the hope that it will be useful,
	but WITHOUT ANY WARRANTY; without even the implied warranty of
	MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
	GNU General Public License for more details.

	You should have received a copy of the GNU General Public License
	along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"konjiyami/server/api"
	"konjiyami/server/configs"
	"konjiyami/server/log"
	"konjiyami/server/transport"
)

var (
	serverConfig = configs.GetServerConfig()
	daemonLog    = log.NamedLogger("daemon", "main")

	// BlankHost is a blank hostname
	BlankHost = "-"
	// BlankPort is a blank port number
	BlankPort = uint16(0)
)

// Start - Start as daemon process
func Start(host string, port uint16, devMode bool, insecure bool) {

	// cli args take president over config
	if host == BlankHost {
		daemonLog.Info("No cli lhost, using config file or default value")
		host = serverConfig.DaemonConfig.Host
	}
	if port == BlankPort {
		daemonLog.Info("No cli lport, using config file or default value")
		port = uint16(serverConfig.DaemonConfig.Port)
	}

	daemonLog.Infof("Starting KONJIYAMI daemon %s:%d ...", host, port)
	_, ln, err := transport.StartClientListener(host, port)
	if err != nil {
		fmt.Printf("[!] Failed to start daemon %s", err)
		daemonLog.Errorf("Error starting client listener %s", err)
		os.Exit(1)
	}

	httpServer := api.NewHTTPServer(devMode)
	if err := httpServer.Start("127.0.0.1", 8080, insecure); err != nil {
		daemonLog.Errorf("Failed to start HTTP API: %v", err)
	}

	done := make(chan bool)
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	go func() {
		<-signals
		daemonLog.Infof("Received SIGTERM, exiting ...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpServer.Shutdown(ctx)
		ln.Close()
		done <- true
	}()
	<-done
}

