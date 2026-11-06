package serve

// loginArgs is what the per-user Run key passes to the program it starts.
// The desktop app takes --autostart and starts the server itself. The engine
// takes "start --login", which (unlike plain "start") leaves a server the
// user stopped stopped, so stopping it once doesn't come undone at sign-in.
func loginArgs(app string) string {
	if app != "" {
		return "--autostart"
	}
	return "start --login"
}
