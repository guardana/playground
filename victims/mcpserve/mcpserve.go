// Package mcpserve is the part of a victim tool server that is not the lie.
//
// Every victim reads the same four variables, writes the same journal, serves
// MCP at the same path, answers the same health check and has to close its
// journal on the way down. Written six times, those five things drift, and a
// victim whose journal differs from its neighbour's is a victim a scenario
// cannot grade against the others.
//
// What each server keeps for itself is its tools, its data and the annotations
// it lies with. Nothing in this package knows about any of that.
package mcpserve

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/guardana/playground/internal/journal"
)

// Config is what the lab tells a victim about the run it is part of.
type Config struct {
	Listen     string
	ServerName string
	RunID      string
	ReportsDir string
}

// ConfigFromEnv reads the four variables compose gives every lab service.
// lookup is passed in so a test does not have to change the process
// environment to state what it is testing.
//
// Every variable is required. A default would let a server come up writing its
// journal somewhere the runner does not read, and a run graded from a file
// nobody wrote reports that the victim served nothing.
func ConfigFromEnv(lookup func(string) string) (Config, error) {
	config := Config{
		Listen:     lookup("LAB_LISTEN"),
		ServerName: lookup("LAB_SERVER_NAME"),
		RunID:      lookup("LAB_RUN_ID"),
		ReportsDir: lookup("LAB_REPORTS_DIR"),
	}
	for _, variable := range []struct{ name, value string }{
		{"LAB_LISTEN", config.Listen},
		{"LAB_SERVER_NAME", config.ServerName},
		{"LAB_RUN_ID", config.RunID},
		{"LAB_REPORTS_DIR", config.ReportsDir},
	} {
		if variable.value == "" {
			return Config{}, fmt.Errorf("mcpserve: %s is not set", variable.name)
		}
	}
	return config, nil
}

// JournalPath is where this server records the calls it served, under the
// directory the runner creates for the run.
func (c Config) JournalPath() string {
	return filepath.Join(c.ReportsDir, c.RunID, "journals", c.ServerName+".jsonl")
}

// OpenJournal creates the run's journal directory and opens this server's
// journal in it.
func OpenJournal(config Config) (*Recorder, error) {
	path := config.JournalPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	writer, err := journal.Open(path, config.ServerName)
	if err != nil {
		return nil, err
	}
	return NewRecorder(writer, config.RunID), nil
}
