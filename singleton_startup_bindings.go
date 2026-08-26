//go:build bindings

package main

// Binding generation runs a temporary Wails executable. It must not hand off
// to an already-running desktop instance before Wails can emit JS bindings.
func ensureSingleInstance(_ []string) {}
