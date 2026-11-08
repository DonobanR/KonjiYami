package environment

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

	"konjiyami/client/console"
	"konjiyami/protobuf/clientpb"
	"konjiyami/protobuf/konjiyamipb"
	"github.com/desertbit/grumble"
	"google.golang.org/protobuf/proto"
)

// EnvUnsetCmd - Unset a remote environment variable
func EnvUnsetCmd(ctx *grumble.Context, con *console.KonjiyamiConsoleClient) {
	session, beacon := con.ActiveTarget.GetInteractive()
	if session == nil && beacon == nil {
		return
	}

	name := ctx.Args.String("name")
	if name == "" {
		con.PrintErrorf("Usage: setenv NAME\n")
		return
	}

	unsetResp, err := con.Rpc.UnsetEnv(context.Background(), &konjiyamipb.UnsetEnvReq{
		Name:    name,
		Request: con.ActiveTarget.Request(ctx),
	})

	if err != nil {
		con.PrintErrorf("%s\n", err)
		return
	}
	if unsetResp.Response != nil && unsetResp.Response.Async {
		con.AddBeaconCallback(unsetResp.Response.TaskID, func(task *clientpb.BeaconTask) {
			err = proto.Unmarshal(task.Response, unsetResp)
			if err != nil {
				con.PrintErrorf("Failed to decode response %s\n", err)
				return
			}
			PrintUnsetEnvInfo(name, unsetResp, con)
		})
		con.PrintAsyncResponse(unsetResp.Response)
	} else {
		PrintUnsetEnvInfo(name, unsetResp, con)
	}

}

// PrintUnsetEnvInfo - Print the set environment info
func PrintUnsetEnvInfo(name string, envInfo *konjiyamipb.UnsetEnv, con *console.KonjiyamiConsoleClient) {
	if envInfo.Response != nil && envInfo.Response.Err != "" {
		con.PrintErrorf("%s\n", envInfo.Response.Err)
		return
	}
	con.PrintInfof("Successfully unset %s\n", name)
}

