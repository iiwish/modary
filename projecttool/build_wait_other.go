//go:build !linux && !darwin

package projecttool

import "os/exec"

func waitBuildCommandExit(*exec.Cmd) error { return ErrBuildUnsupported }
