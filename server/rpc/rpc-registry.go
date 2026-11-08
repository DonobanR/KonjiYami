package rpc

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

	"konjiyami/protobuf/commonpb"
	"konjiyami/protobuf/konjiyamipb"
)

// RegistryRead - gRPC interface to read a registry key from a session
func (rpc *Server) RegistryRead(ctx context.Context, req *konjiyamipb.RegistryReadReq) (*konjiyamipb.RegistryRead, error) {
	resp := &konjiyamipb.RegistryRead{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// RegistryWrite - gRPC interface to write to a registry key on a session
func (rpc *Server) RegistryWrite(ctx context.Context, req *konjiyamipb.RegistryWriteReq) (*konjiyamipb.RegistryWrite, error) {
	resp := &konjiyamipb.RegistryWrite{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// RegistryCreateKey - gRPC interface to create a registry key on a session
func (rpc *Server) RegistryCreateKey(ctx context.Context, req *konjiyamipb.RegistryCreateKeyReq) (*konjiyamipb.RegistryCreateKey, error) {
	resp := &konjiyamipb.RegistryCreateKey{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// RegistryDeleteKey - gRPC interface to delete a registry key on a session
func (rpc *Server) RegistryDeleteKey(ctx context.Context, req *konjiyamipb.RegistryDeleteKeyReq) (*konjiyamipb.RegistryDeleteKey, error) {
	resp := &konjiyamipb.RegistryDeleteKey{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// RegistryListSubKeys - gRPC interface to list the sub keys of a registry key
func (rpc *Server) RegistryListSubKeys(ctx context.Context, req *konjiyamipb.RegistrySubKeyListReq) (*konjiyamipb.RegistrySubKeyList, error) {
	resp := &konjiyamipb.RegistrySubKeyList{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// RegistryListSubKeys - gRPC interface to list the sub keys of a registry key
func (rpc *Server) RegistryListValues(ctx context.Context, req *konjiyamipb.RegistryListValuesReq) (*konjiyamipb.RegistryValuesList, error) {
	resp := &konjiyamipb.RegistryValuesList{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

