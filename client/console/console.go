package console

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
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"konjiyami/client/assets"
	consts "konjiyami/client/constants"
	"konjiyami/client/core"
	"konjiyami/client/prelude"
	"konjiyami/client/spin"
	"konjiyami/client/version"
	"konjiyami/protobuf/clientpb"
	"konjiyami/protobuf/commonpb"
	"konjiyami/protobuf/rpcpb"
	"github.com/desertbit/go-shlex"
	"github.com/desertbit/grumble"
	"github.com/fatih/color"
	"github.com/gofrs/uuid"
	"google.golang.org/protobuf/proto"
)

const (
	// ANSI Colors
	Normal    = "\033[0m"
	Black     = "\033[30m"
	Red       = "\033[31m"
	Green     = "\033[32m"
	Orange    = "\033[33m"
	Blue      = "\033[34m"
	Purple    = "\033[35m"
	Cyan      = "\033[36m"
	Gray      = "\033[37m"
	Bold      = "\033[1m"
	Clearln   = "\r\x1b[2K"
	UpN       = "\033[%dA"
	DownN     = "\033[%dB"
	Underline = "\033[4m"

	// Info - Display colorful information
	Info = Bold + Cyan + "[*] " + Normal
	// Warn - Warn a user
	Warn = Bold + Red + "[!] " + Normal
	// Debug - Display debug information
	Debug = Bold + Purple + "[-] " + Normal
	// Woot - Display success
	Woot = Bold + Green + "[$] " + Normal
	// Success - Diplay success
	Success = Bold + Green + "[+] " + Normal
)

// Observer - A function to call when the sessions changes
type Observer func(*clientpb.Session, *clientpb.Beacon)
type BeaconTaskCallback func(*clientpb.BeaconTask)

type ActiveTarget struct {
	session    *clientpb.Session
	beacon     *clientpb.Beacon
	observers  map[int]Observer
	observerID int
}

type KonjiyamiConsoleClient struct {
	App                      *grumble.App
	Rpc                      rpcpb.KonjiyamiRPCClient
	ActiveTarget             *ActiveTarget
	EventListeners           *sync.Map
	BeaconTaskCallbacks      map[string]BeaconTaskCallback
	BeaconTaskCallbacksMutex *sync.Mutex
	IsServer                 bool
	Settings                 *assets.ClientSettings
}

// BindCmds - Bind extra commands to the app object
type BindCmds func(console *KonjiyamiConsoleClient)

// Start - Console entrypoint
func Start(rpc rpcpb.KonjiyamiRPCClient, bindCmds BindCmds, extraCmds BindCmds, isServer bool) error {
	assets.Setup(false, false)
	settings, _ := assets.LoadSettings()
	con := &KonjiyamiConsoleClient{
		App: grumble.New(&grumble.Config{
			Name:                  "KonjiYami",
			Description:           "KonjiYami Client",
			HistoryFile:           filepath.Join(assets.GetRootAppDir(), "history"),
			PromptColor:           color.New(),
			HelpHeadlineColor:     color.New(),
			HelpHeadlineUnderline: true,
			HelpSubCommands:       true,
			VimMode:               settings.VimMode,
		}),
		Rpc: rpc,
		ActiveTarget: &ActiveTarget{
			observers:  map[int]Observer{},
			observerID: 0,
		},
		EventListeners:           &sync.Map{},
		BeaconTaskCallbacks:      map[string]BeaconTaskCallback{},
		BeaconTaskCallbacksMutex: &sync.Mutex{},
		IsServer:                 isServer,
		Settings:                 settings,
	}
	con.App.SetPrintASCIILogo(func(_ *grumble.App) {
		con.PrintLogo()
	})
	con.App.SetPrompt(con.GetPrompt())
	bindCmds(con)
	extraCmds(con)

	con.ActiveTarget.AddObserver(func(_ *clientpb.Session, _ *clientpb.Beacon) {
		con.App.SetPrompt(con.GetPrompt())
	})

	go con.startEventLoop()
	go core.TunnelLoop(rpc)

	err := con.App.Run()
	if err != nil {
		log.Printf("Run loop returned error: %v", err)
	}
	return err
}

