package webui

import "embed"

// Files contains the production console built by Vite.
//
//go:embed dist
var Files embed.FS
