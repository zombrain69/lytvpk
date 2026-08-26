//go:build !bindings

package main

import backend "vpk-manager/internal/app"

func ensureSingleInstance(args []string) {
	backend.EnsureSingleton(args)
}
