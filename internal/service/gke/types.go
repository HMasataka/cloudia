package gke

// Store kind for GKE clusters.
const kindCluster = "gcp:gke:cluster"

// Cluster status names.
const (
	statusRunning = "RUNNING"
)

// CreateClusterRequest は GKE clusters.create リクエストボディです。
type CreateClusterRequest struct {
	Cluster ClusterSpec `json:"cluster"`
}

// ClusterSpec はクラスタの仕様を表します。
type ClusterSpec struct {
	Name             string `json:"name"`
	InitialNodeCount int    `json:"initialNodeCount,omitempty"`
	MasterVersion    string `json:"initialClusterVersion,omitempty"`
}

// ClusterItem は GKE クラスタを表します。
type ClusterItem struct {
	Name             string     `json:"name"`
	Status           string     `json:"status"`
	Endpoint         string     `json:"endpoint"`
	MasterAuth       MasterAuth `json:"masterAuth,omitempty"`
	InitialNodeCount int        `json:"initialNodeCount,omitempty"`
	MasterVersion    string     `json:"initialClusterVersion,omitempty"`
}

// MasterAuth はマスター認証情報を表します。
type MasterAuth struct {
	ClusterCaCertificate string `json:"clusterCaCertificate,omitempty"`
}

// Operation は GKE オペレーションレスポンスです。
type Operation struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	TargetLink string `json:"targetLink,omitempty"`
}

// ListClustersResponse は clusters.list のレスポンスです。
type ListClustersResponse struct {
	Clusters []ClusterItem `json:"clusters"`
}
