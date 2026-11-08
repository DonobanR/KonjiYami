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
	"crypto/sha256"
	"fmt"

	"konjiyami/protobuf/commonpb"
	"konjiyami/protobuf/konjiyamipb"
	"konjiyami/server/core"
	"konjiyami/server/db"
	"konjiyami/server/db/models"
	"konjiyami/server/log"
)

var (
	fsLog = log.NamedLogger("rcp", "fs")
)

// Ls - List a directory
func (rpc *Server) Ls(ctx context.Context, req *konjiyamipb.LsReq) (*konjiyamipb.Ls, error) {
	resp := &konjiyamipb.Ls{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Mv - Move or rename a file
func (rpc *Server) Mv(ctx context.Context, req *konjiyamipb.MvReq) (*konjiyamipb.Mv, error) {
	resp := &konjiyamipb.Mv{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Rm - Remove file or directory
func (rpc *Server) Rm(ctx context.Context, req *konjiyamipb.RmReq) (*konjiyamipb.Rm, error) {
	resp := &konjiyamipb.Rm{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Mkdir - Make a directory
func (rpc *Server) Mkdir(ctx context.Context, req *konjiyamipb.MkdirReq) (*konjiyamipb.Mkdir, error) {
	resp := &konjiyamipb.Mkdir{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Cd - Change directory
func (rpc *Server) Cd(ctx context.Context, req *konjiyamipb.CdReq) (*konjiyamipb.Pwd, error) {
	resp := &konjiyamipb.Pwd{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Pwd - Print working directory
func (rpc *Server) Pwd(ctx context.Context, req *konjiyamipb.PwdReq) (*konjiyamipb.Pwd, error) {
	resp := &konjiyamipb.Pwd{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Download - Download a file from the remote file system
func (rpc *Server) Download(ctx context.Context, req *konjiyamipb.DownloadReq) (*konjiyamipb.Download, error) {
	resp := &konjiyamipb.Download{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Upload - Upload a file from the remote file system
func (rpc *Server) Upload(ctx context.Context, req *konjiyamipb.UploadReq) (*konjiyamipb.Upload, error) {
	resp := &konjiyamipb.Upload{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	if req.IsIOC {
		go trackIOC(req, resp)
	}
	return resp, nil
}

// Chmod - Change permission on a file or directory
func (rpc *Server) Chmod(ctx context.Context, req *konjiyamipb.ChmodReq) (*konjiyamipb.Chmod, error) {
	resp := &konjiyamipb.Chmod{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Chown - Change owner on a file or directory
func (rpc *Server) Chown(ctx context.Context, req *konjiyamipb.ChownReq) (*konjiyamipb.Chown, error) {
	resp := &konjiyamipb.Chown{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Chtimes - Change file access and modification times on a file or directory
func (rpc *Server) Chtimes(ctx context.Context, req *konjiyamipb.ChtimesReq) (*konjiyamipb.Chtimes, error) {
	resp := &konjiyamipb.Chtimes{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// MemfilesList - List memfiles
func (rpc *Server) MemfilesList(ctx context.Context, req *konjiyamipb.MemfilesListReq) (*konjiyamipb.Ls, error) {
	resp := &konjiyamipb.Ls{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// MemfilesAdd - Add memfile
func (rpc *Server) MemfilesAdd(ctx context.Context, req *konjiyamipb.MemfilesAddReq) (*konjiyamipb.MemfilesAdd, error) {
	resp := &konjiyamipb.MemfilesAdd{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// MemfilesRm - Close memfile
func (rpc *Server) MemfilesRm(ctx context.Context, req *konjiyamipb.MemfilesRmReq) (*konjiyamipb.MemfilesRm, error) {
	resp := &konjiyamipb.MemfilesRm{Response: &commonpb.Response{}}
	err := rpc.GenericHandler(req, resp)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func trackIOC(req *konjiyamipb.UploadReq, resp *konjiyamipb.Upload) {
	fsLog.Debugf("Adding IOC to database ...")
	request := req.GetRequest()
	if request == nil {
		fsLog.Error("No request for upload")
		return
	}
	session := core.Sessions.Get(request.SessionID)
	if session == nil {
		fsLog.Error("No session for upload request")
		return
	}
	host, err := db.HostByHostUUID(session.UUID)
	if err != nil {
		fsLog.Errorf("No host for session uuid %v", session.UUID)
		return
	}

	sum := sha256.Sum256(req.Data)
	ioc := &models.IOC{
		HostID:   host.ID,
		Path:     resp.Path,
		FileHash: fmt.Sprintf("%x", sum),
	}
	if db.Session().Create(ioc).Error != nil {
		fsLog.Error("Failed to create IOC")
	}
}

