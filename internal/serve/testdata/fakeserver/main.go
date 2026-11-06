// Command fakeserver stands in for llama-server in tests. It stays up until
// it is killed, so a test can start it, forget it the way a crashed engine
// would, and then look for it again by PID.
package main

import "time"

func main() {
	for {
		time.Sleep(time.Hour)
	}
}
