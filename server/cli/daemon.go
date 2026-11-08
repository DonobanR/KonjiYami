package cli

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"

	"konjiyami/server/assets"
	"konjiyami/server/c2"
	"konjiyami/server/certs"
	"konjiyami/server/configs"
	"konjiyami/server/cryptography"
	"konjiyami/server/daemon"
	"github.com/spf13/cobra"
)

const (
	devFlagStr      = "dev"
	insecureFlagStr = "insecure"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Force start server in daemon mode",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		force, err := cmd.Flags().GetBool(forceFlagStr)
		if err != nil {
			fmt.Printf("Failed to parse --%s flag %s\n", forceFlagStr, err)
			return
		}
		lhost, err := cmd.Flags().GetString(lhostFlagStr)
		if err != nil {
			fmt.Printf("Failed to parse --%s flag %s\n", lhostFlagStr, err)
			return
		}
		lport, err := cmd.Flags().GetUint16(lportFlagStr)
		if err != nil {
			fmt.Printf("Failed to parse --%s flag %s\n", lportFlagStr, err)
			return
		}
		devMode, err := cmd.Flags().GetBool(devFlagStr)
		if err != nil {
			fmt.Printf("Failed to parse --%s flag %s\n", devFlagStr, err)
			return
		}
		insecure, err := cmd.Flags().GetBool(insecureFlagStr)
		if err != nil {
			fmt.Printf("Failed to parse --%s flag %s\n", insecureFlagStr, err)
			return
		}

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

		assets.Setup(force, false)
		certs.SetupCAs()
		certs.SetupWGKeys()
		cryptography.ECCServerKeyPair()
		cryptography.TOTPServerSecret()
		cryptography.MinisignServerPrivateKey()

		serverConfig := configs.GetServerConfig()
		c2.StartPersistentJobs(serverConfig)

		daemon.Start(lhost, uint16(lport), devMode, insecure)
	},
}

