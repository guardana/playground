package vaulttest

import "fmt"

// Probe sends, in order, the requests the verifier's authorization checks
// turn on, and returns "<request>: <status>" for each. The session the run's
// token opened is the known one the later requests present.
func (v *Victim) Probe() []string {
	v.t.Helper()
	bearer := "Bearer " + Token()
	var got []string
	send := func(label, authorization, session, body string) Answer {
		answer := v.Post(authorization, session, body)
		got = append(got, fmt.Sprintf("%s: %d", label, answer.Status))
		return answer
	}
	send("initialize, no credential", "", "", Initialize)
	opened := send("initialize, the run's token", bearer, "", Initialize)
	if opened.Session == "" {
		v.t.Fatalf("the run's token opened no session: %v", got)
	}
	send("initialized, the run's token", bearer, opened.Session, initialized)
	send("initialize, another token", "Bearer "+Token()[1:]+"Q", "", Initialize)
	send("initialize, an alg:none token", "Bearer "+AlgNone(), "", Initialize)
	send("tools/list, the session alone", "", opened.Session, ListTools)
	send("tools/list, another token and the session", "Bearer "+Token()[1:]+"Q", opened.Session, ListTools)
	send("tools/list, the run's token and the session", bearer, opened.Session, ListTools)
	send("tools/list, a session never issued alone", "", "session-never-issued", ListTools)
	return got
}
