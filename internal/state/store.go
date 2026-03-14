package state

import (
	"context"

	"github.com/HMasataka/cloudia/pkg/models"
)

// Filter はリソース一覧を絞り込むための条件マップです。
// キーに "Status", "Provider", "Service", "Region", "Kind" を指定するとフィールド直接比較、
// "tag:<key>" を指定すると Tags マップを参照します。全条件は AND で結合されます。
type Filter map[string]string

// Store はリソースの永続化インターフェースです。
type Store interface {
	// Get は kind と id でリソースを取得します。見つからない場合は models.ErrNotFound を返します。
	Get(ctx context.Context, kind, id string) (*models.Resource, error)

	// List は kind に一致するリソースを Filter で絞り込んで返します。
	// kind が空文字の場合は全 Kind を対象にします。
	List(ctx context.Context, kind string, filter Filter) ([]*models.Resource, error)

	// Put はリソースを保存します。同一 kind:id が存在する場合は上書きします。
	Put(ctx context.Context, resource *models.Resource) error

	// Delete は kind と id でリソースを削除します。
	Delete(ctx context.Context, kind, id string) error

	// Snapshot は現在の State をファイルに書き出します（アトミック書き込み）。
	Snapshot(ctx context.Context, path string) error

	// Restore はファイルから State を読み込み、現在の map を置き換えます。
	Restore(ctx context.Context, path string) error
}
