package serve

import "os/exec"

func hideChild(cmd *exec.Cmd) { hidden(cmd) }
