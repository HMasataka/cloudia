package memorystore

// State store kind for Memorystore instances.
const kindInstance = "gcp:memorystore:instance"

// Instance status values.
const (
	statusReady   = "READY"
	statusCreating = "CREATING"
	statusDeleting = "DELETING"
)

// CreateInstanceRequest は Memorystore instances.create リクエストボディです。
type CreateInstanceRequest struct {
	Name              string `json:"name"`
	Tier              string `json:"tier"`
	MemorySizeGb      int    `json:"memorySizeGb"`
	RedisVersion      string `json:"redisVersion"`
	AuthorizedNetwork string `json:"authorizedNetwork,omitempty"`
}

// InstanceItem は GCP Memorystore インスタンスを表します。
type InstanceItem struct {
	Name         string `json:"name"`
	DisplayName  string `json:"displayName,omitempty"`
	Tier         string `json:"tier"`
	MemorySizeGb int    `json:"memorySizeGb"`
	RedisVersion string `json:"redisVersion"`
	State        string `json:"state"`
	Host         string `json:"host,omitempty"`
	Port         int    `json:"port,omitempty"`
	CreateTime   string `json:"createTime,omitempty"`
}

// UpdateInstanceRequest は Memorystore instances.patch リクエストボディです。
type UpdateInstanceRequest struct {
	DisplayName  string `json:"displayName,omitempty"`
	MemorySizeGb int    `json:"memorySizeGb,omitempty"`
	RedisVersion string `json:"redisVersion,omitempty"`
}

// ListInstancesResponse は instances.list のレスポンスです。
type ListInstancesResponse struct {
	Instances []InstanceItem `json:"instances,omitempty"`
}

// Operation は GCP オペレーションレスポンスです。
type Operation struct {
	Name     string      `json:"name"`
	Done     bool        `json:"done"`
	Response interface{} `json:"response,omitempty"`
}
