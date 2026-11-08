package handlers

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
	------------------------------------------------------------------------

	WARNING: These functions can be invoked by remote implants without user interaction

*/

import (
	"sync"

	"konjiyami/protobuf/konjiyamipb"
	"konjiyami/server/core"
)

type ServerHandler func(*core.ImplantConnection, []byte) *konjiyamipb.Envelope

var (
	tunnelHandlerMutex = &sync.Mutex{}
)

// GetHandlers - Returns a map of server-side msg handlers
func GetHandlers() map[uint32]ServerHandler {
	return map[uint32]ServerHandler{
		// Sessions
		konjiyamipb.MsgRegister:    registerSessionHandler,
		konjiyamipb.MsgTunnelData:  tunnelDataHandler,
		konjiyamipb.MsgTunnelClose: tunnelCloseHandler,
		konjiyamipb.MsgPing:        pingHandler,
		konjiyamipb.MsgSocksData:   socksDataHandler,

		// Beacons
		konjiyamipb.MsgBeaconRegister: beaconRegisterHandler,
		konjiyamipb.MsgBeaconTasks:    beaconTasksHandler,

		// Pivots
		konjiyamipb.MsgPivotPeerEnvelope: pivotPeerEnvelopeHandler,
		konjiyamipb.MsgPivotPeerFailure:  pivotPeerFailureHandler,
	}
}

// GetNonPivotHandlers - Server handlers for pivot connections, its important
// to avoid a pivot handler from calling a pivot handler and causing a recursive
// call stack
func GetNonPivotHandlers() map[uint32]ServerHandler {
	return map[uint32]ServerHandler{
		// Sessions
		konjiyamipb.MsgRegister:    registerSessionHandler,
		konjiyamipb.MsgTunnelData:  tunnelDataHandler,
		konjiyamipb.MsgTunnelClose: tunnelCloseHandler,
		konjiyamipb.MsgPing:        pingHandler,
		konjiyamipb.MsgSocksData:   socksDataHandler,

		// Beacons - Not currently supported in pivots
	}
}

