package main

import (
	"errors"
	"testing"
	"time"
)

func lookupFrom(env map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := env[name]
		return value, ok
	}
}

func validEnv() map[string]string {
	return map[string]string{"LAB_RUN_ID": "run-1", "LAB_REPORTS_DIR": "/reports"}
}

func validArgs() []string {
	return []string{
		"-identifier", "https://pdp-double:8443", "-san", "pdp-double, 10.0.0.5",
		"-ca-out", "/pki/pdp-ca.pem", "-script", "/scripts/pdp.yaml",
	}
}

func TestSettingsFromFlagsAndEnvironment(t *testing.T) {
	s, err := parseSettings(validArgs(), lookupFrom(validEnv()))
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range []struct{ name, got, want string }{
		{"listen", s.listen, ":8443"},
		{"journal", s.journalPath(), "/reports/run-1/journals/pdp-double.jsonl"},
		{"evaluation path", s.evaluationPath(), "/access/v1/evaluation"},
		{"discovery path", s.discoveryPath(), "/.well-known/authzen-configuration"},
		{"endpoint", s.endpoint(), "https://pdp-double:8443/access/v1/evaluation"},
	} {
		if check.got != check.want {
			t.Errorf("%s = %q, want %q", check.name, check.got, check.want)
		}
	}
	if len(s.sans) != 2 || s.sans[0] != "pdp-double" || s.sans[1] != "10.0.0.5" {
		t.Errorf("sans = %q", s.sans)
	}
	if s.hold != 30*time.Second {
		t.Errorf("hold = %v, want 30s", s.hold)
	}
}

func TestSettingsRefuse(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra []string
		env   map[string]string
	}{
		{"plaintext identifier", []string{"-identifier", "http://127.0.0.1:8443"}, validEnv()},
		{"identifier with a query", []string{"-identifier", "https://pdp-double?x=1"}, validEnv()},
		{"identifier with userinfo", []string{"-identifier", "https://lab@pdp-double"}, validEnv()},
		{"identifier with a fragment", []string{"-identifier", "https://pdp-double/#f"}, validEnv()},
		{"no identifier", []string{"-identifier", ""}, validEnv()},
		{"no SAN", []string{"-san", " , "}, validEnv()},
		{"identifier name not among the SANs", []string{"-identifier", "https://pdp.lab.test:8443"}, validEnv()},
		{"identifier IP not among the SANs", []string{"-identifier", "https://10.0.0.6:8443"}, validEnv()},
		{"a SAN under another", []string{"-san", "lab.test,pdp.lab.test", "-identifier", "https://pdp.lab.test:8443"}, validEnv()},
		{"no CA path", []string{"-ca-out", ""}, validEnv()},
		{"no script", []string{"-script", ""}, validEnv()},
		{"zero hold", []string{"-hold", "0s"}, validEnv()},
		{"hold above a minute", []string{"-hold", "61s"}, validEnv()},
		{"no run id", nil, map[string]string{"LAB_REPORTS_DIR": "/reports"}},
		{"no reports dir", nil, map[string]string{"LAB_RUN_ID": "run-1"}},
		{"stray argument", []string{"extra"}, validEnv()},
		{"unknown flag", []string{"-key-out", "/pki/key.pem"}, validEnv()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSettings(append(validArgs(), tc.extra...), lookupFrom(tc.env))
			if !errors.Is(err, errInvalidConfig) {
				t.Fatalf("parseSettings = %v, want errInvalidConfig", err)
			}
		})
	}
}

func TestAHoldOfAMinuteIsAccepted(t *testing.T) {
	s, err := parseSettings(append(validArgs(), "-hold", "60s"), lookupFrom(validEnv()))
	if err != nil || s.hold != time.Minute {
		t.Fatalf("hold %v, error %v; want a minute accepted", s.hold, err)
	}
}

func TestAnIdentifierHostAmongTheSANsIsAccepted(t *testing.T) {
	for _, identifier := range []string{"https://PDP-Double:8443/pdp", "https://10.0.0.5", "https://[::1]:8443"} {
		args := append(validArgs(), "-identifier", identifier, "-san", "pdp-double,10.0.0.5,::1")
		if _, err := parseSettings(args, lookupFrom(validEnv())); err != nil {
			t.Errorf("%s: %v", identifier, err)
		}
	}
}
