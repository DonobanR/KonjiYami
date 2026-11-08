//go:build client

package assets

import "embed"

var (
	//go:embed fs/english.txt fs/konjiyami.asc
	assetsFs embed.FS
)
