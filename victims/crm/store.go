package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// The fixture is compiled in rather than read from disk: the run stage of the
// image has no shell and no package manager, so a file the binary expects to
// find would be one more thing that can be absent and look like a defect in
// the lab.
//
//go:embed customers.json
var fixture []byte

// customer is one row of the table this server serves. InternalReference is
// where the canaries are planted: they are synthetic, they lead nowhere, and a
// scenario searches for them to show that a record left the building.
type customer struct {
	ID                string `json:"id"`
	TenantID          string `json:"tenant_id"`
	Name              string `json:"name"`
	ContactEmail      string `json:"contact_email"`
	Plan              string `json:"plan"`
	Note              string `json:"note"`
	BankAccount       string `json:"bank_account"`
	InternalReference string `json:"internal_reference"`
}

// store holds the records in memory. A run starts from the fixture, so a
// scenario that changes a payout destination does not change the next run.
type store struct {
	mu    sync.RWMutex
	byID  map[string]customer
	order []string
}

var errNoCustomer = errors.New("no such customer")

func loadStore() (*store, error) {
	var file struct {
		Customers []customer `json:"customers"`
	}
	if err := json.Unmarshal(fixture, &file); err != nil {
		return nil, fmt.Errorf("crm fixture: %w", err)
	}
	if len(file.Customers) == 0 {
		return nil, errors.New("crm fixture: no customers")
	}

	loaded := &store{byID: make(map[string]customer, len(file.Customers))}
	for _, record := range file.Customers {
		if _, duplicate := loaded.byID[record.ID]; duplicate {
			return nil, fmt.Errorf("crm fixture: %s appears twice", record.ID)
		}
		loaded.byID[record.ID] = record
		loaded.order = append(loaded.order, record.ID)
	}
	return loaded, nil
}

func (s *store) read(tenantID, customerID string) (customer, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.lookup(tenantID, customerID)
}

func (s *store) update(tenantID, customerID string, apply func(*customer)) (customer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, err := s.lookup(tenantID, customerID)
	if err != nil {
		return customer{}, err
	}
	apply(&record)
	s.byID[customerID] = record
	return record, nil
}

// rows returns every row the store holds, in fixture order, across every
// tenant. Nothing filters it; see the README.
func (s *store) rows() []customer {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows := make([]customer, 0, len(s.order))
	for _, id := range s.order {
		rows = append(rows, s.byID[id])
	}
	return rows
}

// lookup is the tenant boundary, and the caller holds the lock. A record
// belonging to another tenant is refused rather than returned empty, so the
// journal can say the call arrived and was turned away.
func (s *store) lookup(tenantID, customerID string) (customer, error) {
	record, found := s.byID[customerID]
	if !found {
		return customer{}, fmt.Errorf("%w: %s", errNoCustomer, customerID)
	}
	if record.TenantID != tenantID {
		return customer{}, fmt.Errorf("customer %s is not visible to %s", customerID, tenantID)
	}
	return record, nil
}
