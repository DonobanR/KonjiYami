package rpc

/*
	KONJIYAMI Implant Framework
	Copyright (C) 2020  Bishop Fox

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

// Execute - Execute a remote process
func (rpc *Server) Execute(ctx context.Context, req *konjiyamipb.ExecuteReq) (*konjiyamipb.Execute, error) {
	resp := &konjiyamipb.Execute{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// ExecuteWindows - Execute a remote process with specific options (PPID, Token, windows only)
func (rpc *Server) ExecuteWindows(ctx context.Context, req *konjiyamipb.ExecuteWindowsReq) (*konjiyamipb.Execute, error) {
	resp := &konjiyamipb.Execute{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

