package common

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aeciopires/updateGit/internal/config"
)

// LogFileName returns the name of the log file of an execution started at start:
// updateGit_<YEAR>-<MONTH>-<DAY>_<HOUR>-<MINUTE>-<SECOND>.log
func LogFileName(start time.Time) string {
	return fmt.Sprintf("%s_%s.log", config.CLIName, start.Format("2006-01-02_15-04-05"))
}

// OpenLogFile creates the directory (if necessary) and the log file of an execution
// started at start. The file is created with permission 0600 because the default
// directory (/tmp) is shared with other users. It returns the file and its path.
func OpenLogFile(dir string, start time.Time) (*os.File, string, error) {
	if dir == "" {
		dir = "/tmp"
	}
	if err := os.MkdirAll(dir, config.PermissionDir); err != nil {
		return nil, "", fmt.Errorf("cannot create log directory %s: %w", dir, err)
	}

	path := filepath.Join(dir, LogFileName(start))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, config.PermissionLogFile)
	if err != nil {
		return nil, "", fmt.Errorf("cannot write log file %s: %w", path, err)
	}
	// Enforce the permission even if the file already existed or the umask changed it.
	if err := file.Chmod(config.PermissionLogFile); err != nil {
		_ = file.Close()
		return nil, "", fmt.Errorf("cannot set permission of log file %s: %w", path, err)
	}
	return file, path, nil
}
