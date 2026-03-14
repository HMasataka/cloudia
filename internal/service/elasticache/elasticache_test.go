package elasticache

import (
	"context"
	"strings"
	"testing"

	"github.com/HMasataka/cloudia/internal/backend/redis"
	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"go.uber.org/zap"
)

// newTestElastiCacheService は Docker/Redis 依存なしでサービスを構築します。
// redis.RedisBackend は空のまま（Init を呼ばない）で store を直接注入します。
func newTestElastiCacheService(t *testing.T) (*ElastiCacheService, *state.MemoryStore) {
	t.Helper()
	store := state.NewMemoryStore()
	svc := &ElastiCacheService{
		redis:  redis.NewRedisBackend(zap.NewNop()),
		store:  store,
		cfg:    config.AWSAuthConfig{},
		logger: zap.NewNop(),
	}
	return svc, store
}

func handleElastiCacheRequest(t *testing.T, svc *ElastiCacheService, action string, params map[string]string) service.Response {
	t.Helper()
	resp, err := svc.HandleRequest(context.Background(), service.Request{
		Provider: "aws",
		Service:  "elasticache",
		Action:   action,
		Params:   params,
	})
	if err != nil {
		t.Fatalf("HandleRequest(%s): unexpected error: %v", action, err)
	}
	return resp
}

// TestElastiCacheService_Name は Name() が "elasticache" を返すことを検証します。
func TestElastiCacheService_Name(t *testing.T) {
	svc := NewElastiCacheService(config.AWSAuthConfig{}, zap.NewNop())
	if got := svc.Name(); got != "elasticache" {
		t.Errorf("Name() = %q, want %q", got, "elasticache")
	}
}

// TestElastiCacheService_Provider は Provider() が "aws" を返すことを検証します。
func TestElastiCacheService_Provider(t *testing.T) {
	svc := NewElastiCacheService(config.AWSAuthConfig{}, zap.NewNop())
	if got := svc.Provider(); got != "aws" {
		t.Errorf("Provider() = %q, want %q", got, "aws")
	}
}

// TestElastiCacheService_CreateCacheCluster_DescribeCacheClusters は作成後に Describe で確認できることを検証します。
func TestElastiCacheService_CreateCacheCluster_DescribeCacheClusters(t *testing.T) {
	svc, _ := newTestElastiCacheService(t)

	createResp := handleElastiCacheRequest(t, svc, "CreateCacheCluster", map[string]string{
		"CacheClusterId": "test-cluster-1",
		"Engine":         "redis",
		"CacheNodeType":  "cache.t3.micro",
	})
	if createResp.StatusCode != 200 {
		t.Fatalf("CreateCacheCluster: expected 200, got %d. body=%s", createResp.StatusCode, createResp.Body)
	}
	body := string(createResp.Body)
	if !strings.Contains(body, "test-cluster-1") {
		t.Errorf("CreateCacheCluster: response missing CacheClusterId: %s", body)
	}
	if !strings.Contains(body, "available") {
		t.Errorf("CreateCacheCluster: response missing status available: %s", body)
	}

	// DescribeCacheClusters で確認
	descResp := handleElastiCacheRequest(t, svc, "DescribeCacheClusters", map[string]string{
		"CacheClusterId": "test-cluster-1",
	})
	if descResp.StatusCode != 200 {
		t.Fatalf("DescribeCacheClusters: expected 200, got %d. body=%s", descResp.StatusCode, descResp.Body)
	}
	descBody := string(descResp.Body)
	if !strings.Contains(descBody, "test-cluster-1") {
		t.Errorf("DescribeCacheClusters: cluster not found: %s", descBody)
	}
}

