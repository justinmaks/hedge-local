package cli

import "os"

// hedgeDirPerm is the permission for ~/.hedge and other hcli-owned directories.
// 0700 keeps the telemetry database (which may contain prompt/log content) and
// daemon logs readable only by the owner.
const hedgeDirPerm = 0o700

// secureLogPerm is the permission for daemon log files, which can contain
// captured telemetry detail.
const secureLogPerm = 0o600

// mkdirSecure creates dir (and parents) and enforces owner-only permissions,
// downgrading directories created by older versions that used 0755.
func mkdirSecure(dir string) error {
	if err := os.MkdirAll(dir, hedgeDirPerm); err != nil {
		return err
	}
	// MkdirAll is a no-op when dir already exists, so set the mode explicitly.
	return os.Chmod(dir, hedgeDirPerm)
}

// openSecureAppendLog opens path for appending, creating it owner-only.
func openSecureAppendLog(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, secureLogPerm)
}
