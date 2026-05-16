package scheduler

import (
	"fmt"
	"sort"
)

// FindAgentByID scans agentsDir and returns the agent with the given id. The
// scanner already enforces first-wins for duplicate ids, so the returned
// agent is the same one cron would schedule. Returns a NotFoundError when no
// agent matches.
func FindAgentByID(agentsDir, id string) (Agent, ScanResult, error) {
	res, err := ScanAgents(agentsDir)
	if err != nil {
		return Agent{}, res, err
	}
	for _, a := range res.Agents {
		if a.ID == id {
			return a, res, nil
		}
	}
	return Agent{}, res, NotFoundError{ID: id, Available: agentIDs(res.Agents)}
}

// NotFoundError signals that no agent with the requested id exists. The
// caller decides how to render the list of known ids in its diagnostic.
type NotFoundError struct {
	ID        string
	Available []string
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("no agent with id %q", e.ID)
}

func agentIDs(agents []Agent) []string {
	out := make([]string, 0, len(agents))
	for _, a := range agents {
		out = append(out, a.ID)
	}
	sort.Strings(out)
	return out
}
