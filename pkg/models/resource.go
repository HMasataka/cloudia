package models

import "time"

// Resource はクラウドリソースを表す共通構造体です。
type Resource struct {
	// Kind はリソースの種別を示します (例: "aws:s3:bucket", "gcp:compute:instance")。
	Kind string

	// ID はリソースの一意識別子です。
	ID string

	// Provider はクラウドプロバイダを示します ("aws" または "gcp")。
	Provider string

	// Service はクラウドサービス名を示します ("s3", "compute" 等)。
	Service string

	// Region はリソースが存在するリージョンです。
	Region string

	// Tags はリソースに付与されたタグです。
	Tags map[string]string

	// Spec はリソース固有の仕様を保持します。
	Spec map[string]interface{}

	// Status はリソースの現在の状態です ("creating", "active", "deleting" 等)。
	Status string

	// CreatedAt はリソースが作成された日時です。
	CreatedAt time.Time

	// UpdatedAt はリソースが最後に更新された日時です。
	UpdatedAt time.Time

	// ContainerID は対応する Docker コンテナ ID です。
	ContainerID string

	// TTL は自動クリーンアップまでの存続期間です。nil の場合は自動クリーンアップされません。
	TTL *time.Duration
}
