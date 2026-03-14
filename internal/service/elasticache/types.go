package elasticache

import "encoding/xml"

// ElastiCache XML namespace.
const xmlNamespace = "https://elasticache.amazonaws.com/doc/2015-02-02/"

// State store kinds.
const (
	kindCacheCluster    = "aws:elasticache:cache-cluster"
	kindReplicationGroup = "aws:elasticache:replication-group"
)

// Cluster status values.
const (
	statusAvailable = "available"
	statusDeleting  = "deleting"
)

// Endpoint はノードエンドポイントを表します。
type Endpoint struct {
	Address string `xml:"Address"`
	Port    int    `xml:"Port"`
}

// CacheClusterItem はレスポンス内の CacheCluster 要素です。
type CacheClusterItem struct {
	CacheClusterId        string   `xml:"CacheClusterId"`
	CacheClusterStatus    string   `xml:"CacheClusterStatus"`
	CacheNodeType         string   `xml:"CacheNodeType"`
	Engine                string   `xml:"Engine"`
	EngineVersion         string   `xml:"EngineVersion"`
	NumCacheNodes         int      `xml:"NumCacheNodes"`
	ConfigurationEndpoint Endpoint `xml:"ConfigurationEndpoint"`
}

// CreateCacheClusterResult はクラスター作成結果です。
type CreateCacheClusterResult struct {
	CacheCluster CacheClusterItem `xml:"CacheCluster"`
}

// CreateCacheClusterResponse は CreateCacheCluster アクションのレスポンスです。
type CreateCacheClusterResponse struct {
	XMLName                  xml.Name                 `xml:"CreateCacheClusterResponse"`
	RequestId                string                   `xml:"ResponseMetadata>RequestId"`
	CreateCacheClusterResult CreateCacheClusterResult `xml:"CreateCacheClusterResult"`
}

// DeleteCacheClusterResult はクラスター削除結果です。
type DeleteCacheClusterResult struct {
	CacheCluster CacheClusterItem `xml:"CacheCluster"`
}

// DeleteCacheClusterResponse は DeleteCacheCluster アクションのレスポンスです。
type DeleteCacheClusterResponse struct {
	XMLName                  xml.Name                 `xml:"DeleteCacheClusterResponse"`
	RequestId                string                   `xml:"ResponseMetadata>RequestId"`
	DeleteCacheClusterResult DeleteCacheClusterResult `xml:"DeleteCacheClusterResult"`
}

// DescribeCacheClustersResult は Describe 結果です。
type DescribeCacheClustersResult struct {
	CacheClusters []CacheClusterItem `xml:"CacheClusters>CacheCluster"`
}

// DescribeCacheClustersResponse は DescribeCacheClusters アクションのレスポンスです。
type DescribeCacheClustersResponse struct {
	XMLName                     xml.Name                    `xml:"DescribeCacheClustersResponse"`
	RequestId                   string                      `xml:"ResponseMetadata>RequestId"`
	DescribeCacheClustersResult DescribeCacheClustersResult `xml:"DescribeCacheClustersResult"`
}

// ModifyCacheClusterResult はクラスター変更結果です。
type ModifyCacheClusterResult struct {
	CacheCluster CacheClusterItem `xml:"CacheCluster"`
}

// ModifyCacheClusterResponse は ModifyCacheCluster アクションのレスポンスです。
type ModifyCacheClusterResponse struct {
	XMLName                  xml.Name                 `xml:"ModifyCacheClusterResponse"`
	RequestId                string                   `xml:"ResponseMetadata>RequestId"`
	ModifyCacheClusterResult ModifyCacheClusterResult `xml:"ModifyCacheClusterResult"`
}

// ReplicationGroupItem はレスポンス内の ReplicationGroup 要素です。
type ReplicationGroupItem struct {
	ReplicationGroupId          string   `xml:"ReplicationGroupId"`
	Description                 string   `xml:"Description"`
	Status                      string   `xml:"Status"`
	ConfigurationEndpoint        Endpoint `xml:"ConfigurationEndpoint"`
	AutomaticFailover            string   `xml:"AutomaticFailover"`
}

// CreateReplicationGroupResult はレプリケーショングループ作成結果です。
type CreateReplicationGroupResult struct {
	ReplicationGroup ReplicationGroupItem `xml:"ReplicationGroup"`
}

// CreateReplicationGroupResponse は CreateReplicationGroup アクションのレスポンスです。
type CreateReplicationGroupResponse struct {
	XMLName                      xml.Name                     `xml:"CreateReplicationGroupResponse"`
	RequestId                    string                       `xml:"ResponseMetadata>RequestId"`
	CreateReplicationGroupResult CreateReplicationGroupResult `xml:"CreateReplicationGroupResult"`
}

// DeleteReplicationGroupResult はレプリケーショングループ削除結果です。
type DeleteReplicationGroupResult struct {
	ReplicationGroup ReplicationGroupItem `xml:"ReplicationGroup"`
}

// DeleteReplicationGroupResponse は DeleteReplicationGroup アクションのレスポンスです。
type DeleteReplicationGroupResponse struct {
	XMLName                      xml.Name                     `xml:"DeleteReplicationGroupResponse"`
	RequestId                    string                       `xml:"ResponseMetadata>RequestId"`
	DeleteReplicationGroupResult DeleteReplicationGroupResult `xml:"DeleteReplicationGroupResult"`
}

// DescribeReplicationGroupsResult は Describe 結果です。
type DescribeReplicationGroupsResult struct {
	ReplicationGroups []ReplicationGroupItem `xml:"ReplicationGroups>ReplicationGroup"`
}

// DescribeReplicationGroupsResponse は DescribeReplicationGroups アクションのレスポンスです。
type DescribeReplicationGroupsResponse struct {
	XMLName                         xml.Name                        `xml:"DescribeReplicationGroupsResponse"`
	RequestId                       string                          `xml:"ResponseMetadata>RequestId"`
	DescribeReplicationGroupsResult DescribeReplicationGroupsResult `xml:"DescribeReplicationGroupsResult"`
}
