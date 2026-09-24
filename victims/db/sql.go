package main

import (
	"fmt"
	"regexp"
	"strings"
)

// The statement forms this server understands. They are deliberately naive:
// this is a victim, not a database. They know nothing about joins,
// sub-selects, precedence or escaping, and a statement they cannot match is
// refused rather than guessed at.
var (
	dropStatement   = regexp.MustCompile(`(?is)^drop\s+table\s+([a-z0-9_]+)\s*;?$`)
	selectStatement = regexp.MustCompile(`(?is)^select\s+(.+?)\s+from\s+([a-z0-9_]+)\s*(?:where\s+(.+?))?\s*;?$`)
	insertStatement = regexp.MustCompile(`(?is)^insert\s+into\s+([a-z0-9_]+)\s+values\s*\((.+)\)\s*;?$`)
	updateStatement = regexp.MustCompile(`(?is)^update\s+([a-z0-9_]+)\s+set\s+([a-z0-9_]+)\s*=\s*'([^']*)'\s*(?:where\s+(.+?))?\s*;?$`)
	deleteStatement = regexp.MustCompile(`(?is)^delete\s+from\s+([a-z0-9_]+)\s*(?:where\s+(.+?))?\s*;?$`)
	equality        = regexp.MustCompile(`(?is)^([a-z0-9_]+)\s*=\s*'([^']*)'$`)
	quoted          = regexp.MustCompile(`'([^']*)'`)
)

// run executes one statement. Nothing here asks which tool it was called
// through, which is why db.query writes when it is given a statement that
// writes.
func (d *database) run(statement string) (resultSet, error) {
	statement = strings.TrimSpace(statement)

	d.mu.Lock()
	defer d.mu.Unlock()

	if match := dropStatement.FindStringSubmatch(statement); match != nil {
		return resultSet{}, d.dropLocked(match[1])
	}
	if match := selectStatement.FindStringSubmatch(statement); match != nil {
		return d.executeSelect(match)
	}
	if match := updateStatement.FindStringSubmatch(statement); match != nil {
		return d.executeUpdate(match)
	}
	if match := deleteStatement.FindStringSubmatch(statement); match != nil {
		return d.executeDelete(match)
	}
	if match := insertStatement.FindStringSubmatch(statement); match != nil {
		return d.executeInsert(match)
	}
	return resultSet{}, fmt.Errorf("cannot parse statement: %s", statement)
}

func (d *database) executeSelect(match []string) (resultSet, error) {
	target, err := d.lookup(match[2])
	if err != nil {
		return resultSet{}, err
	}
	columns, indexes, err := target.project(match[1])
	if err != nil {
		return resultSet{}, err
	}
	matches, err := target.predicate(match[3])
	if err != nil {
		return resultSet{}, err
	}

	selected := resultSet{Columns: columns, Rows: [][]string{}}
	for _, row := range target.Rows {
		if !matches(row) {
			continue
		}
		projected := make([]string, 0, len(indexes))
		for _, index := range indexes {
			projected = append(projected, row[index])
		}
		selected.Rows = append(selected.Rows, projected)
	}
	return selected, nil
}

func (d *database) executeUpdate(match []string) (resultSet, error) {
	target, err := d.lookup(match[1])
	if err != nil {
		return resultSet{}, err
	}
	index, err := target.column(match[2])
	if err != nil {
		return resultSet{}, err
	}
	matches, err := target.predicate(match[4])
	if err != nil {
		return resultSet{}, err
	}

	affected := 0
	for number, row := range target.Rows {
		if !matches(row) {
			continue
		}
		target.Rows[number][index] = match[3]
		affected++
	}
	return resultSet{Rows: [][]string{}, RowsAffected: affected}, nil
}

func (d *database) executeDelete(match []string) (resultSet, error) {
	target, err := d.lookup(match[1])
	if err != nil {
		return resultSet{}, err
	}
	matches, err := target.predicate(match[2])
	if err != nil {
		return resultSet{}, err
	}

	kept := make([][]string, 0, len(target.Rows))
	for _, row := range target.Rows {
		if !matches(row) {
			kept = append(kept, row)
		}
	}
	affected := len(target.Rows) - len(kept)
	target.Rows = kept
	return resultSet{Rows: [][]string{}, RowsAffected: affected}, nil
}

func (d *database) executeInsert(match []string) (resultSet, error) {
	target, err := d.lookup(match[1])
	if err != nil {
		return resultSet{}, err
	}

	var row []string
	for _, value := range quoted.FindAllStringSubmatch(match[2], -1) {
		row = append(row, value[1])
	}
	if len(row) != len(target.Columns) {
		return resultSet{}, fmt.Errorf("%s takes %d values and the statement has %d",
			target.Name, len(target.Columns), len(row))
	}
	target.Rows = append(target.Rows, row)
	return resultSet{Rows: [][]string{}, RowsAffected: 1}, nil
}

// project resolves the column list of a select. A column the table does not
// have is an error, because a report that silently drops one is worse than a
// report that fails.
func (t *table) project(list string) ([]string, []int, error) {
	if strings.TrimSpace(list) == "*" {
		indexes := make([]int, len(t.Columns))
		for index := range t.Columns {
			indexes[index] = index
		}
		return t.Columns, indexes, nil
	}

	var (
		columns []string
		indexes []int
	)
	for _, name := range strings.Split(list, ",") {
		name = strings.TrimSpace(name)
		index, err := t.column(name)
		if err != nil {
			return nil, nil, err
		}
		columns = append(columns, name)
		indexes = append(indexes, index)
	}
	return columns, indexes, nil
}

// predicate compiles a where clause. An empty clause matches every row, which
// is how a statement without one deletes the table's contents.
func (t *table) predicate(clause string) (func([]string) bool, error) {
	clause = strings.TrimSpace(clause)
	if clause == "" {
		return func([]string) bool { return true }, nil
	}
	match := equality.FindStringSubmatch(clause)
	if match == nil {
		return nil, fmt.Errorf("cannot parse condition: %s", clause)
	}
	index, err := t.column(match[1])
	if err != nil {
		return nil, err
	}
	value := match[2]
	return func(row []string) bool { return row[index] == value }, nil
}
