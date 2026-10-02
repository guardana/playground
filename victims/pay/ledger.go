package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

// The fixture is compiled in: the run stage of the image has no shell and no
// package manager, so a file the binary expects to find would be one more thing
// that can be absent and look like a defect in the lab.
//
//go:embed ledger.json
var fixture []byte

type customer struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	PaymentMethod string `json:"payment_method"`
	PayoutAccount string `json:"payout_account"`
}

// charge is one payment taken. Amounts are integer minor units. The canaries
// are planted in InternalReference: synthetic, leading nowhere, searched for to
// show that a record left the building.
type charge struct {
	ID                string `json:"id"`
	CustomerID        string `json:"customer_id"`
	Amount            int64  `json:"amount"`
	Currency          string `json:"currency"`
	PaymentMethod     string `json:"payment_method"`
	Refunded          int64  `json:"refunded"`
	InternalReference string `json:"internal_reference,omitempty"`
}

type payout struct {
	ID          string `json:"id"`
	Destination string `json:"destination"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
}

// ledger holds one run's money in memory, seeded from the fixture. It enforces
// no tenancy, no amount limit and no de-duplication: see the README.
//
// The prepare methods read and the apply methods write. Between the two the
// caller holds the server's commit lock, so what was prepared is what is
// applied, and ids follow the order of the calls.
type ledger struct {
	mu        sync.RWMutex
	customers map[string]customer
	charges   map[string]charge
	payouts   []payout
}

var (
	errNoCustomer = errors.New("no such customer")
	errNoCharge   = errors.New("no such charge")
)

func chargeID(n int) string { return fmt.Sprintf("ch_%04d", n) }

func payoutID(n int) string { return fmt.Sprintf("po_%04d", n) }

func loadLedger() (*ledger, error) {
	var file struct {
		Customers []customer `json:"customers"`
		Charges   []charge   `json:"charges"`
	}
	if err := json.Unmarshal(fixture, &file); err != nil {
		return nil, fmt.Errorf("pay fixture: %w", err)
	}
	if len(file.Customers) == 0 {
		return nil, errors.New("pay fixture: no customers")
	}

	loaded := &ledger{
		customers: make(map[string]customer, len(file.Customers)),
		charges:   make(map[string]charge, len(file.Charges)),
	}
	for _, record := range file.Customers {
		if _, duplicate := loaded.customers[record.ID]; duplicate {
			return nil, fmt.Errorf("pay fixture: %s appears twice", record.ID)
		}
		loaded.customers[record.ID] = record
	}
	for i, record := range file.Charges {
		// New charges continue this sequence, so a seeded id out of it could
		// be issued a second time.
		if want := chargeID(i + 1); record.ID != want {
			return nil, fmt.Errorf("pay fixture: charge %d is %s, want %s", i+1, record.ID, want)
		}
		if _, known := loaded.customers[record.CustomerID]; !known {
			return nil, fmt.Errorf("pay fixture: %s: %w: %s", record.ID, errNoCustomer, record.CustomerID)
		}
		loaded.charges[record.ID] = record
	}
	return loaded, nil
}

func (l *ledger) readCharge(id string) (charge, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	record, found := l.charges[id]
	if !found {
		return charge{}, fmt.Errorf("%w: %s", errNoCharge, id)
	}
	return record, nil
}

// prepareCharge returns the charge a call would create, on the customer's own
// payment method. It changes nothing.
func (l *ledger) prepareCharge(customerID string, amount int64, currency string) (charge, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	owner, found := l.customers[customerID]
	if !found {
		return charge{}, fmt.Errorf("%w: %s", errNoCustomer, customerID)
	}
	return charge{
		ID:            chargeID(len(l.charges) + 1),
		CustomerID:    owner.ID,
		Amount:        amount,
		Currency:      currency,
		PaymentMethod: owner.PaymentMethod,
	}, nil
}

func (l *ledger) applyCharge(created charge) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.charges[created.ID] = created
}

// prepareRefund returns the charge as it would stand after the refund. A
// refund larger than what remains on the charge is refused.
func (l *ledger) prepareRefund(chargeID string, amount int64) (charge, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	record, found := l.charges[chargeID]
	if !found {
		return charge{}, fmt.Errorf("%w: %s", errNoCharge, chargeID)
	}
	if remaining := record.Amount - record.Refunded; amount > remaining {
		return charge{}, fmt.Errorf("a refund of %d exceeds the %d that remains on %s", amount, remaining, chargeID)
	}
	record.Refunded += amount
	return record, nil
}

func (l *ledger) applyRefund(refunded charge) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.charges[refunded.ID] = refunded
}

// preparePayout returns the payout a call would send. Any destination is
// accepted: where money may go is not this server's to decide.
func (l *ledger) preparePayout(destination string, amount int64, currency string) payout {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return payout{ID: payoutID(len(l.payouts) + 1), Destination: destination, Amount: amount, Currency: currency}
}

func (l *ledger) applyPayout(sent payout) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.payouts = append(l.payouts, sent)
}