func (con *KonjiyamiConsoleClient) startEventLoop() {
	eventStream, err := con.Rpc.Events(context.Background(), &commonpb.Empty{})
	if err != nil {
		fmt.Printf(Warn+"%s\n", err)
		return
	}
	for {
		event, err := eventStream.Recv()
		if err == io.EOF || event == nil {
			return
		}

		go con.triggerEventListeners(event)

		// Trigger event based on type
		echoed := false // Only echo the event once
		switch event.EventType {

		case consts.CanaryEvent:
			eventMsg := fmt.Sprintf(Bold+"WARNING: %s%s has been burned (DNS Canary)\n", Normal, event.Session.Name)
			sessions := con.GetSessionsByName(event.Session.Name)
			for _, session := range sessions {
				shortID := strings.Split(session.ID, "-")[0]
				con.PrintEventErrorf(eventMsg+"\n"+Clearln+"\t🔥 Session %s is affected\n", shortID)
			}
			echoed = true

		case consts.WatchtowerEvent:
			msg := string(event.Data)
			eventMsg := fmt.Sprintf(Bold+"WARNING: %s%s has been burned (seen on %s)\n", Normal, event.Session.Name, msg)
			sessions := con.GetSessionsByName(event.Session.Name)
			for _, session := range sessions {
				shortID := strings.Split(session.ID, "-")[0]
				con.PrintEventErrorf(eventMsg+"\n"+Clearln+"\t🔥 Session %s is affected", shortID)
			}
			echoed = true

		case consts.JoinedEvent:
			con.PrintEventInfof("%s has joined the game", event.Client.Operator.Name)
			echoed = true
		case consts.LeftEvent:
			con.PrintEventInfof("%s left the game", event.Client.Operator.Name)
			echoed = true

		case consts.JobStoppedEvent:
			job := event.Job
			con.PrintEventErrorf("Job #%d stopped (%s/%s)", job.ID, job.Protocol, job.Name)
			echoed = true

		case consts.SessionOpenedEvent:
			session := event.Session
			currentTime := time.Now().Format(time.RFC1123)
			shortID := strings.Split(session.ID, "-")[0]
			con.PrintEventInfof("Session %s %s - %s (%s) - %s/%s - %v",
				shortID, session.Name, session.RemoteAddress, session.Hostname, session.OS, session.Arch, currentTime)

			// Prelude Operator
			if prelude.ImplantMapper != nil {
				err = prelude.ImplantMapper.AddImplant(session, nil)
				if err != nil {
					con.PrintEventErrorf("Could not add session to Operator: %s", err)
				}
			}
			echoed = true

		case consts.SessionUpdateEvent:
			session := event.Session
			currentTime := time.Now().Format(time.RFC1123)
			shortID := strings.Split(session.ID, "-")[0]
			con.PrintEventInfof("Session %s has been updated - %v", shortID, currentTime)
			echoed = true

		case consts.SessionClosedEvent:
			session := event.Session
			currentTime := time.Now().Format(time.RFC1123)
			shortID := strings.Split(session.ID, "-")[0]
			con.PrintEventErrorf("Lost session %s %s - %s (%s) - %s/%s - %v",
				shortID, session.Name, session.RemoteAddress, session.Hostname, session.OS, session.Arch, currentTime)
			activeSession := con.ActiveTarget.GetSession()
			core.GetTunnels().CloseForSession(session.ID)
			core.CloseCursedProcesses(session.ID)
			if activeSession != nil && activeSession.ID == session.ID {
				con.ActiveTarget.Set(nil, nil)
				con.PrintEventErrorf("Active session disconnected")
				con.App.SetPrompt(con.GetPrompt())
			}
			if prelude.ImplantMapper != nil {
				err = prelude.ImplantMapper.RemoveImplant(session)
				if err != nil {
					con.PrintEventErrorf("Could not remove session from Operator: %s", err)
				}
				con.PrintEventInfof("Removed session %s from Operator", session.Name)
			}
			echoed = true

		case consts.BeaconRegisteredEvent:
			beacon := &clientpb.Beacon{}
			proto.Unmarshal(event.Data, beacon)
			currentTime := time.Now().Format(time.RFC1123)
			shortID := strings.Split(beacon.ID, "-")[0]
			con.PrintEventInfof("Beacon %s %s - %s (%s) - %s/%s - %v",
				shortID, beacon.Name, beacon.RemoteAddress, beacon.Hostname, beacon.OS, beacon.Arch, currentTime)

			// Prelude Operator
			if prelude.ImplantMapper != nil {
				err = prelude.ImplantMapper.AddImplant(beacon, func(taskID string, cb func(*clientpb.BeaconTask)) {
					con.AddBeaconCallback(taskID, cb)
				})
				if err != nil {
					con.PrintEventErrorf("Could not add beacon to Operator: %s", err)
				}
			}
			echoed = true

		case consts.BeaconTaskResultEvent:
			con.triggerBeaconTaskCallback(event.Data)
			echoed = true

		}

		con.triggerReactions(event)

		// Only render if we echoed the event
		if echoed {
			con.Printf(Clearln + con.GetPrompt())
			bufio.NewWriter(con.App.Stdout()).Flush()
		}
	}
}

