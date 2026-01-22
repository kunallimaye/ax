package eventlog

import (
	"fmt"
	"os"
)

const defaultEventLogDir = "eventlog"

// ListSessions returns all session IDs that have event log files.
func ListSessions(dir string) ([]string, error) {
	if dir == "" {
		dir = defaultEventLogDir
	}

	// Check if directory exists
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return []string{}, nil
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read event log directory: %w", err)
	}

	var sessions []string
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		// Extract session ID from filename (remove .log extension)
		name := file.Name()
		if len(name) > 4 && name[len(name)-4:] == ".log" {
			sessionID := name[:len(name)-4]
			sessions = append(sessions, sessionID)
		}
	}

	return sessions, nil
}

// TODO(rakyll): Move Exists, Delete, ListSessions.
