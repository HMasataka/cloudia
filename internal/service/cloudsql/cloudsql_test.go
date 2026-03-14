package cloudsql

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"go.uber.org/zap"
)

// newTestCloudSQLService は Registry/MySQL 依存なしでサービスを構築します。
// store を直接注入し、dbHosts/dbPorts はダミー値を使います。
func newTestCloudSQLService(t *testing.T) (*CloudSQLService, *state.MemoryStore) {
	t.Helper()
	store := state.NewMemoryStore()
	svc := &CloudSQLService{
		store:  store,
		logger: zap.NewNop(),
		dbHosts: map[string]string{
			"mysql": "localhost",
		},
		dbPorts: map[string]string{
			"mysql": "3306",
		},
	}
	return svc, store
}

func handleCloudSQLRequest(t *testing.T, svc *CloudSQLService, method, action string, body []byte) service.Response {
	t.Helper()
	resp, err := svc.HandleRequest(context.Background(), service.Request{
		Provider: "gcp",
		Service:  "cloudsql",
		Action:   action,
		Method:   method,
		Body:     body,
	})
	if err != nil {
		t.Fatalf("HandleRequest(%s %s): unexpected error: %v", method, action, err)
	}
	return resp
}

// TestCloudSQLService_Name は Name() が "cloudsql" を返すことを検証します。
func TestCloudSQLService_Name(t *testing.T) {
	svc := NewCloudSQLService(zap.NewNop())
	if got := svc.Name(); got != "cloudsql" {
		t.Errorf("Name() = %q, want %q", got, "cloudsql")
	}
}

// TestCloudSQLService_Provider は Provider() が "gcp" を返すことを検証します。
func TestCloudSQLService_Provider(t *testing.T) {
	svc := NewCloudSQLService(zap.NewNop())
	if got := svc.Provider(); got != "gcp" {
		t.Errorf("Provider() = %q, want %q", got, "gcp")
	}
}

// TestCloudSQLService_Insert_Get_List_Delete は Insert → Get → List → Delete の正常系を検証します。
func TestCloudSQLService_Insert_Get_List_Delete(t *testing.T) {
	svc, _ := newTestCloudSQLService(t)

	project := "my-project"
	instanceName := "my-mysql"

	// Insert
	insertBody, _ := json.Marshal(map[string]interface{}{
		"name":            instanceName,
		"databaseVersion": "MYSQL_8_0",
		"region":          "us-central1",
		"settings": map[string]interface{}{
			"tier": "db-f1-micro",
		},
	})
	insertResp := handleCloudSQLRequest(t, svc,
		http.MethodPost,
		"projects/"+project+"/instances",
		insertBody,
	)
	if insertResp.StatusCode != 200 {
		t.Fatalf("Insert: expected 200, got %d. body=%s", insertResp.StatusCode, insertResp.Body)
	}
	insertBodyStr := string(insertResp.Body)
	if !strings.Contains(insertBodyStr, instanceName) {
		t.Errorf("Insert: response missing instance name: %s", insertBodyStr)
	}
	if !strings.Contains(insertBodyStr, "DONE") {
		t.Errorf("Insert: expected operation status DONE: %s", insertBodyStr)
	}

	// Get
	getResp := handleCloudSQLRequest(t, svc,
		http.MethodGet,
		"projects/"+project+"/instances/"+instanceName,
		nil,
	)
	if getResp.StatusCode != 200 {
		t.Fatalf("Get: expected 200, got %d. body=%s", getResp.StatusCode, getResp.Body)
	}
	getBodyStr := string(getResp.Body)
	if !strings.Contains(getBodyStr, instanceName) {
		t.Errorf("Get: instance name not found in response: %s", getBodyStr)
	}
	if !strings.Contains(getBodyStr, "RUNNABLE") {
		t.Errorf("Get: expected RUNNABLE state: %s", getBodyStr)
	}

	// List
	listResp := handleCloudSQLRequest(t, svc,
		http.MethodGet,
		"projects/"+project+"/instances",
		nil,
	)
	if listResp.StatusCode != 200 {
		t.Fatalf("List: expected 200, got %d. body=%s", listResp.StatusCode, listResp.Body)
	}
	listBodyStr := string(listResp.Body)
	if !strings.Contains(listBodyStr, instanceName) {
		t.Errorf("List: instance name not found in response: %s", listBodyStr)
	}

	// Delete
	delResp := handleCloudSQLRequest(t, svc,
		http.MethodDelete,
		"projects/"+project+"/instances/"+instanceName,
		nil,
	)
	if delResp.StatusCode != 200 {
		t.Fatalf("Delete: expected 200, got %d. body=%s", delResp.StatusCode, delResp.Body)
	}
	delBodyStr := string(delResp.Body)
	if !strings.Contains(delBodyStr, "DELETE") {
		t.Errorf("Delete: expected operationType DELETE: %s", delBodyStr)
	}

	// 削除後は Get でエラーになることを確認
	getAfterDelResp := handleCloudSQLRequest(t, svc,
		http.MethodGet,
		"projects/"+project+"/instances/"+instanceName,
		nil,
	)
	if getAfterDelResp.StatusCode == 200 {
		t.Errorf("Get after delete: expected error, got 200: %s", getAfterDelResp.Body)
	}
	if !strings.Contains(string(getAfterDelResp.Body), "NOT_FOUND") {
		t.Errorf("Get after delete: expected NOT_FOUND: %s", getAfterDelResp.Body)
	}
}

