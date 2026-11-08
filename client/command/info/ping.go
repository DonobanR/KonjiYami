package info

import (
	"context"
	insecureRand "math/rand"

	"konjiyami/client/console"
	"konjiyami/protobuf/konjiyamipb"
	"github.com/desertbit/grumble"
)

// PingCmd - Send a round trip C2 message to an implant (does not use ICMP)
func PingCmd(ctx *grumble.Context, con *console.KonjiyamiConsoleClient) {
	session := con.ActiveTarget.GetSessionInteractive()
	if session == nil {
		return
	}

	nonce := insecureRand.Intn(999999)
	con.PrintInfof("Ping %d\n", nonce)
	pong, err := con.Rpc.Ping(context.Background(), &konjiyamipb.Ping{
		Nonce:   int32(nonce),
		Request: con.ActiveTarget.Request(ctx),
	})
	if err != nil {
		con.PrintErrorf("%s\n", err)
	} else {
		con.PrintInfof("Pong %d\n", pong.Nonce)
	}
}

