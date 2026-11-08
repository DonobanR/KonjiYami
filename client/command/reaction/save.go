package reaction

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
	"fmt"
	"os"

	"github.com/AlecAivazis/survey/v2"
	"konjiyami/client/console"
	"konjiyami/client/core"
	"github.com/desertbit/grumble"
)

// ReactionSaveCmd - Manage reactions to events
func ReactionSaveCmd(ctx *grumble.Context, con *console.KonjiyamiConsoleClient) {
	reactionPath := GetReactionFilePath()
	if _, err := os.Stat(reactionPath); !os.IsNotExist(err) {
		confirm := false
		prompt := &survey.Confirm{Message: fmt.Sprintf("Overwrite reactions (%s) on disk?", reactionPath)}
		survey.AskOne(prompt, &confirm)
		if !confirm {
			return
		}
	}
	err := SaveReactions(core.Reactions.All())
	if err != nil {
		con.PrintErrorf("%s\n", err)
	} else {
		con.PrintInfof("Saved reactions to disk (%s)\n", reactionPath)
	}
}

