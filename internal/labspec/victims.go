package labspec

// Victims names the tool servers the lab runs, in the order compose declares
// them. A trajectory calls these and no other; the runner points the
// enforcer's upstreams at them, and internal/labcheck holds every other copy
// of the list (compose, the chaos proxies, the classification) to this one.
func Victims() []string {
	return []string{"victim-crm", "victim-db", "victim-fs", "victim-shell", "victim-mail", "victim-web"}
}
