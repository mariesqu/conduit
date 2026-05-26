//go:build !windows

package server

// restrictToCurrentUser is a no-op on Unix — os.WriteFile(path, data, 0o600)
// already produces a file readable+writable only by the owning user, and
// the surrounding code uses that mode.
func restrictToCurrentUser(_ string) error {
	return nil
}
