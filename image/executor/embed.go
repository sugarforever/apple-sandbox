// Package executor embeds the sandbox executor's Dockerfile and entrypoint
// script into the apple-sandbox binary, so `apple-sandbox build` works from
// a bare binary download — no repo checkout required.
package executor

import "embed"

//go:embed Dockerfile entrypoint.sh
var Files embed.FS
