package state_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

func newResource(kind, id, provider, service, region, status string, tags map[string]string) *models.Resource {
	return &models.Resource{
		Kind:      kind,
		ID:        id,
		Provider:  provider,
		Service:   service,
		Region:    region,
		Status:    status,
		Tags:      tags,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestMemoryStore_GetNotFound(t *testing.T) {
	s := state.NewMemoryStore()
	_, err := s.Get(context.Background(), "aws:s3:bucket", "nonexistent")
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMemoryStore_PutAndGet(t *testing.T) {
	s := state.NewMemoryStore()
	r := newResource("aws:s3:bucket", "id1", "aws", "s3", "us-east-1", "active", nil)
	if err := s.Put(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(context.Background(), "aws:s3:bucket", "id1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "id1" {
		t.Fatalf("expected id1, got %s", got.ID)
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	s := state.NewMemoryStore()
	r := newResource("aws:s3:bucket", "id1", "aws", "s3", "us-east-1", "active", nil)
	s.Put(context.Background(), r)
	s.Delete(context.Background(), "aws:s3:bucket", "id1")
	_, err := s.Get(context.Background(), "aws:s3:bucket", "id1")
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestMemoryStore_ListByKind(t *testing.T) {
	s := state.NewMemoryStore()
	s.Put(context.Background(), newResource("aws:s3:bucket", "b1", "aws", "s3", "us-east-1", "active", nil))
	s.Put(context.Background(), newResource("aws:s3:bucket", "b2", "aws", "s3", "us-east-1", "active", nil))
	s.Put(context.Background(), newResource("gcp:compute:instance", "i1", "gcp", "compute", "us-central1", "active", nil))

	list, err := s.List(context.Background(), "aws:s3:bucket", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
}

func TestMemoryStore_ListAllKinds(t *testing.T) {
	s := state.NewMemoryStore()
	s.Put(context.Background(), newResource("aws:s3:bucket", "b1", "aws", "s3", "us-east-1", "active", nil))
	s.Put(context.Background(), newResource("gcp:compute:instance", "i1", "gcp", "compute", "us-central1", "active", nil))

	list, err := s.List(context.Background(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
}

func TestMemoryStore_FilterByStatus(t *testing.T) {
	s := state.NewMemoryStore()
	s.Put(context.Background(), newResource("k", "a", "aws", "s3", "us-east-1", "active", nil))
	s.Put(context.Background(), newResource("k", "b", "aws", "s3", "us-east-1", "terminated", nil))

	list, err := s.List(context.Background(), "", state.Filter{"Status": "active"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "a" {
		t.Fatalf("expected 1 active resource, got %v", list)
	}
}

func TestMemoryStore_FilterByTag(t *testing.T) {
	s := state.NewMemoryStore()
	s.Put(context.Background(), newResource("k", "a", "aws", "s3", "us-east-1", "active", map[string]string{"env": "prod"}))
	s.Put(context.Background(), newResource("k", "b", "aws", "s3", "us-east-1", "active", map[string]string{"env": "dev"}))

	list, err := s.List(context.Background(), "", state.Filter{"tag:env": "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "a" {
		t.Fatalf("expected 1 prod resource, got %v", list)
	}
}

func TestMemoryStore_FilterByKindField(t *testing.T) {
	s := state.NewMemoryStore()
	s.Put(context.Background(), newResource("aws:s3:bucket", "a", "aws", "s3", "us-east-1", "active", nil))
	s.Put(context.Background(), newResource("gcp:compute:instance", "b", "gcp", "compute", "us-central1", "active", nil))

	list, err := s.List(context.Background(), "", state.Filter{"Kind": "aws:s3:bucket"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "a" {
		t.Fatalf("expected 1 result, got %v", list)
	}
}

func TestMemoryStore_FilterAND(t *testing.T) {
	s := state.NewMemoryStore()
	s.Put(context.Background(), newResource("k", "a", "aws", "s3", "us-east-1", "active", map[string]string{"env": "prod"}))
	s.Put(context.Background(), newResource("k", "b", "gcp", "compute", "us-central1", "active", map[string]string{"env": "prod"}))

	list, err := s.List(context.Background(), "", state.Filter{"Provider": "aws", "tag:env": "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "a" {
		t.Fatalf("expected 1 result for AND filter, got %v", list)
	}
}

func TestMemoryStore_SnapshotAndRestore(t *testing.T) {
	s := state.NewMemoryStore()
	s.Put(context.Background(), newResource("aws:s3:bucket", "snap1", "aws", "s3", "us-east-1", "active", nil))

	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	if err := s.Snapshot(context.Background(), path); err != nil {
		t.Fatal(err)
	}

	s2 := state.NewMemoryStore()
	if err := s2.Restore(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	got, err := s2.Get(context.Background(), "aws:s3:bucket", "snap1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "snap1" {
		t.Fatalf("expected snap1, got %s", got.ID)
	}
}

func TestMemoryStore_RestoreInvalidJSON(t *testing.T) {
	s := state.NewMemoryStore()
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("{invalid json"), 0644)
	err := s.Restore(context.Background(), path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestMemoryStore_ConcurrentAccess(t *testing.T) {
	s := state.NewMemoryStore()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id := "res" + string(rune('0'+n%10))
			r := newResource("k", id, "aws", "s3", "us-east-1", "active", nil)
			s.Put(context.Background(), r)
			s.Get(context.Background(), "k", id)
			s.List(context.Background(), "k", nil)
		}(i)
	}
	wg.Wait()
}
