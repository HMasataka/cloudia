package cloudsql

// State store kind for Cloud SQL instances.
const kindInstance = "gcp:cloudsql:instance"

// Instance status values.
const (
	statusRunnable = "RUNNABLE"
	statusCreating = "PENDING_CREATE"
	statusDeleting = "DELETING"
)

// InsertInstanceRequest は Cloud SQL instances.insert リクエストボディです。
type InsertInstanceRequest struct {
	Name            string          `json:"name"`
	DatabaseVersion string          `json:"databaseVersion"`
	Region          string          `json:"region"`
	Settings        *InstanceSettings `json:"settings,omitempty"`
}

// InstanceSettings は Cloud SQL インスタンスの設定です。
type InstanceSettings struct {
	Tier             string `json:"tier,omitempty"`
	DataDiskSizeGb   string `json:"dataDiskSizeGb,omitempty"`
	DataDiskType     string `json:"dataDiskType,omitempty"`
	ActivationPolicy string `json:"activationPolicy,omitempty"`
	BackupConfiguration *BackupConfiguration `json:"backupConfiguration,omitempty"`
}

// BackupConfiguration はバックアップ設定です。
type BackupConfiguration struct {
	Enabled bool `json:"enabled"`
}

// InstanceItem は Cloud SQL インスタンスを表します。
type InstanceItem struct {
	Kind            string          `json:"kind"`
	Name            string          `json:"name"`
	Project         string          `json:"project"`
	DatabaseVersion string          `json:"databaseVersion"`
	Region          string          `json:"region"`
	State           string          `json:"state"`
	IPAddresses     []IPMapping     `json:"ipAddresses,omitempty"`
	Settings        InstanceSettings `json:"settings"`
	CreateTime      string          `json:"createTime,omitempty"`
}

// IPMapping は IP アドレスマッピングです。
type IPMapping struct {
	Type      string `json:"type"`
	IPAddress string `json:"ipAddress"`
}

// UpdateInstanceRequest は Cloud SQL instances.patch リクエストボディです。
type UpdateInstanceRequest struct {
	DatabaseVersion string            `json:"databaseVersion,omitempty"`
	Settings        *InstanceSettings `json:"settings,omitempty"`
}

// ListInstancesResponse は instances.list のレスポンスです。
type ListInstancesResponse struct {
	Kind  string         `json:"kind"`
	Items []InstanceItem `json:"items,omitempty"`
}

// Operation は GCP オペレーションレスポンスです。
type Operation struct {
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	OperationType string `json:"operationType"`
	Status        string `json:"status"`
	TargetID      string `json:"targetId,omitempty"`
	TargetLink    string `json:"targetLink,omitempty"`
}
