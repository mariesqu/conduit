//go:build windows

package server

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// restrictToCurrentUser tightens a file's DACL so only the current user
// has read/write access. Windows ignores POSIX 0o600 in any meaningful
// security sense — files inherit the parent directory's ACL by default,
// which usually grants the Users group read access. This function fixes
// that for the auth token file.
//
// Best-effort: on failure we log and continue. The token is still
// random and the file path is user-chosen, so a hardened deploy can
// override by placing the config under a directory the operator
// already locked down.
func restrictToCurrentUser(path string) error {
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("stat: %w", err)
	}

	// Look up the current user's SID.
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return fmt.Errorf("token user: %w", err)
	}
	sid := user.User.Sid

	// Build a DACL with a single ACE: this user → full file access.
	access := []windows.EXPLICIT_ACCESS{
		{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.GRANT_ACCESS,
			Inheritance:       windows.NO_INHERITANCE,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_USER,
				TrusteeValue: windows.TrusteeValueFromSID(sid),
			},
		},
	}
	dacl, err := windows.ACLFromEntries(access, nil)
	if err != nil {
		return fmt.Errorf("acl from entries: %w", err)
	}

	// Apply: replace the file's DACL, do not inherit further.
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("utf16 path: %w", err)
	}
	err = windows.SetNamedSecurityInfo(
		windows.UTF16PtrToString(pathPtr),
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, // owner unchanged
		nil, // group unchanged
		dacl,
		nil, // sacl
	)
	if err != nil {
		return fmt.Errorf("set named security info: %w", err)
	}
	return nil
}
