package eks

// CreateClusterRequest は CreateCluster API のリクエストボディです。
type CreateClusterRequest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	RoleArn string `json:"roleArn"`
}

// CertificateAuthority は EKS クラスタの証明書認証局情報です。
type CertificateAuthority struct {
	Data string `json:"data"`
}

// Cluster は EKS クラスタの情報を表します。
type Cluster struct {
	Name                string               `json:"name"`
	Arn                 string               `json:"arn"`
	Status              string               `json:"status"`
	Endpoint            string               `json:"endpoint"`
	KubernetesVersion   string               `json:"version"`
	RoleArn             string               `json:"roleArn"`
	CertificateAuthority CertificateAuthority `json:"certificateAuthority"`
}

// ClusterResponse は単一クラスタのレスポンスです。
type ClusterResponse struct {
	Cluster Cluster `json:"cluster"`
}

// ListClustersResponse はクラスタ一覧のレスポンスです。
type ListClustersResponse struct {
	Clusters []string `json:"clusters"`
}

// eksError は EKS エラーレスポンスの構造体です。
type eksError struct {
	Message string `json:"message"`
	Type    string `json:"__type"`
}