func (con *KonjiyamiConsoleClient) CreateEventListener() (string, <-chan *clientpb.Event) {
	listener := make(chan *clientpb.Event, 100)
	listenerID, _ := uuid.NewV4()
	con.EventListeners.Store(listenerID.String(), listener)
	return listenerID.String(), listener
}

func (con *KonjiyamiConsoleClient) RemoveEventListener(listenerID string) {
	value, ok := con.EventListeners.LoadAndDelete(listenerID)
	if ok {
		close(value.(chan *clientpb.Event))
	}
}

func (con *KonjiyamiConsoleClient) triggerEventListeners(event *clientpb.Event) {
	con.EventListeners.Range(func(key, value interface{}) bool {
		listener := value.(chan *clientpb.Event)
		listener <- event // Do not block while sending the event to the listener
		return true
	})
}

func (con *KonjiyamiConsoleClient) triggerReactions(event *clientpb.Event) {
	reactions := core.Reactions.On(event.EventType)
	if len(reactions) == 0 {
		return
	}

	// We need some special handling for SessionOpenedEvent to
	// set the new session as the active session
	currentActiveSession, currentActiveBeacon := con.ActiveTarget.Get()
	defer func() {
		con.ActiveTarget.Set(currentActiveSession, currentActiveBeacon)
	}()

	con.ActiveTarget.Set(nil, nil)
	if event.EventType == consts.SessionOpenedEvent {
		con.ActiveTarget.Set(event.Session, nil)
	} else if event.EventType == consts.BeaconRegisteredEvent {
		beacon := &clientpb.Beacon{}
		proto.Unmarshal(event.Data, beacon)
		con.ActiveTarget.Set(nil, beacon)
	}

	for _, reaction := range reactions {
		for _, line := range reaction.Commands {
			con.PrintInfof("Execute reaction: '%s'\n", line)
			args, err := shlex.Split(line, true)
			if err != nil {
				con.PrintErrorf("Reaction command has invalid args: %s\n", err)
				continue
			}
			err = con.App.RunCommand(args)
			if err != nil {
				con.PrintErrorf("Reaction command error: %s\n", err)
			}
		}
	}
}

