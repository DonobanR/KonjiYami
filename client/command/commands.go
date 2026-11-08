package command

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

	---------------------------------------------------------------------
	This file registers the minimal set of CLI commands that the KONJIYAMI UI relies on.
	Only the commands tied to Quick Actions, listener management, jobs, sessions and payload generation remain.
*/

import (
	"github.com/desertbit/grumble"
	"konjiyami/client/command/beacons"
	"konjiyami/client/command/filesystem"
	"konjiyami/client/command/generate"
	"konjiyami/client/command/info"
	"konjiyami/client/command/jobs"
	"konjiyami/client/command/network"
	"konjiyami/client/command/processes"
	"konjiyami/client/command/screenshot"
	"konjiyami/client/command/sessions"
	"konjiyami/client/command/use"
	"konjiyami/client/console"
	consts "konjiyami/client/constants"
)

const defaultTimeout = 60

func runCommand(ctx *grumble.Context, con *console.KonjiyamiConsoleClient, fn func(*grumble.Context, *console.KonjiyamiConsoleClient)) error {
	con.Println()
	fn(ctx, con)
	con.Println()
	return nil
}

// BindCommands registers the commands required by the KONJIYAMI UI.
func BindCommands(con *console.KonjiyamiConsoleClient) {
	con.App.AddCommand(sessionsCommand(con))
	con.App.AddCommand(jobsCommand(con))
	con.App.AddCommand(beaconsCommand(con))
	con.App.AddCommand(generateCommand(con))
	con.App.AddCommand(mtlsCommand(con))
	con.App.AddCommand(httpCommand(con))
	con.App.AddCommand(httpsCommand(con))
	con.App.AddCommand(dnsCommand(con))
	con.App.AddCommand(useCommand(con))
	con.App.AddCommand(infoCommand(con))
	con.App.AddCommand(whoamiCommand(con))
	con.App.AddCommand(psCommand(con))
	con.App.AddCommand(ifconfigCommand(con))
	con.App.AddCommand(netstatCommand(con))
	con.App.AddCommand(lsCommand(con))
	con.App.AddCommand(downloadCommand(con))
	con.App.AddCommand(uploadCommand(con))
	con.App.AddCommand(screenshotCommand(con))
}

func sessionsCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.SessionsStr,
		Help: "Session management",
		Flags: func(f *grumble.Flags) {
			f.String("i", "interact", "", "interact with a session")
			f.String("k", "kill", "", "kill the designated session")
			f.Bool("K", "kill-all", false, "kill all the sessions")
			f.Bool("C", "clean", false, "clean out any sessions marked as [DEAD]")
			f.Bool("F", "force", false, "force session action without waiting for results")

			f.String("f", "filter", "", "filter sessions by substring")
			f.String("e", "filter-re", "", "filter sessions by regular expression")

			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, sessions.SessionsCmd)
		},
		HelpGroup: consts.GenericHelpGroup,
	}
}

func jobsCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.JobsStr,
		Help: "Job control",
		Flags: func(f *grumble.Flags) {
			f.Int("k", "kill", -1, "kill a background job")
			f.Bool("K", "kill-all", false, "kill all jobs")

			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, jobs.JobsCmd)
		},
		HelpGroup: consts.GenericHelpGroup,
	}
}

func beaconsCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.BeaconsStr,
		Help: "Manage beacons",
		Flags: func(f *grumble.Flags) {
			f.String("k", "kill", "", "beacon ID to kill")
			f.Bool("K", "kill-all", false, "kill all beacons")
			f.String("f", "filter", "", "filter beacons by substring")
			f.String("e", "filter-re", "", "filter beacons by regular expression")
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, beacons.BeaconsCmd)
		},
		HelpGroup: consts.GenericHelpGroup,
	}
}

func generateCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.GenerateStr,
		Help: "Generate an implant binary",
		Flags: func(f *grumble.Flags) {
			f.String("o", "os", "windows", "operating system")
			f.String("a", "arch", "amd64", "cpu architecture")
			f.String("N", "name", "", "agent name")
			f.Bool("d", "debug", false, "enable debug features")
			f.String("O", "debug-file", "", "path to debug output")
			f.Bool("e", "evasion", false, "enable evasion features")
			f.Bool("l", "skip-symbols", false, "skip symbol obfuscation")
			f.String("I", "template", "KonjiYami", "implant code template")
			f.Bool("E", "external-builder", false, "use an external builder")
			f.Bool("G", "disable-sgn", false, "disable shikata ga nai shellcode encoder")

			f.String("c", "canary", "", "canary domain(s)")

			f.String("m", "mtls", "", "mtls connection strings")
			f.String("g", "wg", "", "wg connection strings")
			f.String("b", "http", "", "http(s) connection strings")
			f.String("n", "dns", "", "dns connection strings")
			f.String("p", "named-pipe", "", "named-pipe connection strings")
			f.String("i", "tcp-pivot", "", "tcp-pivot connection strings")

			f.Int("X", "key-exchange", generate.DefaultWGKeyExPort, "wg key-exchange port")
			f.Int("T", "tcp-comms", generate.DefaultWGNPort, "wg c2 comms port")

			f.Bool("R", "run-at-load", false, "run the implant entrypoint from DllMain/Constructor (shared library only)")

			f.String("Z", "strategy", "", "connection strategy: r=random, rd=random domain, s=sequential")
			f.Int("j", "reconnect", generate.DefaultReconnect, "attempt to reconnect every n second(s)")
			f.Int("P", "poll-timeout", generate.DefaultPollTimeout, "long poll request timeout")
			f.Int("k", "max-errors", generate.DefaultMaxErrors, "max number of connection errors")

			f.String("w", "limit-datetime", "", "limit execution to before datetime")
			f.Bool("x", "limit-domainjoined", false, "limit execution to domain joined machines")
			f.String("y", "limit-username", "", "limit execution to specified username")
			f.String("z", "limit-hostname", "", "limit execution to specified hostname")
			f.String("F", "limit-fileexists", "", "limit execution to hosts with this file")
			f.String("L", "limit-locale", "", "limit execution to hosts with this locale")

			f.String("f", "format", "exe", "output format (exe, shared, service, shellcode)")
			f.String("s", "save", "", "directory or file to save the binary")

			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, generate.GenerateCmd)
		},
		HelpGroup: consts.GenericHelpGroup,
	}
}

func mtlsCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.MtlsStr,
		Help: "Start an mTLS listener",
		Flags: func(f *grumble.Flags) {
			f.String("L", "lhost", "", "interface to bind server to")
			f.Int("l", "lport", generate.DefaultMTLSLPort, "tcp listen port")
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
			f.Bool("p", "persistent", false, "make persistent across restarts")
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, jobs.MTLSListenerCmd)
		},
		HelpGroup: consts.GenericHelpGroup,
	}
}

func httpCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.HttpStr,
		Help: "Start an HTTP listener",
		Flags: func(f *grumble.Flags) {
			f.String("d", "domain", "", "limit responses to specific domain")
			f.String("w", "website", "", "website name")
			f.String("L", "lhost", "", "interface to bind server to")
			f.Int("l", "lport", generate.DefaultHTTPLPort, "tcp listen port")
			f.Bool("D", "disable-otp", false, "disable OTP authentication")
			f.String("T", "long-poll-timeout", "1s", "server-side long poll timeout")
			f.String("J", "long-poll-jitter", "2s", "server-side long poll jitter")
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
			f.Bool("p", "persistent", false, "make persistent across restarts")
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, jobs.HTTPListenerCmd)
		},
		HelpGroup: consts.GenericHelpGroup,
	}
}

func httpsCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.HttpsStr,
		Help: "Start an HTTPS listener",
		Flags: func(f *grumble.Flags) {
			f.String("d", "domain", "", "limit responses to specific domain")
			f.String("w", "website", "", "website name")
			f.String("L", "lhost", "", "interface to bind server to")
			f.Int("l", "lport", generate.DefaultHTTPSLPort, "tcp listen port")
			f.Bool("D", "disable-otp", false, "disable OTP authentication")
			f.String("T", "long-poll-timeout", "1s", "server-side long poll timeout")
			f.String("J", "long-poll-jitter", "2s", "server-side long poll jitter")
			f.String("c", "cert", "", "PEM encoded certificate file")
			f.String("k", "key", "", "PEM encoded private key file")
			f.Bool("e", "lets-encrypt", false, "attempt to provision Let's Encrypt certificate")
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
			f.Bool("p", "persistent", false, "make persistent across restarts")
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, jobs.HTTPSListenerCmd)
		},
		HelpGroup: consts.GenericHelpGroup,
	}
}

func dnsCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.DnsStr,
		Help: "Start a DNS listener",
		Flags: func(f *grumble.Flags) {
			f.String("d", "domains", "", "parent domain(s) to use")
			f.Bool("c", "no-canaries", false, "disable DNS canary detection")
			f.String("L", "lhost", "", "interface to bind server to")
			f.Int("l", "lport", generate.DefaultDNSLPort, "udp listen port")
			f.Bool("D", "disable-otp", false, "disable OTP authentication")
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
			f.Bool("p", "persistent", false, "make persistent across restarts")
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, jobs.DNSListenerCmd)
		},
		HelpGroup: consts.GenericHelpGroup,
	}
}

func infoCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.InfoStr,
		Help: "Get info about a session",
		Flags: func(f *grumble.Flags) {
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
		},
		Args: func(a *grumble.Args) {
			a.String("session", "session ID", grumble.Default(""))
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, info.InfoCmd)
		},
		HelpGroup: consts.KONJIYAMIHelpGroup,
	}
}

func whoamiCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.WhoamiStr,
		Help: "Print current session user",
		Flags: func(f *grumble.Flags) {
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, info.WhoamiCmd)
		},
		HelpGroup: consts.KONJIYAMIHelpGroup,
	}
}

func psCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.PsStr,
		Help: "List remote processes",
		Flags: func(f *grumble.Flags) {
			f.Int("p", "pid", -1, "filter based on pid")
			f.String("e", "exe", "", "filter based on executable name")
			f.String("o", "owner", "", "filter based on owner")
			f.Bool("c", "print-cmdline", false, "print command line arguments")
			f.Bool("O", "overflow", false, "overflow terminal width")
			f.Int("S", "skip-pages", 0, "skip the first n page(s)")
			f.Bool("T", "tree", false, "print process tree")
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, processes.PsCmd)
		},
		HelpGroup: consts.KONJIYAMIHelpGroup,
	}
}

func ifconfigCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.IfconfigStr,
		Help: "View network interface configurations",
		Flags: func(f *grumble.Flags) {
			f.Bool("A", "all", false, "show all network adapters")
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, network.IfconfigCmd)
		},
		HelpGroup: consts.KONJIYAMIHelpGroup,
	}
}

func netstatCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.NetstatStr,
		Help: "Print network connection info",
		Flags: func(f *grumble.Flags) {
			f.Bool("T", "tcp", true, "display TCP sockets")
			f.Bool("u", "udp", false, "display UDP sockets")
			f.Bool("4", "ip4", true, "display IPv4 sockets")
			f.Bool("6", "ip6", false, "display IPv6 sockets")
			f.Bool("l", "listen", false, "display listening sockets")
			f.Bool("n", "numeric", false, "display numeric addresses")
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, network.NetstatCmd)
		},
		HelpGroup: consts.KONJIYAMIHelpGroup,
	}
}

func useCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	cmd := &grumble.Command{
		Name: consts.UseStr,
		Help: "Switch the active session or beacon",
		Flags: func(f *grumble.Flags) {
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
		},
		Args: func(a *grumble.Args) {
			a.String("id", "beacon or session ID", grumble.Default(""))
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, use.UseCmd)
		},
		HelpGroup: consts.GenericHelpGroup,
	}

	cmd.AddCommand(&grumble.Command{
		Name: consts.SessionsStr,
		Help: "Switch the active session",
		Flags: func(f *grumble.Flags) {
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, use.UseSessionCmd)
		},
		HelpGroup: consts.GenericHelpGroup,
	})

	cmd.AddCommand(&grumble.Command{
		Name: consts.BeaconsStr,
		Help: "Switch the active beacon",
		Flags: func(f *grumble.Flags) {
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, use.UseBeaconCmd)
		},
		HelpGroup: consts.GenericHelpGroup,
	})

	return cmd
}

func lsCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.LsStr,
		Help: "List current directory",
		Flags: func(f *grumble.Flags) {
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
			f.Bool("r", "reverse", false, "reverse sort order")
			f.Bool("m", "modified", false, "sort by modified time")
			f.Bool("s", "size", false, "sort by size")
		},
		Args: func(a *grumble.Args) {
			a.String("path", "path to enumerate", grumble.Default("."))
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, filesystem.LsCmd)
		},
		HelpGroup: consts.KONJIYAMIHelpGroup,
	}
}

func downloadCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.DownloadStr,
		Help: "Download a file",
		Flags: func(f *grumble.Flags) {
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
			f.Bool("X", "loot", false, "save output as loot")
			f.String("T", "type", "", "force a specific loot type (file/cred)")
			f.String("F", "file-type", "", "force a specific file type (binary/text)")
			f.String("n", "name", "", "name to assign the download if looting")
			f.Bool("r", "recurse", false, "recursively download directories")
		},
		Args: func(a *grumble.Args) {
			a.String("remote-path", "path to the file or directory to download")
			a.String("local-path", "local path where the downloaded file will be saved", grumble.Default("."))
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, filesystem.DownloadCmd)
		},
		HelpGroup: consts.KONJIYAMIHelpGroup,
	}
}

func uploadCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.UploadStr,
		Help: "Upload a file",
		Flags: func(f *grumble.Flags) {
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
			f.Bool("i", "ioc", false, "track uploaded file as an IOC")
		},
		Args: func(a *grumble.Args) {
			a.String("local-path", "local path to the file to upload")
			a.String("remote-path", "path to upload to", grumble.Default(""))
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, filesystem.UploadCmd)
		},
		HelpGroup: consts.KONJIYAMIHelpGroup,
	}
}

func screenshotCommand(con *console.KonjiyamiConsoleClient) *grumble.Command {
	return &grumble.Command{
		Name: consts.ScreenshotStr,
		Help: "Take a screenshot",
		Flags: func(f *grumble.Flags) {
			f.String("s", "save", "", "save to file (will overwrite if exists)")
			f.Bool("X", "loot", false, "save output as loot")
			f.String("n", "name", "", "name to assign loot")
			f.Int("t", "timeout", defaultTimeout, "command timeout in seconds")
		},
		Run: func(ctx *grumble.Context) error {
			return runCommand(ctx, con, screenshot.ScreenshotCmd)
		},
		HelpGroup: consts.KONJIYAMIHelpGroup,
	}
}
