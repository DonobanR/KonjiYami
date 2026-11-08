//go:build server

package assets

import "embed"

var (
	//go:embed fs/*.zip fs/windows/amd64/*
	assetsFs embed.FS
)
