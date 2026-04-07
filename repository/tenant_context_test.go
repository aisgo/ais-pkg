package repository

import (
	"context"
	"testing"

	"github.com/aisgo/ais-pkg/ulid"
)

func TestTenantContextRoundTrip(t *testing.T) {
	tc := TenantContext{
		TenantID: ulid.NewID(),
		IsAdmin:  false,
	}

	ctx := WithTenantContext(context.Background(), tc)
	got, ok := TenantFromContext(ctx)

	if !ok {
		t.Fatalf("expected tenant context")
	}
	if got.TenantID != tc.TenantID {
		t.Fatalf("unexpected tenant id: %v", got.TenantID)
	}
}
