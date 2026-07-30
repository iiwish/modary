package agent_mcp

import (
	_ "embed"

	"modary/core/module"
)

//go:embed module.yaml
var manifestData []byte

func Module() module.Registration {
	return module.Registration{Manifest: module.MustParseManifest(manifestData)}
}
