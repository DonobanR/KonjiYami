package tasks

/*
	KONJIYAMI Implant Framework
	Copyright (C) 2021  Bishop Fox

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
	"io/ioutil"
	"os"
	"strings"
	"time"

	"github.com/AlecAivazis/survey/v2"
	"konjiyami/client/command/environment"
	"konjiyami/client/command/exec"
	"konjiyami/client/command/extensions"
	"konjiyami/client/command/filesystem"
	"konjiyami/client/command/network"
	"konjiyami/client/command/privilege"
	"konjiyami/client/command/processes"
	"konjiyami/client/command/registry"
	"konjiyami/client/command/settings"
	"konjiyami/client/console"
	"konjiyami/protobuf/clientpb"
	"konjiyami/protobuf/konjiyamipb"
	"konjiyami/util"
	"github.com/desertbit/grumble"
	"github.com/jedib0t/go-pretty/v6/table"
	"google.golang.org/protobuf/proto"
)

// TasksFetchCmd - Manage beacon tasks
func TasksFetchCmd(ctx *grumble.Context, con *console.KonjiyamiConsoleClient) {
	beacon := con.ActiveTarget.GetBeaconInteractive()
	if beacon == nil {
		return
	}
	beaconTasks, err := con.Rpc.GetBeaconTasks(context.Background(), &clientpb.Beacon{ID: beacon.ID})
	if err != nil {
		con.PrintErrorf("%s\n", err)
		return
	}
	tasks := beaconTasks.Tasks
	if len(tasks) == 0 {
		con.PrintErrorf("No tasks for beacon\n")
		return
	}

	idArg := ctx.Args.String("id")
	if idArg != "" {
		tasks = filterTasksByID(idArg, tasks)
		if len(tasks) == 0 {
			con.PrintErrorf("No beacon task found with id %s\n", idArg)
			return
		}
	}

	filter := ctx.Flags.String("filter")
	if filter != "" {
		tasks = filterTasksByTaskType(filter, tasks)
		if len(tasks) == 0 {
			con.PrintErrorf("No beacon tasks with filter type '%s'\n", filter)
			return
		}
	}

	var task *clientpb.BeaconTask
	if 1 < len(tasks) {
		task, err = SelectBeaconTask(tasks)
		if err != nil {
			con.PrintErrorf("%s\n", err)
			return
		}
		con.Printf(console.UpN+console.Clearln, 1)
	} else {
		task = tasks[0]
	}
	task, err = con.Rpc.GetBeaconTaskContent(context.Background(), &clientpb.BeaconTask{ID: task.ID})
	if err != nil {
		con.PrintErrorf("Failed to fetch task content: %s\n", err)
		return
	}
	PrintTask(task, con)
}

func filterTasksByID(taskID string, tasks []*clientpb.BeaconTask) []*clientpb.BeaconTask {
	filteredTasks := []*clientpb.BeaconTask{}
	for _, task := range tasks {
		if strings.HasPrefix(task.ID, strings.ToLower(taskID)) {
			filteredTasks = append(filteredTasks, task)
		}
	}
	return filteredTasks
}

func filterTasksByTaskType(taskType string, tasks []*clientpb.BeaconTask) []*clientpb.BeaconTask {
	filteredTasks := []*clientpb.BeaconTask{}
	for _, task := range tasks {
		if strings.HasPrefix(strings.ToLower(task.Description), strings.ToLower(taskType)) {
			filteredTasks = append(filteredTasks, task)
		}
	}
	return filteredTasks
}

// PrintTask - Print the details of a beacon task
func PrintTask(task *clientpb.BeaconTask, con *console.KonjiyamiConsoleClient) {
	tw := table.NewWriter()
	tw.SetStyle(settings.GetTableWithBordersStyle(con))
	tw.AppendRow(table.Row{console.Bold + "Beacon Task" + console.Normal, task.ID})
	tw.AppendSeparator()
	tw.AppendRow(table.Row{"State", emojiState(task.State) + " " + prettyState(strings.Title(task.State))})
	tw.AppendRow(table.Row{"Description", task.Description})
	tw.AppendRow(table.Row{"Created", time.Unix(task.CreatedAt, 0).Format(time.RFC1123)})
	if !time.Unix(task.SentAt, 0).IsZero() {
		tw.AppendRow(table.Row{"Sent", time.Unix(task.SentAt, 0).Format(time.RFC1123)})
	}
	if !time.Unix(task.CompletedAt, 0).IsZero() {
		tw.AppendRow(table.Row{"Completed", time.Unix(task.CompletedAt, 0).Format(time.RFC1123)})
	}

	tw.AppendRow(table.Row{"Request Size", util.ByteCountBinary(int64(len(task.Request)))})
	if !time.Unix(task.CompletedAt, 0).IsZero() {
		tw.AppendRow(table.Row{"Response Size", util.ByteCountBinary(int64(len(task.Response)))})
	}
	tw.AppendSeparator()
	con.Printf("%s\n", tw.Render())
	if !time.Unix(task.CompletedAt, 0).IsZero() {
		con.Println()
		if 0 < len(task.Response) {
			renderTaskResponse(task, con)
		} else {
			con.PrintInfof("No task response\n")
		}
	}
}

func emojiState(state string) string {
	switch strings.ToLower(state) {
	case "completed":
		return "✅"
	case "pending":
		return "⏳"
	case "failed":
		return "❌"
	case "canceled":
		return "🚫"
	default:
		return "❓"
	}
}

// Decode and render message specific content
func renderTaskResponse(task *clientpb.BeaconTask, con *console.KonjiyamiConsoleClient) {
	reqEnvelope := &konjiyamipb.Envelope{}
	proto.Unmarshal(task.Request, reqEnvelope)
	switch reqEnvelope.Type {

	// ---------------------
	// Environment commands
	// ---------------------
	case konjiyamipb.MsgEnvReq:
		envInfo := &konjiyamipb.EnvInfo{}
		err := proto.Unmarshal(task.Response, envInfo)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		environment.PrintGetEnvInfo(envInfo, con)

	case konjiyamipb.MsgSetEnvReq:
		setEnvReq := &konjiyamipb.SetEnvReq{}
		err := proto.Unmarshal(task.Request, setEnvReq)
		if err != nil {
			con.PrintErrorf("Failed to decode task request: %s\n", err)
			return
		}
		setEnv := &konjiyamipb.SetEnv{}
		err = proto.Unmarshal(task.Response, setEnv)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		environment.PrintSetEnvInfo(setEnvReq.Variable.Key, setEnvReq.Variable.Value, setEnv, con)

	case konjiyamipb.MsgUnsetEnvReq:
		unsetEnvReq := &konjiyamipb.UnsetEnvReq{}
		err := proto.Unmarshal(task.Request, unsetEnvReq)
		if err != nil {
			con.PrintErrorf("Failed to decode task request: %s\n", err)
			return
		}
		unsetEnv := &konjiyamipb.UnsetEnv{}
		err = proto.Unmarshal(task.Response, unsetEnv)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		environment.PrintUnsetEnvInfo(unsetEnvReq.Name, unsetEnv, con)

	// ---------------------
	// Call extension commands
	// ---------------------
	case konjiyamipb.MsgCallExtensionReq:
		callExtension := &konjiyamipb.CallExtension{}
		err := proto.Unmarshal(task.Response, callExtension)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		extensions.PrintExtOutput("", "", callExtension, con)

	// ---------------------
	// Exec commands
	// ---------------------
	case konjiyamipb.MsgInvokeExecuteAssemblyReq:
		fallthrough
	case konjiyamipb.MsgInvokeInProcExecuteAssemblyReq:
		fallthrough
	case konjiyamipb.MsgExecuteAssemblyReq:
		execAssembly := &konjiyamipb.ExecuteAssembly{}
		err := proto.Unmarshal(task.Response, execAssembly)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		beacon, _ := con.Rpc.GetBeacon(context.Background(), &clientpb.Beacon{ID: task.BeaconID})
		hostname := "hostname"
		if beacon != nil {
			hostname = beacon.Hostname
		}
		assemblyPath := ""
		ctx := &grumble.Context{
			Command: &grumble.Command{Name: "execute-assembly"},
			Flags: grumble.FlagMap{
				"save": &grumble.FlagMapItem{Value: false, IsDefault: true},
				"loot": &grumble.FlagMapItem{Value: false, IsDefault: true},
				"name": &grumble.FlagMapItem{Value: "", IsDefault: true},
			},
		}
		exec.HandleExecuteAssemblyResponse(execAssembly, assemblyPath, hostname, ctx, con)

	// execute-shellcode
	case konjiyamipb.MsgTaskReq:
		shellcodeExec := &konjiyamipb.Task{}
		err := proto.Unmarshal(task.Response, shellcodeExec)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		exec.PrintExecuteShellcode(shellcodeExec, con)

	case konjiyamipb.MsgExecuteReq:
		execReq := &konjiyamipb.ExecuteReq{}
		err := proto.Unmarshal(reqEnvelope.Data, execReq)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		execResult := &konjiyamipb.Execute{}
		err = proto.Unmarshal(task.Response, execResult)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		ctx := &grumble.Context{
			Flags: grumble.FlagMap{
				"ignore-stderr": &grumble.FlagMapItem{Value: false},
				"loot":          &grumble.FlagMapItem{Value: false},
				"stdout":        &grumble.FlagMapItem{Value: ""},
				"stderr":        &grumble.FlagMapItem{Value: ""},
				"output":        &grumble.FlagMapItem{Value: true},
			},
			Args: grumble.ArgMap{
				"command":   &grumble.ArgMapItem{Value: execReq.Path},
				"arguments": &grumble.ArgMapItem{Value: execReq.Args},
			},
		}
		exec.PrintExecute(execResult, ctx, con)

	case konjiyamipb.MsgSideloadReq:
		sideload := &konjiyamipb.Sideload{}
		err := proto.Unmarshal(task.Response, sideload)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		beacon, _ := con.Rpc.GetBeacon(context.Background(), &clientpb.Beacon{ID: task.BeaconID})
		hostname := "hostname"
		if beacon != nil {
			hostname = beacon.Hostname
		}
		ctx := &grumble.Context{
			Command: &grumble.Command{Name: "sideload"},
			Flags: grumble.FlagMap{
				"save": &grumble.FlagMapItem{Value: false},
				"loot": &grumble.FlagMapItem{Value: false},
			},
		}
		exec.HandleSideloadResponse(sideload, "", hostname, ctx, con)

	case konjiyamipb.MsgSpawnDllReq:
		spawnDll := &konjiyamipb.SpawnDll{}
		err := proto.Unmarshal(task.Response, spawnDll)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		beacon, _ := con.Rpc.GetBeacon(context.Background(), &clientpb.Beacon{ID: task.BeaconID})
		hostname := "hostname"
		if beacon != nil {
			hostname = beacon.Hostname
		}
		ctx := &grumble.Context{
			Command: &grumble.Command{Name: "spawndll"},
			Flags: grumble.FlagMap{
				"save": &grumble.FlagMapItem{Value: false},
				"loot": &grumble.FlagMapItem{Value: false},
			},
		}
		exec.HandleSpawnDLLResponse(spawnDll, "", hostname, ctx, con)

	case konjiyamipb.MsgSSHCommandReq:
		sshCommand := &konjiyamipb.SSHCommand{}
		err := proto.Unmarshal(task.Response, sshCommand)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		exec.PrintSSHCmd(sshCommand, con)

	// ---------------------
	// File system commands
	// ---------------------
	// Cat = download
	case konjiyamipb.MsgCdReq:
		pwd := &konjiyamipb.Pwd{}
		err := proto.Unmarshal(task.Response, pwd)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		filesystem.PrintPwd(pwd, con)

	case konjiyamipb.MsgDownloadReq:
		download := &konjiyamipb.Download{}
		err := proto.Unmarshal(task.Response, download)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		taskResponseDownload(download, con)

	case konjiyamipb.MsgLsReq:
		ls := &konjiyamipb.Ls{}
		err := proto.Unmarshal(task.Response, ls)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		flags := grumble.FlagMap{
			"reverse":  &grumble.FlagMapItem{Value: false},
			"modified": &grumble.FlagMapItem{Value: false},
			"size":     &grumble.FlagMapItem{Value: false},
		}
		filesystem.PrintLs(ls, flags, con)

	case konjiyamipb.MsgMvReq:
		mv := &konjiyamipb.Mv{}
		err := proto.Unmarshal(task.Response, mv)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}

	case konjiyamipb.MsgMkdirReq:
		mkdir := &konjiyamipb.Mkdir{}
		err := proto.Unmarshal(task.Response, mkdir)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		filesystem.PrintMkdir(mkdir, con)

	case konjiyamipb.MsgPwdReq:
		pwd := &konjiyamipb.Pwd{}
		err := proto.Unmarshal(task.Response, pwd)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		filesystem.PrintPwd(pwd, con)

	case konjiyamipb.MsgRmReq:
		rm := &konjiyamipb.Rm{}
		err := proto.Unmarshal(task.Response, rm)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		filesystem.PrintRm(rm, con)

	case konjiyamipb.MsgUploadReq:
		upload := &konjiyamipb.Upload{}
		err := proto.Unmarshal(task.Response, upload)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		filesystem.PrintUpload(upload, con)

	case konjiyamipb.MsgChmodReq:
		chmod := &konjiyamipb.Chmod{}
		err := proto.Unmarshal(task.Response, chmod)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		filesystem.PrintChmod(chmod, con)

	case konjiyamipb.MsgChownReq:
		chown := &konjiyamipb.Chown{}
		err := proto.Unmarshal(task.Response, chown)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		filesystem.PrintChown(chown, con)

	case konjiyamipb.MsgChtimesReq:
		chtimes := &konjiyamipb.Chtimes{}
		err := proto.Unmarshal(task.Response, chtimes)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		filesystem.PrintChtimes(chtimes, con)

	case konjiyamipb.MsgMemfilesListReq:
		memfilesList := &konjiyamipb.Ls{}
		err := proto.Unmarshal(task.Response, memfilesList)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		filesystem.PrintMemfiles(memfilesList, con)

	case konjiyamipb.MsgMemfilesAddReq:
		memfilesAdd := &konjiyamipb.MemfilesAdd{}
		err := proto.Unmarshal(task.Response, memfilesAdd)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		filesystem.PrintAddMemfile(memfilesAdd, con)

	case konjiyamipb.MsgMemfilesRmReq:
		memfilesRm := &konjiyamipb.MemfilesRm{}
		err := proto.Unmarshal(task.Response, memfilesRm)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		filesystem.PrintRmMemfile(memfilesRm, con)

	// ---------------------
	// Network commands
	// ---------------------
	case konjiyamipb.MsgIfconfigReq:
		ifconfig := &konjiyamipb.Ifconfig{}
		err := proto.Unmarshal(task.Response, ifconfig)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		network.PrintIfconfig(ifconfig, true, con)

	case konjiyamipb.MsgNetstatReq:
		netstat := &konjiyamipb.Netstat{}
		err := proto.Unmarshal(task.Response, netstat)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		beacon, err := con.Rpc.GetBeacon(context.Background(), &clientpb.Beacon{ID: task.BeaconID})
		if err != nil {
			con.PrintErrorf("Failed to fetch beacon: %s\n", err)
			return
		}
		network.PrintNetstat(netstat, beacon.PID, beacon.ActiveC2, false, con)

	// ---------------------
	// Privilege commands
	// ---------------------
	case konjiyamipb.MsgGetPrivsReq:
		privs := &konjiyamipb.GetPrivs{}
		err := proto.Unmarshal(task.Response, privs)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		beacon, err := con.Rpc.GetBeacon(context.Background(), &clientpb.Beacon{ID: task.BeaconID})
		if err != nil {
			con.PrintErrorf("Failed to fetch beacon: %s\n", err)
			return
		}
		privilege.PrintGetPrivs(privs, beacon.PID, con)

	case konjiyamipb.MsgInvokeGetSystemReq:
		getSystem := &konjiyamipb.GetSystem{}
		err := proto.Unmarshal(task.Response, getSystem)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		privilege.PrintGetSystem(getSystem, con)

	case konjiyamipb.MsgCurrentTokenOwnerReq:
		cto := &konjiyamipb.CurrentTokenOwner{}
		err := proto.Unmarshal(task.Response, cto)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}

	case konjiyamipb.MsgImpersonateReq:
		impersonateReq := &konjiyamipb.ImpersonateReq{}
		err := proto.Unmarshal(task.Response, impersonateReq)
		if err != nil {
			con.PrintErrorf("Failed to decode task request: %s\n", err)
			return
		}
		impersonate := &konjiyamipb.Impersonate{}
		err = proto.Unmarshal(task.Response, impersonate)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		privilege.PrintImpersonate(impersonate, impersonateReq.Username, con)

	case konjiyamipb.MsgMakeTokenReq:
		makeTokenReq := &konjiyamipb.MakeTokenReq{}
		err := proto.Unmarshal(task.Response, makeTokenReq)
		if err != nil {
			con.PrintErrorf("Failed to decode task request: %s\n", err)
			return
		}
		makeToken := &konjiyamipb.MakeToken{}
		err = proto.Unmarshal(task.Response, makeToken)
		if err != nil {
			con.PrintErrorf("Failed to decode task request: %s\n", err)
			return
		}
		privilege.PrintMakeToken(makeToken, makeTokenReq.Domain, makeTokenReq.Username, con)

	case konjiyamipb.MsgRunAsReq:
		runAsReq := &konjiyamipb.RunAsReq{}
		err := proto.Unmarshal(task.Response, runAsReq)
		if err != nil {
			con.PrintErrorf("Failed to decode task request: %s\n", err)
			return
		}
		runAs := &konjiyamipb.RunAs{}
		err = proto.Unmarshal(task.Response, runAs)
		if err != nil {
			con.PrintErrorf("Failed to decode task request: %s\n", err)
			return
		}
		beacon, err := con.Rpc.GetBeacon(context.Background(), &clientpb.Beacon{ID: task.BeaconID})
		if err != nil {
			con.PrintErrorf("Failed to fetch beacon: %s\n", err)
			return
		}
		privilege.PrintRunAs(runAs, runAsReq.ProcessName, runAsReq.Args, beacon.Name, con)

	// ---------------------
	// Processes commands
	// ---------------------
	case konjiyamipb.MsgProcessDumpReq:
		dump := &konjiyamipb.ProcessDump{}
		err := proto.Unmarshal(task.Response, dump)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		promptSaveToFile(dump.Data, con)

	case konjiyamipb.MsgPsReq:
		ps := &konjiyamipb.Ps{}
		err := proto.Unmarshal(task.Response, ps)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		beacon, err := con.Rpc.GetBeacon(context.Background(), &clientpb.Beacon{ID: task.BeaconID})
		if err != nil {
			con.PrintErrorf("Failed to get beacon: %s\n", err)
			return
		}
		ctx := &grumble.Context{
			Flags: grumble.FlagMap{
				"pid":           &grumble.FlagMapItem{Value: -1},
				"exe":           &grumble.FlagMapItem{Value: ""},
				"owner":         &grumble.FlagMapItem{Value: ""},
				"overflow":      &grumble.FlagMapItem{Value: false},
				"skip-pages":    &grumble.FlagMapItem{Value: 0},
				"print-cmdline": &grumble.FlagMapItem{Value: true},
				"tree":          &grumble.FlagMapItem{Value: false},
			},
		}
		processes.PrintPS(beacon.OS, ps, true, ctx, con)

	case konjiyamipb.MsgTerminateReq:
		terminate := &konjiyamipb.Terminate{}
		err := proto.Unmarshal(task.Response, terminate)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		processes.PrintTerminate(terminate, con)

	// ---------------------
	// Registry commands
	// ---------------------
	case konjiyamipb.MsgRegistryCreateKeyReq:
		createKeyReq := &konjiyamipb.RegistryCreateKeyReq{}
		err := proto.Unmarshal(task.Request, createKeyReq)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		createKey := &konjiyamipb.RegistryCreateKey{}
		err = proto.Unmarshal(task.Response, createKey)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		registry.PrintCreateKey(createKey, createKeyReq.Path, createKeyReq.Key, con)

	case konjiyamipb.MsgRegistryDeleteKeyReq:
		deleteKeyReq := &konjiyamipb.RegistryDeleteKeyReq{}
		err := proto.Unmarshal(task.Request, deleteKeyReq)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		deleteKey := &konjiyamipb.RegistryDeleteKey{}
		err = proto.Unmarshal(task.Response, deleteKey)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		registry.PrintDeleteKey(deleteKey, deleteKeyReq.Path, deleteKeyReq.Key, con)

	case konjiyamipb.MsgRegistryListValuesReq:
		listValuesReq := &konjiyamipb.RegistryListValuesReq{}
		err := proto.Unmarshal(task.Request, listValuesReq)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		regList := &konjiyamipb.RegistryValuesList{}
		err = proto.Unmarshal(task.Response, regList)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		registry.PrintListValues(regList, listValuesReq.Hive, listValuesReq.Path, con)

	case konjiyamipb.MsgRegistrySubKeysListReq:
		listValuesReq := &konjiyamipb.RegistrySubKeyListReq{}
		err := proto.Unmarshal(task.Request, listValuesReq)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		regList := &konjiyamipb.RegistrySubKeyList{}
		err = proto.Unmarshal(task.Response, regList)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		registry.PrintListSubKeys(regList, listValuesReq.Hive, listValuesReq.Path, con)

	case konjiyamipb.MsgRegistryReadReq:
		regRead := &konjiyamipb.RegistryRead{}
		err := proto.Unmarshal(task.Response, regRead)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		registry.PrintRegRead(regRead, con)

	case konjiyamipb.MsgRegistryWriteReq:
		regWrite := &konjiyamipb.RegistryWrite{}
		err := proto.Unmarshal(task.Response, regWrite)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		registry.PrintRegWrite(regWrite, con)

	// ---------------------
	// Screenshot
	// ---------------------
	case konjiyamipb.MsgScreenshotReq:
		screenshot := &konjiyamipb.Screenshot{}
		err := proto.Unmarshal(task.Response, screenshot)
		if err != nil {
			con.PrintErrorf("Failed to decode task response: %s\n", err)
			return
		}
		promptSaveToFile(screenshot.Data, con)

	// ---------------------
	// Default
	// ---------------------
	default:
		con.PrintErrorf("Cannot render task response for msg type %v\n", reqEnvelope.Type)
	}
}

func taskResponseDownload(download *konjiyamipb.Download, con *console.KonjiyamiConsoleClient) {
	const (
		dump   = "Dump Contents"
		saveTo = "Save to File ..."
	)
	action := saveTo
	prompt := &survey.Select{
		Message: "Choose an option:",
		Options: []string{dump, saveTo},
	}
	err := survey.AskOne(prompt, &action, survey.WithValidator(survey.Required))
	if err != nil {
		con.PrintErrorf("%s\n", err)
		return
	}
	switch action {
	case dump:
		con.Printf("%s\n", string(download.Data))
	default:
		promptSaveToFile(download.Data, con)
	}
}

func promptSaveToFile(data []byte, con *console.KonjiyamiConsoleClient) {
	saveTo := ""
	saveToPrompt := &survey.Input{Message: "Save to: "}
	err := survey.AskOne(saveToPrompt, &saveTo)
	if err != nil {
		con.PrintErrorf("%s\n", err)
		return
	}
	if _, err := os.Stat(saveTo); !os.IsNotExist(err) {
		confirm := false
		prompt := &survey.Confirm{Message: "Overwrite existing file?"}
		survey.AskOne(prompt, &confirm)
		if !confirm {
			return
		}
	}
	err = ioutil.WriteFile(saveTo, data, 0600)
	if err != nil {
		con.PrintErrorf("Failed to save file: %s\n", err)
		return
	}
	con.PrintInfof("Wrote %d byte(s) to %s", len(data), saveTo)
}

