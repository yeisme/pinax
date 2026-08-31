package operation

import (
	"testing"
)

func testCreateRequest(t *testing.T, operationID, key string, request any) CreateRequest {
	t.Helper()
	digest, err := CanonicalDigest(request)
	if err != nil {
		t.Fatal(err)
	}
	return CreateRequest{
		OperationID:     operationID,
		IdempotencyKey:  key,
		CapabilityID:    "inbox.capture",
		BindingID:       "rest.inbox.capture",
		PrincipalDigest: IdentityDigest("principal:test-user"),
		ScopeDigest:     IdentityDigest("vault:test", "write:inbox"),
		RequestDigest:   digest,
	}
}
