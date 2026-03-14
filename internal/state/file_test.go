package state_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

func TestFileStore_PutRestoreGet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	logger := zap.NewNop()

	fs, err := state.NewFileStore(path, logger)
	if err != nil {
		t.Fatal(err)
	}

	r := newResource("aws:s3:bucket", "file1", "aws", "s3", "us-east-1", "active", nil)
	if err := fs.Put(context.Background(), r); err != nil {
		t.Fatal(err)
	}

	// プロセス再起動相当: 新しい FileStore を生成して同じファイルを読む
	fs2, err := state.NewFileStore(path, logger)
	if err != nil {
		t.Fatal(err)
	}

	got, err := fs2.Get(context.Background(), "aws:s3:bucket", "file1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "file1" {
		t.Fatalf("expected file1, got %s", got.ID)
	}
}

func TestFileStore_DeletePersisted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	logger := zap.NewNop()

	fs, err := state.NewFileStore(path, logger)
	if err != nil {
		t.Fatal(err)
	}

	r := newResource("aws:s3:bucket", "file2", "aws", "s3", "us-east-1", "active", nil)
	fs.Put(context.Background(), r)
	fs.Delete(context.Background(), "aws:s3:bucket", "file2")

	fs2, err := state.NewFileStore(path, logger)
	if err != nil {
		t.Fatal(err)
	}

	_, err = fs2.Get(context.Background(), "aws:s3:bucket", "file2")
	if !errors.Is(err, models.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete+restart, got %v", err)
	}
}

func TestFileStore_InvalidJSONStartsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	os.WriteFile(path, []byte("{invalid json"), 0644)
	logger := zap.NewNop()

	fs, err := state.NewFileStore(path, logger)
	if err != nil {
		t.Fatal(err)
	}

	// 空 State で起動されているのでリソースは存在しない
	list, err := fs.List(context.Background(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty state after invalid JSON, got %d resources", len(list))
	}
}

func TestFileStore_SnapshotSeparatePath(t *testing.T) {
	dir := t.TempDir()
	autoPath := filepath.Join(dir, "auto.json")
	snapPath := filepath.Join(dir, "snap.json")
	logger := zap.NewNop()

	fs, err := state.NewFileStore(autoPath, logger)
	if err != nil {
		t.Fatal(err)
	}

	r := newResource("aws:s3:bucket", "snap2", "aws", "s3", "us-east-1", "active", nil)
	fs.Put(context.Background(), r)

	// Snapshot を別パスに書き出す
	if err := fs.Snapshot(context.Background(), snapPath); err != nil {
		t.Fatal(err)
	}

	// 別パスから Restore できる
	ms := state.NewMemoryStore()
	if err := ms.Restore(context.Background(), snapPath); err != nil {
		t.Fatal(err)
	}
	got, err := ms.Get(context.Background(), "aws:s3:bucket", "snap2")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "snap2" {
		t.Fatalf("expected snap2, got %s", got.ID)
	}
}
