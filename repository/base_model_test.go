package repository

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type baseModelTestModel struct {
	BaseModel
	Name string `gorm:"column:name"`
}

func (baseModelTestModel) TenantIgnored() bool {
	return true
}

func openBaseModelTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&baseModelTestModel{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestBaseModelStandardTimestampsAndSoftDelete(t *testing.T) {
	db := openBaseModelTestDB(t)
	repo := NewRepository[baseModelTestModel](db)

	model := &baseModelTestModel{Name: "demo"}
	if err := repo.Create(context.Background(), model); err != nil {
		t.Fatalf("create: %v", err)
	}

	if model.CreatedAt.IsZero() {
		t.Fatalf("expected CreatedAt to be set")
	}
	if model.UpdatedAt.IsZero() {
		t.Fatalf("expected UpdatedAt to be set")
	}
	if model.DeletedAt.Valid {
		t.Fatalf("expected DeletedAt to be empty before delete")
	}

	if err := db.Delete(model).Error; err != nil {
		t.Fatalf("gorm delete: %v", err)
	}

	if _, err := repo.FindByID(context.Background(), model.ID.String()); err == nil {
		t.Fatalf("expected soft-deleted record to be filtered out")
	}

	var stored baseModelTestModel
	if err := db.Unscoped().First(&stored, "id = ?", model.ID).Error; err != nil {
		t.Fatalf("unscoped find: %v", err)
	}
	if !stored.DeletedAt.Valid {
		t.Fatalf("expected DeletedAt to be set after soft delete")
	}
}
