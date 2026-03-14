package state

import (
	"context"
	"errors"
	"os"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/pkg/models"
)

// FileStore は MemoryStore を embed し、Put/Delete のたびにファイルへアトミック書き込みします。
type FileStore struct {
	*MemoryStore
	filePath string
	logger   *zap.Logger
}

// NewFileStore は filePath を永続化先とする FileStore を返します。
// ファイルが存在する場合は Restore で State を復元します。
// JSON パースエラーの場合はログ警告を出力し、空の State で起動します。
func NewFileStore(filePath string, logger *zap.Logger) (*FileStore, error) {
	ms := NewMemoryStore()
	fs := &FileStore{
		MemoryStore: ms,
		filePath:    filePath,
		logger:      logger,
	}

	if _, err := os.Stat(filePath); err == nil {
		if restoreErr := ms.Restore(context.Background(), filePath); restoreErr != nil {
			logger.Warn("file store: failed to restore state from file, starting with empty state",
				zap.String("file_path", filePath),
				zap.Error(restoreErr),
			)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	return fs, nil
}

// Put はリソースを保存し、変更後にファイルへアトミック書き込みします。
func (f *FileStore) Put(ctx context.Context, resource *models.Resource) error {
	if err := f.MemoryStore.Put(ctx, resource); err != nil {
		return err
	}
	return f.MemoryStore.Snapshot(ctx, f.filePath)
}

// Delete はリソースを削除し、変更後にファイルへアトミック書き込みします。
func (f *FileStore) Delete(ctx context.Context, kind, id string) error {
	if err := f.MemoryStore.Delete(ctx, kind, id); err != nil {
		return err
	}
	return f.MemoryStore.Snapshot(ctx, f.filePath)
}
