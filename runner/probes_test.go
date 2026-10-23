package main

import "testing"

func TestReadProbeTellsNoRouteApartFromNotHavingRun(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		ran     bool
		reached bool
	}{
		{"reached", "probe reached enforcer:8080\n", true, true},
		{"no route", "probe unreachable victim-fs:8080: lookup victim-fs: no such host\n", true, false},
		{"never ran", "Error response from daemon: no such image\n", false, false},
		{"said nothing at all", "", false, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := readProbe("victim-fs:8080", test.output)
			if got.Ran != test.ran || got.Reached != test.reached {
				t.Errorf("read %+v, want ran=%t reached=%t", got, test.ran, test.reached)
			}
			if got.Detail == "" {
				t.Errorf("the probe carries no reason: %+v", got)
			}
		})
	}
}
