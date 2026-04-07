package repository

import (
	"context"
	"strings"
	"testing"

	pkgerrors "github.com/aisgo/ais-pkg/errors"
	"github.com/aisgo/ais-pkg/ulid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type tenantTestModel struct {
	ID       string   `gorm:"column:id;type:char(26);primaryKey"`
	TenantID ulid.ID  `gorm:"column:tenant_id;type:bytea;not null"`
	DeptID   *ulid.ID `gorm:"column:dept_id;type:bytea"`
	Name     string   `gorm:"column:name"`
}

type nonTenantModel struct {
	ID   string `gorm:"column:id;type:char(26);primaryKey"`
	Name string `gorm:"column:name"`
}

type invalidTenantModel struct {
	ID   string `gorm:"column:id;type:char(26);primaryKey"`
	Name string `gorm:"column:name"`
}

func (nonTenantModel) TenantIgnored() bool {
	return true
}

func openTenantTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&tenantTestModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func openNonTenantTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&nonTenantModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func openInvalidTenantTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&invalidTenantModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestTenantFindByIDScope(t *testing.T) {
	db := openTenantTestDB(t)
	repo := NewRepository[tenantTestModel](db)

	tenantA := ulid.NewID()
	tenantB := ulid.NewID()

	a := &tenantTestModel{ID: ulid.NewID().String(), Name: "a", TenantID: tenantA}
	b := &tenantTestModel{ID: ulid.NewID().String(), Name: "b", TenantID: tenantB}

	if err := repo.Create(WithTenantContext(context.Background(), TenantContext{TenantID: tenantA, IsAdmin: true}), a); err != nil {
		t.Fatalf("create a: %v", err)
	}
	if err := repo.Create(WithTenantContext(context.Background(), TenantContext{TenantID: tenantB, IsAdmin: true}), b); err != nil {
		t.Fatalf("create b: %v", err)
	}

	ctxA := WithTenantContext(context.Background(), TenantContext{TenantID: tenantA, IsAdmin: true})
	if _, err := repo.FindByID(ctxA, b.ID); err == nil {
		t.Fatalf("expected not found for cross-tenant id")
	}

	if _, err := repo.FindByID(ctxA, a.ID); err != nil {
		t.Fatalf("expected find by id: %v", err)
	}
}

func TestTenantIgnoredModelCreateAndQuery(t *testing.T) {
	db := openNonTenantTestDB(t)
	repo := NewRepository[nonTenantModel](db)

	m := &nonTenantModel{ID: ulid.NewID().String(), Name: "n1"}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("create: %v", err)
	}

	if _, err := repo.FindByID(context.Background(), m.ID); err != nil {
		t.Fatalf("find: %v", err)
	}
}

func TestTenantCreateAutoFill(t *testing.T) {
	db := openTenantTestDB(t)
	repo := NewRepository[tenantTestModel](db)

	tenant := ulid.NewID()
	ctx := WithTenantContext(context.Background(), TenantContext{TenantID: tenant, IsAdmin: true})

	m := &tenantTestModel{ID: ulid.NewID().String(), Name: "auto"}
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("create: %v", err)
	}
	if m.TenantID != tenant {
		t.Fatalf("tenant not set")
	}
}

func TestTenantCreateMissingContext(t *testing.T) {
	db := openTenantTestDB(t)
	repo := NewRepository[tenantTestModel](db)

	m := &tenantTestModel{ID: ulid.NewID().String(), Name: "no-ctx"}
	if err := repo.Create(context.Background(), m); err == nil {
		t.Fatalf("expected error without tenant context")
	}
}

func TestUpdateIgnoresZeroValues(t *testing.T) {
	db := openTenantTestDB(t)
	repo := NewRepository[tenantTestModel](db)
	tenant := ulid.NewID()
	ctx := WithTenantContext(context.Background(), TenantContext{TenantID: tenant, IsAdmin: true})

	m := &tenantTestModel{ID: ulid.NewID().String(), Name: "before"}
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("create: %v", err)
	}

	m.Name = ""
	if err := repo.Update(ctx, m); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := repo.FindByID(ctx, m.ID)
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.Name != "before" {
		t.Fatalf("expected name preserved, got: %s", got.Name)
	}
}

func TestUpdateByIDRespectsTenant(t *testing.T) {
	db := openTenantTestDB(t)
	repo := NewRepository[tenantTestModel](db)

	tenantA := ulid.NewID()
	tenantB := ulid.NewID()

	m := &tenantTestModel{ID: ulid.NewID().String(), Name: "before"}
	if err := repo.Create(WithTenantContext(context.Background(), TenantContext{TenantID: tenantA, IsAdmin: true}), m); err != nil {
		t.Fatalf("create: %v", err)
	}

	ctxB := WithTenantContext(context.Background(), TenantContext{TenantID: tenantB, IsAdmin: true})
	if err := repo.UpdateByID(ctxB, m.ID, map[string]any{"name": "after"}, "name"); err == nil {
		t.Fatalf("expected not found for cross-tenant update")
	}
}

