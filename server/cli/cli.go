package cli

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
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"konjiyami/server/assets"
	"konjiyami/server/c2"
	"konjiyami/server/certs"
	"konjiyami/server/configs"
	"konjiyami/server/console"
	"konjiyami/server/cryptography"
	"konjiyami/server/daemon"
	"konjiyami/server/transport"
	"github.com/spf13/cobra"
)

const (

	// Unpack flags
	forceFlagStr = "force"

	// Operator flags
	nameFlagStr  = "name"
	lhostFlagStr = "lhost"
	lportFlagStr = "lport"
	saveFlagStr  = "save"

	// Cert flags
	caTypeFlagStr = "type"
	loadFlagStr   = "load"

	// console log file name
	logFileName = "console.log"
)

// Initialize logging
func initConsoleLogging(appDir string) *os.File {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	logFile, err := os.OpenFile(filepath.Join(appDir, "logs", logFileName), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	log.SetOutput(logFile)
	return logFile
}

func init() {

	// Unpack
	unpackCmd.Flags().BoolP(forceFlagStr, "f", false, "Force unpack and overwrite")
	rootCmd.AddCommand(unpackCmd)

	// Operator
	operatorCmd.Flags().StringP(nameFlagStr, "n", "", "operator name")
	operatorCmd.Flags().StringP(lhostFlagStr, "l", "", "multiplayer listener host")
	operatorCmd.Flags().Uint16P(lportFlagStr, "p", uint16(31337), "multiplayer listener port")
	operatorCmd.Flags().StringP(saveFlagStr, "s", "", "save file to ...")
	rootCmd.AddCommand(operatorCmd)

	// Certs
	cmdExportCA.Flags().StringP(saveFlagStr, "s", "", "save CA to file ...")
	cmdExportCA.Flags().StringP(caTypeFlagStr, "t", "", fmt.Sprintf("ca type (%s)", strings.Join(validCATypes(), ", ")))
	rootCmd.AddCommand(cmdExportCA)

	cmdImportCA.Flags().StringP(loadFlagStr, "l", "", "load CA from file ...")
	cmdImportCA.Flags().StringP(caTypeFlagStr, "t", "", fmt.Sprintf("ca type (%s)", strings.Join(validCATypes(), ", ")))
	rootCmd.AddCommand(cmdImportCA)

	// Daemon
	daemonCmd.Flags().StringP(lhostFlagStr, "l", daemon.BlankHost, "multiplayer listener host")
	daemonCmd.Flags().Uint16P(lportFlagStr, "p", daemon.BlankPort, "multiplayer listener port")
	daemonCmd.Flags().BoolP(forceFlagStr, "f", false, "force unpack and overwrite static assets")
	daemonCmd.Flags().Bool(devFlagStr, false, "serve UI from disk instead of embedded FS (for UI development)")
	daemonCmd.Flags().Bool(insecureFlagStr, false, "use HTTP instead of HTTPS (for development)")
	rootCmd.AddCommand(daemonCmd)

	// Builder
	rootCmd.AddCommand(initBuilderCmd())

	// Version
	rootCmd.AddCommand(versionCmd)
}

var rootCmd = &cobra.Command{
	Use:   "KonjiYami-server",
	Short: "",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {

		// Root command starts the server normally

		appDir := assets.GetRootAppDir()
		logFile := initConsoleLogging(appDir)
		defer logFile.Close()

		defer func() {
			if r := recover(); r != nil {
				log.Printf("panic:\n%s", debug.Stack())
				fmt.Println("stacktrace from panic: \n" + string(debug.Stack()))
				os.Exit(99)
			}
		}()

		assets.Setup(false, true)
		certs.SetupCAs()
		certs.SetupWGKeys()
		cryptography.ECCServerKeyPair()
		cryptography.TOTPServerSecret()
		cryptography.MinisignServerPrivateKey()

		serverConfig := configs.GetServerConfig()
		c2.StartPersistentJobs(serverConfig)
		console.StartPersistentJobs(serverConfig)
		if serverConfig.DaemonMode {
			daemon.Start(daemon.BlankHost, daemon.BlankPort, false, true)
		} else {
			// Start the gRPC operator listener in background so the broker
			// can reach this process even in interactive console mode.
			grpcHost := serverConfig.DaemonConfig.Host
			if grpcHost == "" {
				grpcHost = "0.0.0.0"
			}
			grpcPort := uint16(serverConfig.DaemonConfig.Port)
			if grpcPort == 0 {
				grpcPort = 31337
			}
			if _, _, err := transport.StartClientListener(grpcHost, grpcPort); err != nil {
				fmt.Printf("[!] Warning: could not start gRPC listener on %s:%d: %v\n", grpcHost, grpcPort, err)
			}
			os.Args = os.Args[:1] // Hide cli from grumble console
			console.Start()
		}
	},
}

// Execute - Execute root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

