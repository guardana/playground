package evidence

import "fmt"

// trailScope is what every event of one trail shares: the request it accounts
// for, and the project and tenant that request belongs to. A requestId is
// unique only within a project, and the enforcer holds the tenant fixed per
// trail, so a trail that mixes either reads one request's outcome as another's.
type trailScope struct {
	request, project, tenant string
}

// scopeOf reads the scope from the first event and refuses an empty part:
// taken as the scope, it would let every event that also lacks it agree.
func scopeOf(first Event) (trailScope, error) {
	s := trailScope{request: first.RequestID, project: first.ProjectID, tenant: first.TenantID}
	for _, part := range [...]struct{ name, value string }{
		{"requestId", s.request},
		{"projectId", s.project},
		{"tenantId", s.tenant},
	} {
		if part.value == "" {
			return trailScope{}, fmt.Errorf("%w: event %q carries no %s", ErrChainBroken, first.EventID, part.name)
		}
	}
	return s, nil
}

// holds reports the first part of the scope event i does not share. Two scopes
// are one broken trail and not two trails.
func (s trailScope) holds(i int, event Event) error {
	for _, part := range [...]struct{ name, got, want string }{
		{"request", event.RequestID, s.request},
		{"project", event.ProjectID, s.project},
		{"tenant", event.TenantID, s.tenant},
	} {
		if part.got != part.want {
			return fmt.Errorf("%w: event %d belongs to %s %q, not %q", ErrChainBroken, i, part.name, part.got, part.want)
		}
	}
	return nil
}
