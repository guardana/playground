package labspec

// Victims names the tool servers the lab runs, in the order compose declares
// them. A trajectory calls these and no other; the runner points the
// enforcer's upstreams at them, and drivers/control/labcheck holds every
// other copy of the list (compose, the chaos proxies, the classification) to
// this one.
func Victims() []string {
	return []string{
		"victim-crm", "victim-db", "victim-fs", "victim-shell", "victim-mail", "victim-web", "victim-pay",
	}
}

// VerifierVictims names the tool servers that require the run's bearer token.
// They boot in the verifier's profile alone: the enforcer lists every upstream's
// tools without a token, so no trajectory reaches them.
func VerifierVictims() []string {
	return []string{"victim-vault", "victim-vault-anytoken", "victim-vault-sessionauth"}
}

// AuthDoubleJournal is the journal the authorization-server double writes.
const AuthDoubleJournal = "auth-double"

// JournalWriters names every service that writes a journal, each into its own
// directory of the run.
func JournalWriters() []string {
	return append(append(Victims(), VerifierVictims()...), PDPDoubleJournal, ApproverJournal, AuthDoubleJournal)
}