func TestUpdateByIDCannotMutateTenant(t *testing.T) {
	db := openTenantTestDB(t)
	repo := NewRepository[tenantTestModel](db)

	tenant := ulid.NewID()
	ctx := WithTenantContext(context.Background(), TenantContext{TenantID: tenant, IsAdmin: true})

	m := &tenantTestModel{ID: ulid.NewID().String(), Name: "before"}
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("create: %v", err)
	}

	otherTenant := ulid.NewID()
	if err := repo.UpdateByID(ctx, m.ID, map[string]any{"tenant_id": otherTenant}, "tenant_id"); err == nil {
		t.Fatalf("expected update to reject tenant_id change")
	}
}

func TestUpdateByIDStrictModeRequiresWhitelist(t *testing.T) {
	db := openTenantTestDB(t)
	repo := NewRepository[tenantTestModel](db)

	impl, ok := repo.(*RepositoryImpl[tenantTestModel])
	if !ok {
		t.Fatalf("expected repository implementation")
	}
	impl.SetStrictUpdates(true)

	tenant := ulid.NewID()
	ctx := WithTenantContext(context.Background(), TenantContext{TenantID: tenant, IsAdmin: true})

	m := &tenantTestModel{ID: ulid.NewID().String(), Name: "before"}
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.UpdateByID(ctx, m.ID, map[string]any{"name": "after"}); err == nil {
		t.Fatalf("expected strict mode to require explicit whitelist")
	}
}

func TestFindByIDNormalizesNotFound(t *testing.T) {
	db := openTenantTestDB(t)
	repo := NewRepository[tenantTestModel](db)
	ctx := WithTenantContext(context.Background(), TenantContext{TenantID: ulid.NewID(), IsAdmin: true})

	_, err := repo.FindByID(ctx, ulid.NewID().String())
	if err == nil {
		t.Fatalf("expected not found error")
	}
	if pkgerrors.Code(err) != pkgerrors.ErrCodeNotFound {
		t.Fatalf("expected not found biz error, got %v", err)
	}
	if !strings.Contains(err.Error(), "failed to find record") {
		t.Fatalf("expected normalized message, got %v", err)
	}
}

func TestFindByIDInvalidOptionFailsClosed(t *testing.T) {
	db := openTenantTestDB(t)
	repo := NewRepository[tenantTestModel](db)
	tenant := ulid.NewID()
	ctx := WithTenantContext(context.Background(), TenantContext{TenantID: tenant, IsAdmin: true})

	m := &tenantTestModel{ID: ulid.NewID().String(), Name: "before"}
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err := repo.FindByID(ctx, m.ID, WithOrderBy("id; DROP TABLE users"))
	if err == nil {
		t.Fatalf("expected invalid query option error")
	}
	if pkgerrors.Code(err) != pkgerrors.ErrCodeInvalidArgument {
		t.Fatalf("expected invalid argument biz error, got %v", err)
	}
}

func TestFindOneByCondition(t *testing.T) {
	db := openTenantTestDB(t)
	repo := NewRepository[tenantTestModel](db)
	tenant := ulid.NewID()
	ctx := WithTenantContext(context.Background(), TenantContext{TenantID: tenant, IsAdmin: true})

	m := &tenantTestModel{ID: ulid.NewID().String(), Name: "alice"}
	if err := repo.Create(ctx, m); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.FindOneByCondition(ctx, map[string]any{"name": "alice"})
	if err != nil {
		t.Fatalf("FindOneByCondition: %v", err)
	}
	if got.ID != m.ID {
		t.Fatalf("unexpected record: %+v", got)
	}
}

func TestTenantScopeErrorSuggestsTenantIgnorable(t *testing.T) {
	db := openInvalidTenantTestDB(t)
	repo := NewRepository[invalidTenantModel](db)
	ctx := WithTenantContext(context.Background(), TenantContext{TenantID: ulid.NewID(), IsAdmin: true})

	if _, err := repo.FindByID(ctx, "missing"); err == nil {
		t.Fatalf("expected tenant scope error")
	} else if got := err.Error(); got == "" || !containsAll(got, "TenantIgnorable", "tenant_id") {
		t.Fatalf("expected guidance to mention tenant_id and TenantIgnorable, got: %v", err)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(s, part) {
			return false
		}
	}
	return true
}
