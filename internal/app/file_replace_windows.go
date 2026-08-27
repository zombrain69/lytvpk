//go:build windows

package app

import "golang.org/x/sys/windows"

// replaceFile replaces destination with source on Windows. os.Rename does not
// consistently replace an existing file on Windows, while these configuration
// paths are routinely rewritten in place.
func replaceFile(source, destination string) error {

	sourcePtr, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	destinationPtr, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(sourcePtr, destinationPtr, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
