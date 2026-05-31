//go:build !windows

package main

// runApp on non-Windows always runs the console loop — there is no tray
// (these targets typically run as a service or in a terminal).
func runApp(app *appRuntime) { runConsole(app) }
