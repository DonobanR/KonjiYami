package sessions

import (
	"konjiyami/client/console"
	"github.com/desertbit/grumble"
)

// BackgroundCmd - Background the active session
func BackgroundCmd(ctx *grumble.Context, con *console.KonjiyamiConsoleClient) {
	con.ActiveTarget.Background()
	con.PrintInfof("Background ...\n")
}

