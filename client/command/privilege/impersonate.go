package privilege

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

	"konjiyami/client/console"
	"konjiyami/protobuf/clientpb"
	"konjiyami/protobuf/konjiyamipb"
	"github.com/desertbit/grumble"
	"google.golang.org/protobuf/proto"
)

// ImpersonateCmd - Windows only, impersonate a user token
func ImpersonateCmd(ctx *grumble.Context, con *console.KonjiyamiConsoleClient) {
	session, beacon := con.ActiveTarget.GetInteractive()
	if session == nil && beacon == nil {
		return
	}

	username := ctx.Args.String("username")
	impersonate, err := con.Rpc.Impersonate(context.Background(), &konjiyamipb.ImpersonateReq{
		Request:  con.ActiveTarget.Request(ctx),
		Username: username,
	})
	if err != nil {
		con.PrintErrorf("%s", err)
		return
	}

	if impersonate.Response != nil && impersonate.Response.Async {
		con.AddBeaconCallback(impersonate.Response.TaskID, func(task *clientpb.BeaconTask) {
			err = proto.Unmarshal(task.Response, impersonate)
			if err != nil {
				con.PrintErrorf("Failed to decode response %s\n", err)
				return
			}
			PrintImpersonate(impersonate, username, con)
		})
		con.PrintAsyncResponse(impersonate.Response)
	} else {
		PrintImpersonate(impersonate, username, con)
	}
}

// PrintImpersonate - Print the results of the attempted impersonation
func PrintImpersonate(impersonate *konjiyamipb.Impersonate, username string, con *console.KonjiyamiConsoleClient) {
	if impersonate.Response != nil && impersonate.Response.GetErr() != "" {
		con.PrintErrorf("%s\n", impersonate.Response.GetErr())
		return
	}
	con.PrintInfof("Successfully impersonated %s\n", username)
}

