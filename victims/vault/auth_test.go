package main

import (
	"testing"

	"github.com/guardana/playground/victims/vaultserve/vaulttest"
)

// The honest vault serves the run's token and nothing else: not another
// token, not an unsigned one, not a session presented without a token.
func TestOnlyTheRunsTokenIsServed(t *testing.T) {
	victim := vaulttest.Start(t, serverName, policy())
	vaulttest.AssertLines(t, "answers", victim.Probe(), []string{
		"initialize, no credential: 401",
		"initialize, the run's token: 200",
		"initialized, the run's token: 202",
		"initialize, another token: 401",
		"initialize, an alg:none token: 401",
		"tools/list, the session alone: 401",
		"tools/list, another token and the session: 401",
		"tools/list, the run's token and the session: 200",
		"tools/list, a session never issued alone: 401",
	})
	vaulttest.AssertLines(t, "auth lines", victim.AuthLines(), []string{
		"none none 401 initialize",
		"run none 200 initialize",
		"run known 202 notifications/initialized",
		"other none 401 initialize",
		"other none 401 initialize",
		"none known 401 tools/list",
		"other known 401 tools/list",
		"run known 200 tools/list",
		"none unknown 401 tools/list",
	})
}
