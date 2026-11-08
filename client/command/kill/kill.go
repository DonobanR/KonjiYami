package kill

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
	"errors"

	"github.com/AlecAivazis/survey/v2"
	"konjiyami/client/console"
	"konjiyami/protobuf/clientpb"
	"konjiyami/protobuf/commonpb"
	"konjiyami/protobuf/konjiyamipb"
	"konjiyami/server/core"
	"github.com/desertbit/grumble"
)

// KillCmd - Kill the active session (not to be confused with TerminateCmd)
func KillCmd(ctx *grumble.Context, con *console.KonjiyamiConsoleClient) {
	session, beacon := con.ActiveTarget.GetInteractive()
	// Confirm with the user, just in case they confused kill with terminate
	confirm := false
	con.PrintWarnf("WARNING: This will kill the remote implant process\n\n")
	if session != nil {
		survey.AskOne(&survey.Confirm{Message: "Kill the active session?"}, &confirm, nil)
		if !confirm {
			return
		}
		err := KillSession(session, ctx, con)
		if err != nil {
			con.PrintErrorf("%s\n", err)
			return
		}
		con.PrintInfof("Killed %s (%s)\n", session.Name, session.ID)
		con.ActiveTarget.Background()
		return
	} else if beacon != nil {
		survey.AskOne(&survey.Confirm{Message: "Kill the active beacon?"}, &confirm, nil)
		if !confirm {
			return
		}
		err := KillBeacon(beacon, ctx, con)
		if err != nil {
			con.PrintErrorf("%s\n", err)
			return
		}
		con.PrintInfof("Killed %s (%s)\n", beacon.Name, beacon.ID)
		con.ActiveTarget.Background()
		return
	}
	con.PrintErrorf("No active session or beacon\n")
	return
}

func KillSession(session *clientpb.Session, ctx *grumble.Context, con *console.KonjiyamiConsoleClient) error {
	if session == nil {
		return errors.New("session does not exist")
	}
	_, err := con.Rpc.Kill(context.Background(), &konjiyamipb.KillReq{
		Request: &commonpb.Request{
			SessionID: session.ID,
			Timeout:   int64(ctx.Flags.Int("timeout")),
		},
		Force: ctx.Flags.Bool("force"),
	})
	core.Sessions.Remove(session.ID)
	return err
}

func KillBeacon(beacon *clientpb.Beacon, ctx *grumble.Context, con *console.KonjiyamiConsoleClient) error {
	if beacon == nil {
		return errors.New("session does not exist")
	}
	_, err := con.Rpc.Kill(context.Background(), &konjiyamipb.KillReq{
		Request: &commonpb.Request{
			BeaconID: beacon.ID,
			Timeout:  int64(ctx.Flags.Int("timeout")),
		},
		Force: ctx.Flags.Bool("force"),
	})
	return err

}

