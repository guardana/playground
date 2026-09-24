package main

import (
	"flag"
	"path/filepath"
	"testing"

	"github.com/guardana/playground/victims/mcpserve"
)

var update = flag.Bool("update", false, "rewrite the listing snapshot the fingerprints were taken from")

func TestTheListingIsTheOneClassified(t *testing.T) {
	path := filepath.Join("..", "..", "config", "gateway", "tools", "victim-mail.json")
	if err := mcpserve.CompareListing(t.Context(), start(t).session, path, *update); err != nil {
		t.Fatal(err)
	}
}
