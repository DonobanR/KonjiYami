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

// EnvGetCmd - Get a remote environment variable
func EnvGetCmd(ctx *grumble.Context, con *console.KonjiyamiConsoleClient) {
	session, beacon := con.ActiveTarget.GetInteractive()
	if session == nil && beacon == nil {
		return
	}

	name := ctx.Args.String("name")
	envInfo, err := con.Rpc.GetEnv(context.Background(), &konjiyamipb.EnvReq{
		Name:    name,
		Request: con.ActiveTarget.Request(ctx),
	})
	if err != nil {
		con.PrintErrorf("%s\n", err)
		return
	}
	if envInfo.Response != nil && envInfo.Response.Async {
		con.AddBeaconCallback(envInfo.Response.TaskID, func(task *clientpb.BeaconTask) {
			err = proto.Unmarshal(task.Response, envInfo)
			if err != nil {
				con.PrintErrorf("Failed to decode response %s\n", err)
				return
			}
			PrintGetEnvInfo(envInfo, con)
		})
		con.PrintAsyncResponse(envInfo.Response)
	} else {
		PrintGetEnvInfo(envInfo, con)
	}
}

// PrintGetEnvInfo - Print the results of the env get command
func PrintGetEnvInfo(envInfo *konjiyamipb.EnvInfo, con *console.KonjiyamiConsoleClient) {
	if envInfo.Response != nil && envInfo.Response.Err != "" {
		con.PrintErrorf("%s\n", envInfo.Response.Err)
		return
	}
	for _, envVar := range envInfo.Variables {
		con.Printf("%s=%s\n", envVar.Key, envVar.Value)
	}
}