// triggerBeaconTaskCallback - Triggers the callback for a beacon task
func (con *KonjiyamiConsoleClient) triggerBeaconTaskCallback(data []byte) {
	task := &clientpb.BeaconTask{}
	err := proto.Unmarshal(data, task)
	if err != nil {
		con.PrintErrorf("\rCould not unmarshal beacon task: %s\n", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	beacon, _ := con.Rpc.GetBeacon(ctx, &clientpb.Beacon{ID: task.BeaconID})

	// If the callback is not in our map then we don't do anything, the beacon task
	// was either issued by another operator in multiplayer mode or the client process
	// was restarted between the time the task was created and when the server got the result
	con.BeaconTaskCallbacksMutex.Lock()
	defer con.BeaconTaskCallbacksMutex.Unlock()
	if callback, ok := con.BeaconTaskCallbacks[task.ID]; ok {
		if con.Settings.BeaconAutoResults {
			if beacon != nil {
				con.PrintEventSuccessf("%s completed task %s", beacon.Name, strings.Split(task.ID, "-")[0])
			}
			task_content, err := con.Rpc.GetBeaconTaskContent(ctx, &clientpb.BeaconTask{
				ID: task.ID,
			})
			con.Printf(Clearln + "\r")
			if err == nil {
				callback(task_content)
			} else {
				con.PrintErrorf("Could not get beacon task content: %s\n", err)
			}
			con.Println()
		}
		delete(con.BeaconTaskCallbacks, task.ID)
	}
}

func (con *KonjiyamiConsoleClient) AddBeaconCallback(taskID string, callback BeaconTaskCallback) {
	con.BeaconTaskCallbacksMutex.Lock()
	defer con.BeaconTaskCallbacksMutex.Unlock()
	con.BeaconTaskCallbacks[taskID] = callback
}

func (con *KonjiyamiConsoleClient) GetPrompt() string {
	prompt := Underline + "KonjiYami" + Normal
	if con.IsServer {
		prompt = Bold + "[server] " + Normal + Underline + "KonjiYami" + Normal
	}
	if con.ActiveTarget.GetSession() != nil {
		prompt += fmt.Sprintf(Bold+Red+" (%s)%s", con.ActiveTarget.GetSession().Name, Normal)
	} else if con.ActiveTarget.GetBeacon() != nil {
		prompt += fmt.Sprintf(Bold+Blue+" (%s)%s", con.ActiveTarget.GetBeacon().Name, Normal)
	}
	prompt += " > "
	return Clearln + prompt
}

func (con *KonjiyamiConsoleClient) PrintLogo() {
	serverVer, err := con.Rpc.GetVersion(context.Background(), &commonpb.Empty{})
	if err != nil {
		panic(err.Error())
	}
	dirty := ""
	if serverVer.Dirty {
		dirty = fmt.Sprintf(" - %sDirty%s", Bold, Normal)
	}
	serverSemVer := fmt.Sprintf("%d.%d.%d", serverVer.Major, serverVer.Minor, serverVer.Patch)

	// Logo removed - KONJIYAMI branding
	con.Printf(Info+"Connected to KONJIYAMI Server v%s - %s%s\n", serverSemVer, serverVer.Commit, dirty)
	if version.GitCommit != serverVer.Commit {
		con.Printf(Info+"Client %s\n", version.FullVersion())
	}
	con.Printf(Info + "Type 'help' for available commands\n")
	con.Println()
	if serverVer.Major != int32(version.SemanticVersion()[0]) {
		con.Printf(Warn + "Warning: Client and server may be running incompatible versions.\n")
	}
	con.CheckLastUpdate()
}

func (con *KonjiyamiConsoleClient) CheckLastUpdate() {
	now := time.Now()
	lastUpdate := getLastUpdateCheck()
	compiledAt, err := version.Compiled()
	if err != nil {
		log.Printf("Failed to parse compiled at timestamp %s", err)
		return
	}

	day := 24 * time.Hour
	if compiledAt.Add(30 * day).Before(now) {
		if lastUpdate == nil || lastUpdate.Add(30*day).Before(now) {
			con.Printf(Info + "Check for updates with the 'update' command\n\n")
		}
	}
}

func getLastUpdateCheck() *time.Time {
	appDir := assets.GetRootAppDir()
	lastUpdateCheckPath := filepath.Join(appDir, consts.LastUpdateCheckFileName)
	data, err := ioutil.ReadFile(lastUpdateCheckPath)
	if err != nil {
		log.Printf("Failed to read last update check %s", err)
		return nil
	}
	unixTime, err := strconv.Atoi(string(data))
	if err != nil {
		log.Printf("Failed to parse last update check %s", err)
		return nil
	}
	lastUpdate := time.Unix(int64(unixTime), 0)
	return &lastUpdate
}

// GetSession - Get session by session ID or name
func (con *KonjiyamiConsoleClient) GetSession(arg string) *clientpb.Session {
	sessions, err := con.Rpc.GetSessions(context.Background(), &commonpb.Empty{})
	if err != nil {
		con.PrintWarnf("%s\n", err)
		return nil
	}
	for _, session := range sessions.GetSessions() {
		if session.Name == arg || strings.HasPrefix(session.ID, arg) {
			return session
		}
	}
	return nil
}

// GetSessionsByName - Return all sessions for an Implant by name
func (con *KonjiyamiConsoleClient) GetSessionsByName(name string) []*clientpb.Session {
	sessions, err := con.Rpc.GetSessions(context.Background(), &commonpb.Empty{})
	if err != nil {
		fmt.Printf(Warn+"%s\n", err)
		return nil
	}
	matched := []*clientpb.Session{}
	for _, session := range sessions.GetSessions() {
		if session.Name == name {
			matched = append(matched, session)
		}
	}
	return matched
}

// GetActiveSessionConfig - Get the active sessions's config
// TODO: Switch to query config based on ConfigID
func (con *KonjiyamiConsoleClient) GetActiveSessionConfig() *clientpb.ImplantConfig {
	session := con.ActiveTarget.GetSession()
	if session == nil {
		return nil
	}
	c2s := []*clientpb.ImplantC2{}
	c2s = append(c2s, &clientpb.ImplantC2{
		URL:      session.GetActiveC2(),
		Priority: uint32(0),
	})
	config := &clientpb.ImplantConfig{
		Name:    session.GetName(),
		GOOS:    session.GetOS(),
		GOARCH:  session.GetArch(),
		Debug:   true,
		Evasion: session.GetEvasion(),

		MaxConnectionErrors: uint32(1000),
		ReconnectInterval:   int64(60),
		Format:              clientpb.OutputFormat_SHELLCODE,
		IsSharedLib:         true,
		C2:                  c2s,
	}
	return config
}

// PrintAsyncResponse - Print the generic async response information
func (con *KonjiyamiConsoleClient) PrintAsyncResponse(resp *commonpb.Response) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	beacon, err := con.Rpc.GetBeacon(ctx, &clientpb.Beacon{ID: resp.BeaconID})
	if err != nil {
		fmt.Printf(Warn+"%s\n", err)
		return
	}
	con.PrintInfof("Tasked beacon %s (%s)\n", beacon.Name, strings.Split(resp.TaskID, "-")[0])
}

func (con *KonjiyamiConsoleClient) Printf(format string, args ...interface{}) (n int, err error) {
	return fmt.Fprintf(con.App.Stdout(), format, args...)
}

func (con *KonjiyamiConsoleClient) Println(args ...interface{}) (n int, err error) {
	return fmt.Fprintln(con.App.Stdout(), args...)
}

func (con *KonjiyamiConsoleClient) PrintInfof(format string, args ...interface{}) (n int, err error) {
	return fmt.Fprintf(con.App.Stdout(), Clearln+Info+format, args...)
}

func (con *KonjiyamiConsoleClient) PrintSuccessf(format string, args ...interface{}) (n int, err error) {
	return fmt.Fprintf(con.App.Stdout(), Clearln+Success+format, args...)
}

func (con *KonjiyamiConsoleClient) PrintWarnf(format string, args ...interface{}) (n int, err error) {
	return fmt.Fprintf(con.App.Stdout(), Clearln+"⚠️  "+Normal+format, args...)
}

func (con *KonjiyamiConsoleClient) PrintErrorf(format string, args ...interface{}) (n int, err error) {
	return fmt.Fprintf(con.App.Stderr(), Clearln+Warn+format, args...)
}

func (con *KonjiyamiConsoleClient) PrintEventInfof(format string, args ...interface{}) (n int, err error) {
	return fmt.Fprintf(con.App.Stdout(), Clearln+Info+format+"\n"+Clearln+"\r\n"+Clearln+"\r", args...)
}

func (con *KonjiyamiConsoleClient) PrintEventErrorf(format string, args ...interface{}) (n int, err error) {
	return fmt.Fprintf(con.App.Stderr(), Clearln+Warn+format+"\n"+Clearln+"\r\n"+Clearln+"\r", args...)
}

func (con *KonjiyamiConsoleClient) PrintEventSuccessf(format string, args ...interface{}) (n int, err error) {
	return fmt.Fprintf(con.App.Stdout(), Clearln+Success+format+"\n"+Clearln+"\r\n"+Clearln+"\r", args...)
}

func (con *KonjiyamiConsoleClient) SpinUntil(message string, ctrl chan bool) {
	go spin.Until(con.App.Stdout(), message, ctrl)
}

// FormatDateDelta - Generate formatted date string of the time delta between then and now
func (con *KonjiyamiConsoleClient) FormatDateDelta(t time.Time, includeDate bool, color bool) string {
	nextTime := t.Format(time.UnixDate)

	var interval string

	if t.Before(time.Now()) {
		if includeDate {
			interval = fmt.Sprintf("%s (%s ago)", nextTime, time.Since(t).Round(time.Second))
		} else {
			interval = time.Since(t).Round(time.Second).String()
		}
		if color {
			interval = fmt.Sprintf("%s%s%s", Bold+Red, interval, Normal)
		}
	} else {
		if includeDate {
			interval = fmt.Sprintf("%s (in %s)", nextTime, time.Until(t).Round(time.Second))
		} else {
			interval = time.Until(t).Round(time.Second).String()
		}
		if color {
			interval = fmt.Sprintf("%s%s%s", Bold+Green, interval, Normal)
		}
	}
	return interval
}

//
// -------------------------- [ Active Target ] --------------------------
//

// GetSessionInteractive - Get the active target(s)
func (s *ActiveTarget) GetInteractive() (*clientpb.Session, *clientpb.Beacon) {
	if s.session == nil && s.beacon == nil {
		fmt.Printf(Warn + "Please select a session or beacon via `use`\n")
		return nil, nil
	}
	return s.session, s.beacon
}

// GetSessionInteractive - Get the active target(s)
func (s *ActiveTarget) Get() (*clientpb.Session, *clientpb.Beacon) {
	return s.session, s.beacon
}

// GetSessionInteractive - GetSessionInteractive the active session
func (s *ActiveTarget) GetSessionInteractive() *clientpb.Session {
	if s.session == nil {
		fmt.Printf(Warn + "Please select a session via `use`\n")
		return nil
	}
	return s.session
}

// GetSession - Same as GetSession() but doesn't print a warning
func (s *ActiveTarget) GetSession() *clientpb.Session {
	return s.session
}

// GetBeaconInteractive - Get beacon interactive the active session
func (s *ActiveTarget) GetBeaconInteractive() *clientpb.Beacon {
	if s.beacon == nil {
		fmt.Printf(Warn + "Please select a beacon via `use`\n")
		return nil
	}
	return s.beacon
}

// GetBeacon - Same as GetBeacon() but doesn't print a warning
func (s *ActiveTarget) GetBeacon() *clientpb.Beacon {
	return s.beacon
}

// IsSession - Is the current target a session?
func (s *ActiveTarget) IsSession() bool {
	return s.session != nil
}

// AddObserver - Observers to notify when the active session changes
func (s *ActiveTarget) AddObserver(observer Observer) int {
	s.observerID++
	s.observers[s.observerID] = observer
	return s.observerID
}

func (s *ActiveTarget) RemoveObserver(observerID int) {
	delete(s.observers, observerID)
}

func (s *ActiveTarget) Request(ctx *grumble.Context) *commonpb.Request {
	if s.session == nil && s.beacon == nil {
		return nil
	}
	timeout := int(time.Second) * ctx.Flags.Int("timeout")
	req := &commonpb.Request{}
	req.Timeout = int64(timeout)
	if s.session != nil {
		req.Async = false
		req.SessionID = s.session.ID
	}
	if s.beacon != nil {
		req.Async = true
		req.BeaconID = s.beacon.ID
	}
	return req
}

// Set - Change the active session
func (s *ActiveTarget) Set(session *clientpb.Session, beacon *clientpb.Beacon) {
	if session != nil && beacon != nil {
		panic("cannot set both an active beacon and an active session")
	}
	if session == nil && beacon == nil {
		s.session = nil
		s.beacon = nil
		for _, observer := range s.observers {
			observer(s.session, s.beacon)
		}
		return
	}

	if session != nil {
		s.session = session
		s.beacon = nil
		for _, observer := range s.observers {
			observer(s.session, s.beacon)
		}
	} else if beacon != nil {
		s.beacon = beacon
		s.session = nil
		for _, observer := range s.observers {
			observer(s.session, s.beacon)
		}
	}
}

// Background - Background the active session
func (s *ActiveTarget) Background() {
	s.session = nil
	s.beacon = nil
	for _, observer := range s.observers {
		observer(nil, nil)
	}
}

// Legacy variables removed - no longer displaying ASCII logos or random abilities

