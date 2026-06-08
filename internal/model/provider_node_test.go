package model

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newProviderNodeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&ProviderNode{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestProviderNode_TableName(t *testing.T) {
	n := ProviderNode{}
	if got := n.TableName(); got != "providerNodes" {
		t.Fatalf("TableName() = %q, want providerNodes", got)
	}
}

func TestProviderNode_TypeIndex(t *testing.T) {
	db := newProviderNodeTestDB(t)
	if !db.Migrator().HasIndex(&ProviderNode{}, "idx_pn_type") {
		t.Errorf("missing index idx_pn_type")
	}
}

func TestProviderNode_Create(t *testing.T) {
	db := newProviderNodeTestDB(t)

	typ := "forward"
	name := "OpenAI Node"
	n := ProviderNode{
		ID:        "pn-1",
		Type:      &typ,
		Name:      &name,
		Data:      `{"baseURL":"https://api.openai.com"}`,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := db.Create(&n).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	var got ProviderNode
	if err := db.First(&got, "id = ?", "pn-1").Error; err != nil {
		t.Fatal(err)
	}
	if got.Data != `{"baseURL":"https://api.openai.com"}` {
		t.Errorf("data mismatch: %q", got.Data)
	}
	if got.Type == nil || *got.Type != "forward" {
		t.Errorf("type = %v, want forward", got.Type)
	}
}

// FindByType returns all nodes matching the given type. Implemented here
// for test coverage; the proper repository function is created in SYS-015.
func FindByType(db *gorm.DB, typ string) ([]ProviderNode, error) {
	var out []ProviderNode
	if err := db.Where("type = ?", typ).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func TestProviderNode_FindByType(t *testing.T) {
	db := newProviderNodeTestDB(t)

	now := time.Now().UTC()
	forward := "forward"
	mw := "middleware"
	n1 := "fwd-1"
	n2 := "fwd-2"
	m1 := "mw-1"

	rows := []ProviderNode{
		{ID: "p-1", Type: &forward, Name: &n1, Data: `{"x":1}`, CreatedAt: now, UpdatedAt: now},
		{ID: "p-2", Type: &forward, Name: &n2, Data: `{"x":2}`, CreatedAt: now, UpdatedAt: now},
		{ID: "p-3", Type: &forward, Name: nil, Data: `{"x":3}`, CreatedAt: now, UpdatedAt: now},
		{ID: "p-4", Type: &mw, Name: &m1, Data: `{"y":1}`, CreatedAt: now, UpdatedAt: now},
	}
	for _, r := range rows {
		if err := db.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}

	got, err := FindByType(db, "forward")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 forward nodes, got %d", len(got))
	}

	empty, _ := FindByType(db, "nonexistent")
	if len(empty) != 0 {
		t.Errorf("expected 0 nodes for nonexistent type, got %d", len(empty))
	}
}

func TestProviderNode_Update(t *testing.T) {
	db := newProviderNodeTestDB(t)

	typ := "forward"
	now := time.Now().UTC()
	n := ProviderNode{
		ID:        "pn-upd",
		Type:      &typ,
		Data:      `{"old":true}`,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := db.Create(&n).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Model(&n).Update("data", `{"new":true}`).Error; err != nil {
		t.Fatalf("update: %v", err)
	}

	var got ProviderNode
	if err := db.First(&got, "id = ?", "pn-upd").Error; err != nil {
		t.Fatal(err)
	}
	if got.Data != `{"new":true}` {
		t.Errorf("data should be updated, got %q", got.Data)
	}
}

func TestProviderNode_DifferentDataShapeByType(t *testing.T) {
	db := newProviderNodeTestDB(t)

	now := time.Now().UTC()
	forward := "forward"
	mw := "middleware"

	fwd := ProviderNode{
		ID:        "pn-fwd",
		Type:      &forward,
		Data:      `{"baseURL":"https://api.openai.com","model":"gpt-4"}`,
		CreatedAt: now, UpdatedAt: now,
	}
	mwNode := ProviderNode{
		ID:        "pn-mw",
		Type:      &mw,
		Data:      `{"header":"X-Trace","removeCookies":["session"]}`,
		CreatedAt: now, UpdatedAt: now,
	}
	for _, r := range []ProviderNode{fwd, mwNode} {
		if err := db.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}

	var gotFwd ProviderNode
	db.First(&gotFwd, "id = ?", "pn-fwd")
	var gotMw ProviderNode
	db.First(&gotMw, "id = ?", "pn-mw")

	if gotFwd.Data == gotMw.Data {
		t.Errorf("forward and middleware should have different data shapes, both got %q", gotFwd.Data)
	}
}
