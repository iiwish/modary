//go:build !linux && !darwin

package projecttool

import "os/exec"

func configureBuildCommand(*exec.Cmd) {}

func cleanupBuildCommand(*exec.Cmd) error { return nil }