// TestElastiCacheService_CreateCacheCluster_MissingID は CacheClusterId 未指定でエラーを返すことを検証します。
func TestElastiCacheService_CreateCacheCluster_MissingID(t *testing.T) {
	svc, _ := newTestElastiCacheService(t)

	resp := handleElastiCacheRequest(t, svc, "CreateCacheCluster", map[string]string{})
	if resp.StatusCode == 200 {
		t.Fatal("CreateCacheCluster without CacheClusterId: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "MissingParameter") {
		t.Errorf("CreateCacheCluster without CacheClusterId: expected MissingParameter: %s", resp.Body)
	}
}

// TestElastiCacheService_DeleteCacheCluster_Exists は存在するクラスターを削除できることを検証します。
func TestElastiCacheService_DeleteCacheCluster_Exists(t *testing.T) {
	svc, _ := newTestElastiCacheService(t)

	handleElastiCacheRequest(t, svc, "CreateCacheCluster", map[string]string{
		"CacheClusterId": "to-delete",
	})

	delResp := handleElastiCacheRequest(t, svc, "DeleteCacheCluster", map[string]string{
		"CacheClusterId": "to-delete",
	})
	if delResp.StatusCode != 200 {
		t.Fatalf("DeleteCacheCluster: expected 200, got %d. body=%s", delResp.StatusCode, delResp.Body)
	}
	delBody := string(delResp.Body)
	if !strings.Contains(delBody, "deleting") {
		t.Errorf("DeleteCacheCluster: expected deleting status: %s", delBody)
	}

	// 削除後は Describe でエラーになることを確認
	descResp := handleElastiCacheRequest(t, svc, "DescribeCacheClusters", map[string]string{
		"CacheClusterId": "to-delete",
	})
	if descResp.StatusCode == 200 {
		t.Errorf("DescribeCacheClusters after delete: expected error, got 200: %s", descResp.Body)
	}
}

// TestElastiCacheService_DeleteCacheCluster_NotFound は存在しないクラスターの削除でエラーを返すことを検証します。
func TestElastiCacheService_DeleteCacheCluster_NotFound(t *testing.T) {
	svc, _ := newTestElastiCacheService(t)

	resp := handleElastiCacheRequest(t, svc, "DeleteCacheCluster", map[string]string{
		"CacheClusterId": "nonexistent",
	})
	if resp.StatusCode == 200 {
		t.Fatal("DeleteCacheCluster nonexistent: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "CacheClusterNotFound") {
		t.Errorf("DeleteCacheCluster nonexistent: expected CacheClusterNotFound: %s", resp.Body)
	}
}

// TestElastiCacheService_ModifyCacheCluster は CacheNodeType を変更できることを検証します。
func TestElastiCacheService_ModifyCacheCluster(t *testing.T) {
	svc, _ := newTestElastiCacheService(t)

	handleElastiCacheRequest(t, svc, "CreateCacheCluster", map[string]string{
		"CacheClusterId": "modify-me",
		"CacheNodeType":  "cache.t3.micro",
	})

	modResp := handleElastiCacheRequest(t, svc, "ModifyCacheCluster", map[string]string{
		"CacheClusterId": "modify-me",
		"CacheNodeType":  "cache.r6g.large",
	})
	if modResp.StatusCode != 200 {
		t.Fatalf("ModifyCacheCluster: expected 200, got %d. body=%s", modResp.StatusCode, modResp.Body)
	}
	modBody := string(modResp.Body)
	if !strings.Contains(modBody, "cache.r6g.large") {
		t.Errorf("ModifyCacheCluster: updated CacheNodeType not found: %s", modBody)
	}
}

// TestElastiCacheService_ModifyCacheCluster_NotFound は存在しないクラスターの変更でエラーを返すことを検証します。
func TestElastiCacheService_ModifyCacheCluster_NotFound(t *testing.T) {
	svc, _ := newTestElastiCacheService(t)

	resp := handleElastiCacheRequest(t, svc, "ModifyCacheCluster", map[string]string{
		"CacheClusterId": "nonexistent",
		"CacheNodeType":  "cache.r6g.large",
	})
	if resp.StatusCode == 200 {
		t.Fatal("ModifyCacheCluster nonexistent: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "CacheClusterNotFound") {
		t.Errorf("ModifyCacheCluster nonexistent: expected CacheClusterNotFound: %s", resp.Body)
	}
}

// TestElastiCacheService_CreateReplicationGroup_DescribeReplicationGroups は作成後に Describe で確認できることを検証します。
func TestElastiCacheService_CreateReplicationGroup_DescribeReplicationGroups(t *testing.T) {
	svc, _ := newTestElastiCacheService(t)

	createResp := handleElastiCacheRequest(t, svc, "CreateReplicationGroup", map[string]string{
		"ReplicationGroupId":          "test-rg-1",
		"ReplicationGroupDescription": "test replication group",
	})
	if createResp.StatusCode != 200 {
		t.Fatalf("CreateReplicationGroup: expected 200, got %d. body=%s", createResp.StatusCode, createResp.Body)
	}
	body := string(createResp.Body)
	if !strings.Contains(body, "test-rg-1") {
		t.Errorf("CreateReplicationGroup: response missing ReplicationGroupId: %s", body)
	}

	// DescribeReplicationGroups で確認
	descResp := handleElastiCacheRequest(t, svc, "DescribeReplicationGroups", map[string]string{
		"ReplicationGroupId": "test-rg-1",
	})
	if descResp.StatusCode != 200 {
		t.Fatalf("DescribeReplicationGroups: expected 200, got %d. body=%s", descResp.StatusCode, descResp.Body)
	}
	descBody := string(descResp.Body)
	if !strings.Contains(descBody, "test-rg-1") {
		t.Errorf("DescribeReplicationGroups: group not found: %s", descBody)
	}
}

// TestElastiCacheService_CreateReplicationGroup_MissingID は ReplicationGroupId 未指定でエラーを返すことを検証します。
func TestElastiCacheService_CreateReplicationGroup_MissingID(t *testing.T) {
	svc, _ := newTestElastiCacheService(t)

	resp := handleElastiCacheRequest(t, svc, "CreateReplicationGroup", map[string]string{})
	if resp.StatusCode == 200 {
		t.Fatal("CreateReplicationGroup without ReplicationGroupId: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "MissingParameter") {
		t.Errorf("CreateReplicationGroup without ReplicationGroupId: expected MissingParameter: %s", resp.Body)
	}
}

// TestElastiCacheService_DeleteReplicationGroup は存在するグループを削除できることを検証します。
func TestElastiCacheService_DeleteReplicationGroup(t *testing.T) {
	svc, _ := newTestElastiCacheService(t)

	handleElastiCacheRequest(t, svc, "CreateReplicationGroup", map[string]string{
		"ReplicationGroupId":          "rg-to-delete",
		"ReplicationGroupDescription": "to be deleted",
	})

	delResp := handleElastiCacheRequest(t, svc, "DeleteReplicationGroup", map[string]string{
		"ReplicationGroupId": "rg-to-delete",
	})
	if delResp.StatusCode != 200 {
		t.Fatalf("DeleteReplicationGroup: expected 200, got %d. body=%s", delResp.StatusCode, delResp.Body)
	}
	delBody := string(delResp.Body)
	if !strings.Contains(delBody, "deleting") {
		t.Errorf("DeleteReplicationGroup: expected deleting status: %s", delBody)
	}

	// 削除後は Describe でエラーになることを確認
	descResp := handleElastiCacheRequest(t, svc, "DescribeReplicationGroups", map[string]string{
		"ReplicationGroupId": "rg-to-delete",
	})
	if descResp.StatusCode == 200 {
		t.Errorf("DescribeReplicationGroups after delete: expected error, got 200: %s", descResp.Body)
	}
}

// TestElastiCacheService_DeleteReplicationGroup_NotFound は存在しないグループの削除でエラーを返すことを検証します。
func TestElastiCacheService_DeleteReplicationGroup_NotFound(t *testing.T) {
	svc, _ := newTestElastiCacheService(t)

	resp := handleElastiCacheRequest(t, svc, "DeleteReplicationGroup", map[string]string{
		"ReplicationGroupId": "nonexistent-rg",
	})
	if resp.StatusCode == 200 {
		t.Fatal("DeleteReplicationGroup nonexistent: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "ReplicationGroupNotFoundFault") {
		t.Errorf("DeleteReplicationGroup nonexistent: expected ReplicationGroupNotFoundFault: %s", resp.Body)
	}
}

// TestElastiCacheService_DescribeCacheClusters_All は全件取得を検証します。
func TestElastiCacheService_DescribeCacheClusters_All(t *testing.T) {
	svc, _ := newTestElastiCacheService(t)

	handleElastiCacheRequest(t, svc, "CreateCacheCluster", map[string]string{
		"CacheClusterId": "cluster-a",
	})
	handleElastiCacheRequest(t, svc, "CreateCacheCluster", map[string]string{
		"CacheClusterId": "cluster-b",
	})

	descResp := handleElastiCacheRequest(t, svc, "DescribeCacheClusters", map[string]string{})
	if descResp.StatusCode != 200 {
		t.Fatalf("DescribeCacheClusters: expected 200, got %d. body=%s", descResp.StatusCode, descResp.Body)
	}
	descBody := string(descResp.Body)
	if !strings.Contains(descBody, "cluster-a") {
		t.Errorf("DescribeCacheClusters: missing cluster-a: %s", descBody)
	}
	if !strings.Contains(descBody, "cluster-b") {
		t.Errorf("DescribeCacheClusters: missing cluster-b: %s", descBody)
	}
}
