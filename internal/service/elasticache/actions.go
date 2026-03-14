package elasticache

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	awsprot "github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

// cacheClusterItemFromResource は models.Resource から CacheClusterItem を構築します。
func cacheClusterItemFromResource(r *models.Resource, host string, port int) CacheClusterItem {
	nodeType, _ := r.Spec["CacheNodeType"].(string)
	engine, _ := r.Spec["Engine"].(string)
	engineVersion, _ := r.Spec["EngineVersion"].(string)
	numNodes := 1
	if n, ok := r.Spec["NumCacheNodes"].(int); ok {
		numNodes = n
	}

	return CacheClusterItem{
		CacheClusterId:     r.ID,
		CacheClusterStatus: r.Status,
		CacheNodeType:      nodeType,
		Engine:             engine,
		EngineVersion:      engineVersion,
		NumCacheNodes:      numNodes,
		ConfigurationEndpoint: Endpoint{
			Address: host,
			Port:    port,
		},
	}
}

// replicationGroupItemFromResource は models.Resource から ReplicationGroupItem を構築します。
func replicationGroupItemFromResource(r *models.Resource, host string, port int) ReplicationGroupItem {
	desc, _ := r.Spec["Description"].(string)
	return ReplicationGroupItem{
		ReplicationGroupId: r.ID,
		Description:        desc,
		Status:             r.Status,
		ConfigurationEndpoint: Endpoint{
			Address: host,
			Port:    port,
		},
		AutomaticFailover: "disabled",
	}
}

