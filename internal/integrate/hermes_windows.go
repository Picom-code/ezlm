package integrate

import (
	"context"
	"os/exec"
	"time"
)

// hermesScheduledTasks lists the default Hermes gateway scheduled task if
// it exists (a read-only query).
func hermesScheduledTasks(ctx context.Context, r *Run) []string {
	if _, err := r.Exec(ctx, 20*time.Second, exec.Command("schtasks", "/Query", "/TN", "Hermes_Gateway")); err != nil {
		return nil
	}
	return []string{"task:Hermes_Gateway"}
}
