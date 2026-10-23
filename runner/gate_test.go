package main

import (
	"context"
	"testing"

	"github.com/guardana/playground/drivers/control"
	"github.com/guardana/playground/runner/driver"
)

// The enforcer's files and services as the compose topology and the lab's own
// configuration name them, written out so a fixture cannot borrow them from
// the driver it tests.
const (
	enforcerService  = "enforcer"
	collectorService = "collector"
	proxyService     = "toxiproxy-tools"
	proxyFile        = "compose/toxiproxy/proxies.json"
	listingSnapshots = "config/gateway/tools"
	classification   = "config/gateway/classification.yaml"
	fingerprints     = "config/gateway/fingerprints.yaml"
	collectorTLSDir  = "collector-tls"
	exportCADir      = "export-ca"
	bundleName       = "policy.bundle"
	healthRecord     = "healthz.json"
	treeLabel        = "io.guardana.playground.enforcer.tree"
	revisionLabel    = "org.opencontainers.image.revision"
)

// testPins are the enforcer's pins every fixture runs at.
func testPins() driver.Pins {
	return driver.Pins{
		{Name: "ENFORCER_NAMESPACE", Value: testNamespace},
		{Name: "ENFORCER_IMAGE", Value: "lab-enforcer"},
		{Name: "ENFORCER_COMMIT", Value: testPin},
	}
}

func newTestControl(t testing.TB, config control.Config) *control.Driver {
	t.Helper()
	built, err := control.New(context.Background(), config)
	if err != nil {
		t.Fatalf("control.New: %v", err)
	}
	return built
}

// reconfigure builds the lab's enforcer again from its configuration with
// change applied.
func reconfigure(t testing.TB, subject *lab, change func(*control.Config)) {
	t.Helper()
	config := subject.control.Config()
	change(&config)
	subject.control = newTestControl(t, config)
	subject.development = subject.control.Development()
}
