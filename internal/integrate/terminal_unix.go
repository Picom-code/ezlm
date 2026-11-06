//go:build !windows

package integrate

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"time"
)

// termOpen opens a new terminal window running the launcher. On a Mac that
// is open -a Terminal with the .command file: no AppleEvents, so no
// Automation prompt, and open's exit status says whether Terminal took it.
// Elsewhere the first terminal program found runs the .sh.
func termOpen(spec LaunchSpec, launcher string) error {
	if runtime.GOOS == "darwin" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		argv := launchMacArgv(launcher)
		out, err := exec.CommandContext(ctx, argv[0], argv[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("macOS couldn't open Terminal (%s). Open %s yourself to start %s", launchErrText(err, out), launcher, launchName(spec))
		}
		return nil
	}
	argv, err := launchLinuxArgv(launcher, exec.LookPath)
	if errors.Is(err, errLaunchNoTerminal) {
		return fmt.Errorf("no terminal program was found. Run %s in a terminal to start %s", launcher, launchName(spec))
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = spec.Dir
	if err := launchStart(cmd); err != nil {
		return fmt.Errorf("the terminal didn't open (%s). Run %s in a terminal to start %s", err, launcher, launchName(spec))
	}
	return nil
}