// TestCloudSQLService_Get_NotFound は存在しないインスタンスの Get で NOT_FOUND を返すことを検証します。
func TestCloudSQLService_Get_NotFound(t *testing.T) {
	svc, _ := newTestCloudSQLService(t)

	resp := handleCloudSQLRequest(t, svc,
		http.MethodGet,
		"projects/my-project/instances/nonexistent",
		nil,
	)
	if resp.StatusCode == 200 {
		t.Fatal("Get nonexistent instance: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "NOT_FOUND") {
		t.Errorf("Get nonexistent instance: expected NOT_FOUND: %s", resp.Body)
	}
}

// TestCloudSQLService_Delete_NotFound は存在しないインスタンスの Delete で NOT_FOUND を返すことを検証します。
func TestCloudSQLService_Delete_NotFound(t *testing.T) {
	svc, _ := newTestCloudSQLService(t)

	resp := handleCloudSQLRequest(t, svc,
		http.MethodDelete,
		"projects/my-project/instances/nonexistent",
		nil,
	)
	if resp.StatusCode == 200 {
		t.Fatal("Delete nonexistent instance: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "NOT_FOUND") {
		t.Errorf("Delete nonexistent instance: expected NOT_FOUND: %s", resp.Body)
	}
}

// TestCloudSQLService_InvalidPath は不正なパスで INVALID_ARGUMENT を返すことを検証します。
func TestCloudSQLService_InvalidPath(t *testing.T) {
	svc, _ := newTestCloudSQLService(t)

	resp := handleCloudSQLRequest(t, svc,
		http.MethodGet,
		"invalid/path",
		nil,
	)
	if resp.StatusCode == 200 {
		t.Fatal("Invalid path: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "INVALID_ARGUMENT") {
		t.Errorf("Invalid path: expected INVALID_ARGUMENT: %s", resp.Body)
	}
}

// TestCloudSQLService_Insert_Duplicate は同名インスタンスの重複作成で ALREADY_EXISTS を返すことを検証します。
func TestCloudSQLService_Insert_Duplicate(t *testing.T) {
	svc, _ := newTestCloudSQLService(t)

	project := "my-project"
	instanceName := "dup-mysql"

	insertBody, _ := json.Marshal(map[string]interface{}{
		"name": instanceName,
	})

	// 1回目の作成
	handleCloudSQLRequest(t, svc,
		http.MethodPost,
		"projects/"+project+"/instances",
		insertBody,
	)

	// 2回目は ALREADY_EXISTS
	dupResp := handleCloudSQLRequest(t, svc,
		http.MethodPost,
		"projects/"+project+"/instances",
		insertBody,
	)
	if dupResp.StatusCode == 200 {
		t.Fatal("Insert duplicate: expected error, got 200")
	}
	if !strings.Contains(string(dupResp.Body), "ALREADY_EXISTS") {
		t.Errorf("Insert duplicate: expected ALREADY_EXISTS: %s", dupResp.Body)
	}
}

// newTestCloudSQLServiceWithPostgres は MySQL と PostgreSQL の両バックエンドを持つサービスを構築します。
func newTestCloudSQLServiceWithPostgres(t *testing.T) (*CloudSQLService, *state.MemoryStore) {
	t.Helper()
	store := state.NewMemoryStore()
	svc := &CloudSQLService{
		store:  store,
		logger: zap.NewNop(),
		dbHosts: map[string]string{
			"mysql":    "localhost",
			"postgres": "localhost",
		},
		dbPorts: map[string]string{
			"mysql":    "3306",
			"postgres": "5432",
		},
	}
	return svc, store
}

// TestCloudSQLService_Postgres_Insert_Get は POSTGRES_16 での insertInstance と getInstance を検証します。
func TestCloudSQLService_Postgres_Insert_Get(t *testing.T) {
	svc, _ := newTestCloudSQLServiceWithPostgres(t)

	project := "my-project"
	instanceName := "my-postgres"

	// Insert
	insertBody, _ := json.Marshal(map[string]interface{}{
		"name":            instanceName,
		"databaseVersion": "POSTGRES_16",
		"region":          "us-central1",
		"settings": map[string]interface{}{
			"tier": "db-custom-2-8192",
		},
	})
	insertResp := handleCloudSQLRequest(t, svc,
		http.MethodPost,
		"projects/"+project+"/instances",
		insertBody,
	)
	if insertResp.StatusCode != 200 {
		t.Fatalf("Insert (POSTGRES_16): expected 200, got %d. body=%s", insertResp.StatusCode, insertResp.Body)
	}
	insertBodyStr := string(insertResp.Body)
	if !strings.Contains(insertBodyStr, instanceName) {
		t.Errorf("Insert (POSTGRES_16): response missing instance name: %s", insertBodyStr)
	}
	if !strings.Contains(insertBodyStr, "DONE") {
		t.Errorf("Insert (POSTGRES_16): expected operation status DONE: %s", insertBodyStr)
	}

	// Get
	getResp := handleCloudSQLRequest(t, svc,
		http.MethodGet,
		"projects/"+project+"/instances/"+instanceName,
		nil,
	)
	if getResp.StatusCode != 200 {
		t.Fatalf("Get (POSTGRES_16): expected 200, got %d. body=%s", getResp.StatusCode, getResp.Body)
	}
	getBodyStr := string(getResp.Body)
	if !strings.Contains(getBodyStr, instanceName) {
		t.Errorf("Get (POSTGRES_16): instance name not found in response: %s", getBodyStr)
	}
	if !strings.Contains(getBodyStr, "RUNNABLE") {
		t.Errorf("Get (POSTGRES_16): expected RUNNABLE state: %s", getBodyStr)
	}
	if !strings.Contains(getBodyStr, "POSTGRES_16") {
		t.Errorf("Get (POSTGRES_16): expected databaseVersion POSTGRES_16: %s", getBodyStr)
	}
	// PostgreSQL バックエンドの IP アドレスが使用されていることを確認
	if !strings.Contains(getBodyStr, "localhost") {
		t.Errorf("Get (POSTGRES_16): expected ipAddress from postgres backend: %s", getBodyStr)
	}
}

// TestCloudSQLService_List_Empty は空のリストが返ることを検証します。
func TestCloudSQLService_List_Empty(t *testing.T) {
	svc, _ := newTestCloudSQLService(t)

	resp := handleCloudSQLRequest(t, svc,
		http.MethodGet,
		"projects/my-project/instances",
		nil,
	)
	if resp.StatusCode != 200 {
		t.Fatalf("List empty: expected 200, got %d. body=%s", resp.StatusCode, resp.Body)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		t.Fatalf("List empty: failed to parse JSON: %v", err)
	}
	// items キーが nil または空スライスであることを確認
	items, _ := result["items"]
	if items != nil {
		t.Errorf("List empty: expected no items, got: %v", items)
	}
}
