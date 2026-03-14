package gce

// Store kind for GCE instances.
const kindInstance = "gcp:compute:instance"

// Instance status names.
const (
	statusRunning = "RUNNING"
	statusStopped = "TERMINATED"
)

// InsertInstanceRequest は GCP REST API の instances.insert リクエストボディです。
type InsertInstanceRequest struct {
	Name        string `json:"name"`
	MachineType string `json:"machineType"`
	Disks       []struct {
		InitializeParams *struct {
			SourceImage string `json:"sourceImage"`
		} `json:"initializeParams,omitempty"`
	} `json:"disks,omitempty"`
}

// InstanceItem は GCP インスタンスを表します。
type InstanceItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MachineType string `json:"machineType"`
	Status      string `json:"status"`
	Zone        string `json:"zone"`
	NetworkInterfaces []NetworkInterface `json:"networkInterfaces,omitempty"`
	CreationTimestamp string `json:"creationTimestamp,omitempty"`
}

// NetworkInterface はネットワークインターフェースを表します。
type NetworkInterface struct {
	NetworkIP string `json:"networkIP,omitempty"`
}

// Operation は GCP オペレーションレスポンスです。
type Operation struct {
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	OperationType string `json:"operationType"`
	Status        string `json:"status"`
	TargetID      string `json:"targetId,omitempty"`
	TargetLink    string `json:"targetLink,omitempty"`
	Zone          string `json:"zone,omitempty"`
	HTTPErrorStatusCode int `json:"httpErrorStatusCode,omitempty"`
}

// ListInstancesResponse は instances.list のレスポンスです。
type ListInstancesResponse struct {
	Kind  string         `json:"kind"`
	Items []InstanceItem `json:"items,omitempty"`
}
