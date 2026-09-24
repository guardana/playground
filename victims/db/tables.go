package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// The fixture is compiled in: the run stage of the image has no shell and no
// package manager, so a file the binary expects to find on disk is one more
// thing that can be absent and look like a defect in the lab.
//
//go:embed tables.json
var fixture []byte

type table struct {
	Name    string     `json:"name"`
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
}

// database is the whole of this server's storage. Every run starts from the
// fixture, so a scenario that drops a table does not change the next run.
type database struct {
	mu     sync.Mutex
	tables map[string]*table
}

// resultSet is what every statement answers with, including the ones that
// write. RowsAffected is zero for a select.
type resultSet struct {
	Columns      []string   `json:"columns"`
	Rows         [][]string `json:"rows"`
	RowsAffected int        `json:"rows_affected"`
}

var errNoTable = errors.New("no such table")

func loadDatabase() (*database, error) {
	var file struct {
		Tables []table `json:"tables"`
	}
	if err := json.Unmarshal(fixture, &file); err != nil {
		return nil, fmt.Errorf("db fixture: %w", err)
	}
	if len(file.Tables) == 0 {
		return nil, errors.New("db fixture: no tables")
	}

	loaded := &database{tables: make(map[string]*table, len(file.Tables))}
	for _, loadedTable := range file.Tables {
		if _, duplicate := loaded.tables[loadedTable.Name]; duplicate {
			return nil, fmt.Errorf("db fixture: %s appears twice", loadedTable.Name)
		}
		for number, row := range loadedTable.Rows {
			if len(row) != len(loadedTable.Columns) {
				return nil, fmt.Errorf("db fixture: %s row %d has %d values for %d columns",
					loadedTable.Name, number+1, len(row), len(loadedTable.Columns))
			}
		}
		loaded.tables[loadedTable.Name] = &loadedTable
	}
	return loaded, nil
}

// dropTable removes a table outright, so a scenario can read the effect back
// through db.query and find it gone.
func (d *database) dropTable(name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.dropLocked(name)
}

// dropLocked is the same drop from inside a statement, where run already holds
// the lock.
func (d *database) dropLocked(name string) error {
	if _, present := d.tables[name]; !present {
		return fmt.Errorf("%w: %s", errNoTable, name)
	}
	delete(d.tables, name)
	return nil
}

func (d *database) lookup(name string) (*table, error) {
	found, present := d.tables[name]
	if !present {
		return nil, fmt.Errorf("%w: %s", errNoTable, name)
	}
	return found, nil
}

// column is the index of name in the table, or an error naming what the table
// does have.
func (t *table) column(name string) (int, error) {
	for index, column := range t.Columns {
		if column == name {
			return index, nil
		}
	}
	return 0, fmt.Errorf("no column %s in %s", name, t.Name)
}
