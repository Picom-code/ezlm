//go:build !windows

package integrate

import "context"

// Only Windows has scheduled tasks.

func hermesScheduledTasks(ctx context.Context, r *Run) []string { return nil }
