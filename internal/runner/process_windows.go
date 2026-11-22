package runner

import "os/exec"

// Windows never runs Apple build jobs. CommandContext terminates the direct
// child for diagnostic/test helpers; remote worker cancellation is a later API.
func configureProcess(cmd *exec.Cmd) {}
