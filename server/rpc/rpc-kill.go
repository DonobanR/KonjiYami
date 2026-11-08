package rpc

/*
	KONJIYAMI Implant Framework
	Copyright (C) 2022  Bishop Fox

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
	"strings"
	"time"

	"konjiyami/protobuf/commonpb"
	"konjiyami/protobuf/konjiyamipb"
	"konjiyami/server/core"
	"konjiyami/server/db"
	"konjiyami/server/db/models"
	"google.golang.org/protobuf/proto"
)

// Kill - Kill the implant process
func (rpc *Server) Kill(ctx context.Context, kill *konjiyamipb.KillReq) (*commonpb.Empty, error) {
	var (
		beacon *models.Beacon
		err    error
	)
	session := core.Sessions.Get(kill.Request.SessionID)
	if session == nil {
		beacon, err = db.BeaconByID(kill.Request.BeaconID)
		if err != nil {
			return &commonpb.Empty{}, ErrInvalidBeaconID
		} else {
			return rpc.killBeacon(kill, beacon)
		}
	}
	return rpc.killSession(kill, session)
}

func (rpc *Server) killSession(kill *konjiyamipb.KillReq, session *core.Session) (*commonpb.Empty, error) {
	data, err := proto.Marshal(kill)
	if err != nil {
		return nil, err
	}
	timeout := time.Duration(kill.Request.GetTimeout())
	// Do not block waiting for the msg send, the implant connection may already be dead
	go session.Request(konjiyamipb.MsgNumber(kill), timeout, data)
	core.Sessions.Remove(session.ID)
	return &commonpb.Empty{}, nil
}

func (rpc *Server) killBeacon(kill *konjiyamipb.KillReq, beacon *models.Beacon) (*commonpb.Empty, error) {
	resp := &commonpb.Empty{}
	request := kill.GetRequest()
	request.SessionID = ""
	request.Async = true
	request.BeaconID = beacon.ID.String()
	reqData, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	task, err := beacon.Task(&konjiyamipb.Envelope{
		Type: konjiyamipb.MsgKillSessionReq,
		Data: reqData,
	})
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(kill.ProtoReflect().Descriptor().FullName().Name()), ".")
	name := parts[len(parts)-1]
	task.Description = name
	// Update db
	err = db.Session().Save(task).Error
	return resp, err
}