// createCacheCluster は CreateCacheCluster アクションを処理します。
func (e *ElastiCacheService) createCacheCluster(ctx context.Context, req service.Request) (service.Response, error) {
	clusterID := req.Params["CacheClusterId"]
	if clusterID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter CacheClusterId.")
	}

	// 重複チェック
	if _, err := e.store.Get(ctx, kindCacheCluster, clusterID); err == nil {
		return errorResponse(http.StatusBadRequest, "CacheClusterAlreadyExists",
			fmt.Sprintf("A cache cluster with the id %q already exists.", clusterID))
	}

	engine := req.Params["Engine"]
	if engine == "" {
		engine = "redis"
	}
	engineVersion := req.Params["EngineVersion"]
	if engineVersion == "" {
		engineVersion = "7.4"
	}
	cacheNodeType := req.Params["CacheNodeType"]
	if cacheNodeType == "" {
		cacheNodeType = "cache.t3.micro"
	}

	numCacheNodes := 1
	if v := req.Params["NumCacheNodes"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			numCacheNodes = n
		}
	}

	authToken := req.Params["AuthToken"]

	now := time.Now().UTC()
	resource := &models.Resource{
		Kind:      kindCacheCluster,
		ID:        clusterID,
		Provider:  "aws",
		Service:   "elasticache",
		Region:    e.cfg.Region,
		Status:    statusAvailable,
		CreatedAt: now,
		UpdatedAt: now,
		Spec: map[string]interface{}{
			"Engine":        engine,
			"EngineVersion": engineVersion,
			"CacheNodeType": cacheNodeType,
			"NumCacheNodes": numCacheNodes,
			"AuthToken":     authToken,
		},
	}

	if err := e.store.Put(ctx, resource); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	item := cacheClusterItemFromResource(resource, e.redis.Host(), e.redisPort())
	resp := CreateCacheClusterResponse{
		RequestId:                "cloudia-elasticache",
		CreateCacheClusterResult: CreateCacheClusterResult{CacheCluster: item},
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// deleteCacheCluster は DeleteCacheCluster アクションを処理します。
func (e *ElastiCacheService) deleteCacheCluster(ctx context.Context, req service.Request) (service.Response, error) {
	clusterID := req.Params["CacheClusterId"]
	if clusterID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter CacheClusterId.")
	}

	r, err := e.store.Get(ctx, kindCacheCluster, clusterID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusBadRequest, "CacheClusterNotFound",
				fmt.Sprintf("Cache cluster %q not found.", clusterID))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	r.Status = statusDeleting
	r.UpdatedAt = time.Now().UTC()

	if err := e.store.Delete(ctx, kindCacheCluster, clusterID); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	item := cacheClusterItemFromResource(r, e.redis.Host(), e.redisPort())
	resp := DeleteCacheClusterResponse{
		RequestId:                "cloudia-elasticache",
		DeleteCacheClusterResult: DeleteCacheClusterResult{CacheCluster: item},
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// describeCacheClusters は DescribeCacheClusters アクションを処理します。
func (e *ElastiCacheService) describeCacheClusters(ctx context.Context, req service.Request) (service.Response, error) {
	clusterID := req.Params["CacheClusterId"]

	var resources []*models.Resource

	if clusterID != "" {
		r, err := e.store.Get(ctx, kindCacheCluster, clusterID)
		if err != nil {
			if errors.Is(err, models.ErrNotFound) {
				return errorResponse(http.StatusBadRequest, "CacheClusterNotFound",
					fmt.Sprintf("Cache cluster %q not found.", clusterID))
			}
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}
		resources = []*models.Resource{r}
	} else {
		var err error
		resources, err = e.store.List(ctx, kindCacheCluster, state.Filter{})
		if err != nil {
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}
	}

	host := e.redis.Host()
	port := e.redisPort()

	items := make([]CacheClusterItem, 0, len(resources))
	for _, r := range resources {
		items = append(items, cacheClusterItemFromResource(r, host, port))
	}

	resp := DescribeCacheClustersResponse{
		RequestId: "cloudia-elasticache",
		DescribeCacheClustersResult: DescribeCacheClustersResult{
			CacheClusters: items,
		},
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// modifyCacheCluster は ModifyCacheCluster アクションを処理します。
func (e *ElastiCacheService) modifyCacheCluster(ctx context.Context, req service.Request) (service.Response, error) {
	clusterID := req.Params["CacheClusterId"]
	if clusterID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter CacheClusterId.")
	}

	r, err := e.store.Get(ctx, kindCacheCluster, clusterID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusBadRequest, "CacheClusterNotFound",
				fmt.Sprintf("Cache cluster %q not found.", clusterID))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	// 変更可能なパラメータを反映 (メタデータのみ)
	if v := req.Params["CacheNodeType"]; v != "" {
		r.Spec["CacheNodeType"] = v
	}
	if v := req.Params["EngineVersion"]; v != "" {
		r.Spec["EngineVersion"] = v
	}
	if v := req.Params["NumCacheNodes"]; v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			r.Spec["NumCacheNodes"] = n
		}
	}
	r.UpdatedAt = time.Now().UTC()

	if err := e.store.Put(ctx, r); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	item := cacheClusterItemFromResource(r, e.redis.Host(), e.redisPort())
	resp := ModifyCacheClusterResponse{
		RequestId:                "cloudia-elasticache",
		ModifyCacheClusterResult: ModifyCacheClusterResult{CacheCluster: item},
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// createReplicationGroup は CreateReplicationGroup アクションを処理します。
func (e *ElastiCacheService) createReplicationGroup(ctx context.Context, req service.Request) (service.Response, error) {
	groupID := req.Params["ReplicationGroupId"]
	if groupID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter ReplicationGroupId.")
	}

	desc := req.Params["ReplicationGroupDescription"]
	if desc == "" {
		desc = groupID
	}

	// 重複チェック
	if _, err := e.store.Get(ctx, kindReplicationGroup, groupID); err == nil {
		return errorResponse(http.StatusBadRequest, "ReplicationGroupAlreadyExistsFault",
			fmt.Sprintf("A replication group with the id %q already exists.", groupID))
	}

	now := time.Now().UTC()
	resource := &models.Resource{
		Kind:      kindReplicationGroup,
		ID:        groupID,
		Provider:  "aws",
		Service:   "elasticache",
		Region:    e.cfg.Region,
		Status:    statusAvailable,
		CreatedAt: now,
		UpdatedAt: now,
		Spec: map[string]interface{}{
			"Description": desc,
		},
	}

	if err := e.store.Put(ctx, resource); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	item := replicationGroupItemFromResource(resource, e.redis.Host(), e.redisPort())
	resp := CreateReplicationGroupResponse{
		RequestId:                    "cloudia-elasticache",
		CreateReplicationGroupResult: CreateReplicationGroupResult{ReplicationGroup: item},
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// deleteReplicationGroup は DeleteReplicationGroup アクションを処理します。
func (e *ElastiCacheService) deleteReplicationGroup(ctx context.Context, req service.Request) (service.Response, error) {
	groupID := req.Params["ReplicationGroupId"]
	if groupID == "" {
		return errorResponse(http.StatusBadRequest, "MissingParameter",
			"The request must contain the parameter ReplicationGroupId.")
	}

	r, err := e.store.Get(ctx, kindReplicationGroup, groupID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return errorResponse(http.StatusBadRequest, "ReplicationGroupNotFoundFault",
				fmt.Sprintf("Replication group %q not found.", groupID))
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	r.Status = statusDeleting
	r.UpdatedAt = time.Now().UTC()

	if err := e.store.Delete(ctx, kindReplicationGroup, groupID); err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	item := replicationGroupItemFromResource(r, e.redis.Host(), e.redisPort())
	resp := DeleteReplicationGroupResponse{
		RequestId:                    "cloudia-elasticache",
		DeleteReplicationGroupResult: DeleteReplicationGroupResult{ReplicationGroup: item},
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}

// describeReplicationGroups は DescribeReplicationGroups アクションを処理します。
func (e *ElastiCacheService) describeReplicationGroups(ctx context.Context, req service.Request) (service.Response, error) {
	groupID := req.Params["ReplicationGroupId"]

	var resources []*models.Resource

	if groupID != "" {
		r, err := e.store.Get(ctx, kindReplicationGroup, groupID)
		if err != nil {
			if errors.Is(err, models.ErrNotFound) {
				return errorResponse(http.StatusBadRequest, "ReplicationGroupNotFoundFault",
					fmt.Sprintf("Replication group %q not found.", groupID))
			}
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}
		resources = []*models.Resource{r}
	} else {
		var err error
		resources, err = e.store.List(ctx, kindReplicationGroup, state.Filter{})
		if err != nil {
			return service.Response{StatusCode: http.StatusInternalServerError}, err
		}
	}

	host := e.redis.Host()
	port := e.redisPort()

	items := make([]ReplicationGroupItem, 0, len(resources))
	for _, r := range resources {
		items = append(items, replicationGroupItemFromResource(r, host, port))
	}

	resp := DescribeReplicationGroupsResponse{
		RequestId: "cloudia-elasticache",
		DescribeReplicationGroupsResult: DescribeReplicationGroupsResult{
			ReplicationGroups: items,
		},
	}
	return awsprot.MarshalXMLResponse(http.StatusOK, resp, xmlNamespace)
}
