package repository

import (
	"context"
	"testing"

	pkgerrors "github.com/aisgo/ais-pkg/errors"
	"github.com/aisgo/ais-pkg/ulid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type ulidPrimaryKeyModel struct {
	BaseModel
	Name string `gorm:"column:name"`
}

func (ulidPrimaryKeyModel) TenantIgnored() bool {
	return true
}

func openULIDPrimaryKeyTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&ulidPrimaryKeyModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestFindByIDSupportsULIDByteaPrimaryKey(t *testing.T) {
	db := openULIDPrimaryKeyTestDB(t)
	repo := NewRepository[ulidPrimaryKeyModel](db)

	model := &ulidPrimaryKeyModel{
		BaseModel: BaseModel{ID: ulid.NewID()},
		Name:      "admin",
	}
	if err := repo.Create(context.Background(), model); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := repo.FindByID(context.Background(), model.ID.String())
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if got.Name != model.Name {
		t.Fatalf("expected %q, got %q", model.Name, got.Name)
	}
}

func TestUpdateByIDSupportsULIDByteaPrimaryKey(t *testing.T) {
	db := openULIDPrimaryKeyTestDB(t)
	repo := NewRepository[ulidPrimaryKeyModel](db)

	model := &ulidPrimaryKeyModel{
		BaseModel: BaseModel{ID: ulid.NewID()},
		Name:      "before",
	}
	if err := repo.Create(context.Background(), model); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.UpdateByID(context.Background(), model.ID.String(), map[string]any{"name": "after"}, "name"); err != nil {
		t.Fatalf("update by id: %v", err)
	}

	got, err := repo.FindByID(context.Background(), model.ID.String())
	if err != nil {
		t.Fatalf("find by id after update: %v", err)
	}
	if got.Name != "after" {
		t.Fatalf("expected updated name, got %q", got.Name)
	}
}

func TestDeleteSupportsULIDByteaPrimaryKey(t *testing.T) {
	db := openULIDPrimaryKeyTestDB(t)
	repo := NewRepository[ulidPrimaryKeyModel](db)

	model := &ulidPrimaryKeyModel{
		BaseModel: BaseModel{ID: ulid.NewID()},
		Name:      "to-delete",
	}
	if err := repo.Create(context.Background(), model); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := repo.Delete(context.Background(), model.ID.String()); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := repo.FindByID(context.Background(), model.ID.String()); err == nil {
		t.Fatalf("expected record to be hidden after soft delete")
	}

	var deleted ulidPrimaryKeyModel
	if err := db.Unscoped().First(&deleted, "id = ?", model.ID).Error; err != nil {
		t.Fatalf("find deleted record: %v", err)
	}
	if !deleted.DeletedAt.Valid {
		t.Fatalf("expected deleted_at to be set")
	}
}

func TestInvalidULIDInputReturnsInvalidArgument(t *testing.T) {
	db := openULIDPrimaryKeyTestDB(t)
	repo := NewRepository[ulidPrimaryKeyModel](db)

	assertInvalidArgument := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: expected error", name)
		}
		if code := pkgerrors.Code(err); code != pkgerrors.ErrCodeInvalidArgument {
			t.Fatalf("%s: expected invalid argument, got %v (%v)", name, code, err)
		}
	}

	_, err := repo.FindByID(context.Background(), "not-a-ulid")
	assertInvalidArgument("FindByID", err)

	_, err = repo.FindByIDs(context.Background(), []string{"not-a-ulid"})
	assertInvalidArgument("FindByIDs", err)

	err = repo.UpdateByID(context.Background(), "not-a-ulid", map[string]any{"name": "after"}, "name")
	assertInvalidArgument("UpdateByID", err)

	err = repo.DeleteBatch(context.Background(), []string{"not-a-ulid"})
	assertInvalidArgument("DeleteBatch", err)

	err = repo.Delete(context.Background(), "not-a-ulid")
	assertInvalidArgument("Delete", err)

	err = repo.HardDelete(context.Background(), "not-a-ulid")
	assertInvalidArgument("HardDelete", err)
}
