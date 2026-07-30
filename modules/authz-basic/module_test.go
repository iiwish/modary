package authz_basic

import (
	"context"
	"testing"
	"time"

	"modary/core/action"
	"modary/core/authz"
	"modary/core/identity"
)

func TestAuthorizerEnforcesPermissionWorkspaceAndAgentLimit(t *testing.T) {
	authorizer := Authorizer{}
	actor := identity.Actor{
		ID:             "agent",
		Type:           "agent",
		WorkspaceID:    "ws_default",
		Permissions:    []string{"rulary.run.execute"},
		AllowedActions: []string{"rulary.run.execute"},
		MaxRows:        50,
		ExpiresAtUnix:  time.Now().Add(time.Hour).Unix(),
		GrantVersion:   "1",
	}
	request := authz.Request{
		Actor: actor, ActionID: "rulary.run.execute", Permission: "rulary.run.execute",
		WorkspaceID: "ws_default", Phase: authz.PhaseImpact, Impact: authz.Impact{Rows: 51},
	}
	decision, err := authorizer.Authorize(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Allowed || decision.Code != action.CodeLimitExceeded {
		t.Fatalf("decision = %+v", decision)
	}
	request.Impact.Rows = 50
	decision, err = authorizer.Authorize(context.Background(), request)
	if err != nil || !decision.Allowed {
		t.Fatalf("decision = %+v, err = %v", decision, err)
	}
}
