package mcpserve

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrListingChanged reports a victim whose first tool listing is no longer the
// one the enforcer's fingerprints in config/gateway were taken from.
var ErrListingChanged = errors.New("mcpserve: the tool listing changed")

// CompareListing compares the first tools/list a client gets with the snapshot
// at path, or rewrites the snapshot when update is set. scripts/classify-victims.sh
// rewrites the snapshots and the fingerprints together, so a victim whose tools
// change is caught here, without Docker, before a scenario meets a tool the
// enforcer no longer classifies.
func CompareListing(ctx context.Context, session *mcp.ClientSession, path string, update bool) error {
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		return err
	}
	got, err := json.MarshalIndent(listed.Tools, "", "  ")
	if err != nil {
		return err
	}
	got = append(got, '\n')
	if update {
		return os.WriteFile(path, got, 0o644) // #nosec G306 -- a tracked snapshot, public.
	}
	want, err := os.ReadFile(path) // #nosec G304 -- the caller's own snapshot path.
	if err != nil {
		return err
	}
	if !bytes.Equal(got, want) {
		return fmt.Errorf("%w: %s no longer matches; run scripts/classify-victims.sh", ErrListingChanged, path)
	}
	return nil
}
