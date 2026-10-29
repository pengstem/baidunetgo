//go:build e2e

package main

// A development build can expose the real window to tests and agents.
// The plugin listens only when MYGO_AUTOMATION names a socket, and MyGo
// leaves it out of release builds.
import _ "github.com/egoist/mygo/plugins/automation"
