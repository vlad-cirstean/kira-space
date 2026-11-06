package redis

import "time"

// SetConsoleReadTimeout shortens the console read timeout for a test and returns the restore.
func SetConsoleReadTimeout(d time.Duration) (restore func()) {
	old := consoleReadTimeout
	consoleReadTimeout = d
	return func() { consoleReadTimeout = old }
}
