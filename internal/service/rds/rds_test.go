package rds

import (
	"context"
	"strings"
	"testing"

	"github.com/HMasataka/cloudia/internal/backend/rdb"
	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"go.uber.org/zap"
)

// newTestRDSService は Docker/MySQL 依存なしでサービスを構築します。
// RDBBackend はスタブ（固定ホスト/ポート）として初期化します。
func newTestRDSService(t *testing.T) (*RDSService, *state.MemoryStore) {
	t.Helper()
	store := state.NewMemoryStore()
	mysqlBackend := rdb.NewRDBBackendStub(&rdb.MySQLEngine{}, "localhost", "3306", zap.NewNop())
	svc := &RDSService{
		backends: map[string]*rdb.RDBBackend{
			"mysql": mysqlBackend,
		},
		store:  store,
		cfg:    config.AWSAuthConfig{},
		logger: zap.NewNop(),
	}
	return svc, store
}

// newTestRDSServiceWithPostgres は MySQL と PostgreSQL の両バックエンドを持つサービスを構築します。
// どちらのバックエンドもスタブ（固定ホスト/ポート）として初期化します。
func newTestRDSServiceWithPostgres(t *testing.T) (*RDSService, *state.MemoryStore) {
	t.Helper()
	store := state.NewMemoryStore()
	mysqlBackend := rdb.NewRDBBackendStub(&rdb.MySQLEngine{}, "localhost", "3306", zap.NewNop())
	postgresBackend := rdb.NewRDBBackendStub(&rdb.PostgreSQLEngine{}, "localhost", "5432", zap.NewNop())
	svc := &RDSService{
		backends: map[string]*rdb.RDBBackend{
			"mysql":    mysqlBackend,
			"postgres": postgresBackend,
		},
		store:  store,
		cfg:    config.AWSAuthConfig{},
		logger: zap.NewNop(),
	}
	return svc, store
}

func handleRDSRequest(t *testing.T, svc *RDSService, action string, params map[string]string) service.Response {
	t.Helper()
	resp, err := svc.HandleRequest(context.Background(), service.Request{
		Provider: "aws",
		Service:  "rds",
		Action:   action,
		Params:   params,
	})
	if err != nil {
		t.Fatalf("HandleRequest(%s): unexpected error: %v", action, err)
	}
	return resp
}

// TestRDSService_Name は Name() が "rds" を返すことを検証します。
func TestRDSService_Name(t *testing.T) {
	svc := NewRDSService(config.AWSAuthConfig{}, zap.NewNop())
	if got := svc.Name(); got != "rds" {
		t.Errorf("Name() = %q, want %q", got, "rds")
	}
}

// TestRDSService_Provider は Provider() が "aws" を返すことを検証します。
func TestRDSService_Provider(t *testing.T) {
	svc := NewRDSService(config.AWSAuthConfig{}, zap.NewNop())
	if got := svc.Provider(); got != "aws" {
		t.Errorf("Provider() = %q, want %q", got, "aws")
	}
}

// TestRDSService_CreateDBInstance_DescribeDBInstances は作成後に Describe で確認できることを検証します。
func TestRDSService_CreateDBInstance_DescribeDBInstances(t *testing.T) {
	svc, _ := newTestRDSService(t)

	createResp := handleRDSRequest(t, svc, "CreateDBInstance", map[string]string{
		"DBInstanceIdentifier": "test-db-1",
		"DBInstanceClass":      "db.t3.micro",
		"Engine":               "mysql",
		"MasterUserPassword":   "password123",
		"MasterUsername":       "admin",
	})
	if createResp.StatusCode != 200 {
		t.Fatalf("CreateDBInstance: expected 200, got %d. body=%s", createResp.StatusCode, createResp.Body)
	}
	body := string(createResp.Body)
	if !strings.Contains(body, "test-db-1") {
		t.Errorf("CreateDBInstance: response missing DBInstanceIdentifier: %s", body)
	}
	if !strings.Contains(body, "available") {
		t.Errorf("CreateDBInstance: response missing status available: %s", body)
	}

	// DescribeDBInstances で確認
	descResp := handleRDSRequest(t, svc, "DescribeDBInstances", map[string]string{
		"DBInstanceIdentifier": "test-db-1",
	})
	if descResp.StatusCode != 200 {
		t.Fatalf("DescribeDBInstances: expected 200, got %d. body=%s", descResp.StatusCode, descResp.Body)
	}
	descBody := string(descResp.Body)
	if !strings.Contains(descBody, "test-db-1") {
		t.Errorf("DescribeDBInstances: instance not found: %s", descBody)
	}
}

// TestRDSService_CreateDBInstance_MissingPassword は MasterUserPassword 未指定でエラーを返すことを検証します。
func TestRDSService_CreateDBInstance_MissingPassword(t *testing.T) {
	svc, _ := newTestRDSService(t)

	resp := handleRDSRequest(t, svc, "CreateDBInstance", map[string]string{
		"DBInstanceIdentifier": "test-db-missing-pw",
	})
	if resp.StatusCode == 200 {
		t.Fatal("CreateDBInstance without MasterUserPassword: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "InvalidParameterValue") {
		t.Errorf("CreateDBInstance without password: expected InvalidParameterValue: %s", resp.Body)
	}
}

// TestRDSService_CreateDBInstance_PasswordTooShort は短すぎるパスワードでバリデーションエラーを返すことを検証します。
func TestRDSService_CreateDBInstance_PasswordTooShort(t *testing.T) {
	svc, _ := newTestRDSService(t)

	resp := handleRDSRequest(t, svc, "CreateDBInstance", map[string]string{
		"DBInstanceIdentifier": "test-db-short-pw",
		"MasterUserPassword":   "short",
	})
	if resp.StatusCode == 200 {
		t.Fatal("CreateDBInstance with too short password: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "InvalidParameterValue") {
		t.Errorf("CreateDBInstance with short password: expected InvalidParameterValue: %s", resp.Body)
	}
}

// TestRDSService_CreateDBInstance_PasswordTooLong は長すぎるパスワードでバリデーションエラーを返すことを検証します。
func TestRDSService_CreateDBInstance_PasswordTooLong(t *testing.T) {
	svc, _ := newTestRDSService(t)

	// 42 文字のパスワード (上限 41 文字)
	longPw := strings.Repeat("a", 42)
	resp := handleRDSRequest(t, svc, "CreateDBInstance", map[string]string{
		"DBInstanceIdentifier": "test-db-long-pw",
		"MasterUserPassword":   longPw,
	})
	if resp.StatusCode == 200 {
		t.Fatal("CreateDBInstance with too long password: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "InvalidParameterValue") {
		t.Errorf("CreateDBInstance with long password: expected InvalidParameterValue: %s", resp.Body)
	}
}

// TestRDSService_CreateDBInstance_InvalidDBName は不正文字を含む DBName でバリデーションエラーを返すことを検証します。
func TestRDSService_CreateDBInstance_InvalidDBName(t *testing.T) {
	svc, _ := newTestRDSService(t)

	resp := handleRDSRequest(t, svc, "CreateDBInstance", map[string]string{
		"DBInstanceIdentifier": "test-db-invalid-name",
		"MasterUserPassword":   "password123",
		"DBName":               "invalid-db-name!", // ハイフンと感嘆符は不正
	})
	if resp.StatusCode == 200 {
		t.Fatal("CreateDBInstance with invalid DBName: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "InvalidParameterValue") {
		t.Errorf("CreateDBInstance with invalid DBName: expected InvalidParameterValue: %s", resp.Body)
	}
}

// TestRDSService_DeleteDBInstance は存在するインスタンスを削除できることを検証します。
func TestRDSService_DeleteDBInstance(t *testing.T) {
	svc, _ := newTestRDSService(t)

	handleRDSRequest(t, svc, "CreateDBInstance", map[string]string{
		"DBInstanceIdentifier": "db-to-delete",
		"MasterUserPassword":   "password123",
	})

	delResp := handleRDSRequest(t, svc, "DeleteDBInstance", map[string]string{
		"DBInstanceIdentifier": "db-to-delete",
	})
	if delResp.StatusCode != 200 {
		t.Fatalf("DeleteDBInstance: expected 200, got %d. body=%s", delResp.StatusCode, delResp.Body)
	}
	delBody := string(delResp.Body)
	if !strings.Contains(delBody, "deleting") {
		t.Errorf("DeleteDBInstance: expected deleting status: %s", delBody)
	}

	// 削除後は Describe でエラーになることを確認
	descResp := handleRDSRequest(t, svc, "DescribeDBInstances", map[string]string{
		"DBInstanceIdentifier": "db-to-delete",
	})
	if descResp.StatusCode == 200 {
		t.Errorf("DescribeDBInstances after delete: expected error, got 200: %s", descResp.Body)
	}
}

// TestRDSService_DeleteDBInstance_NotFound は存在しないインスタンスの削除でエラーを返すことを検証します。
func TestRDSService_DeleteDBInstance_NotFound(t *testing.T) {
	svc, _ := newTestRDSService(t)

	resp := handleRDSRequest(t, svc, "DeleteDBInstance", map[string]string{
		"DBInstanceIdentifier": "nonexistent-db",
	})
	if resp.StatusCode == 200 {
		t.Fatal("DeleteDBInstance nonexistent: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "DBInstanceNotFound") {
		t.Errorf("DeleteDBInstance nonexistent: expected DBInstanceNotFound: %s", resp.Body)
	}
}

// TestRDSService_ModifyDBInstance は DBInstanceClass を変更できることを検証します。
func TestRDSService_ModifyDBInstance(t *testing.T) {
	svc, _ := newTestRDSService(t)

	handleRDSRequest(t, svc, "CreateDBInstance", map[string]string{
		"DBInstanceIdentifier": "modify-db",
		"DBInstanceClass":      "db.t3.micro",
		"MasterUserPassword":   "password123",
	})

	modResp := handleRDSRequest(t, svc, "ModifyDBInstance", map[string]string{
		"DBInstanceIdentifier": "modify-db",
		"DBInstanceClass":      "db.r6g.large",
	})
	if modResp.StatusCode != 200 {
		t.Fatalf("ModifyDBInstance: expected 200, got %d. body=%s", modResp.StatusCode, modResp.Body)
	}
	modBody := string(modResp.Body)
	if !strings.Contains(modBody, "db.r6g.large") {
		t.Errorf("ModifyDBInstance: updated DBInstanceClass not found: %s", modBody)
	}
}

// TestRDSService_CreateDBSnapshot_DescribeDBSnapshots は作成後に Describe で確認できることを検証します。
func TestRDSService_CreateDBSnapshot_DescribeDBSnapshots(t *testing.T) {
	svc, _ := newTestRDSService(t)

	// DB インスタンスを先に作成
	handleRDSRequest(t, svc, "CreateDBInstance", map[string]string{
		"DBInstanceIdentifier": "snap-db",
		"MasterUserPassword":   "password123",
	})

	createResp := handleRDSRequest(t, svc, "CreateDBSnapshot", map[string]string{
		"DBSnapshotIdentifier": "snap-1",
		"DBInstanceIdentifier": "snap-db",
	})
	if createResp.StatusCode != 200 {
		t.Fatalf("CreateDBSnapshot: expected 200, got %d. body=%s", createResp.StatusCode, createResp.Body)
	}
	body := string(createResp.Body)
	if !strings.Contains(body, "snap-1") {
		t.Errorf("CreateDBSnapshot: response missing DBSnapshotIdentifier: %s", body)
	}

	// DescribeDBSnapshots で確認
	descResp := handleRDSRequest(t, svc, "DescribeDBSnapshots", map[string]string{
		"DBSnapshotIdentifier": "snap-1",
	})
	if descResp.StatusCode != 200 {
		t.Fatalf("DescribeDBSnapshots: expected 200, got %d. body=%s", descResp.StatusCode, descResp.Body)
	}
	descBody := string(descResp.Body)
	if !strings.Contains(descBody, "snap-1") {
		t.Errorf("DescribeDBSnapshots: snapshot not found: %s", descBody)
	}
}

// TestRDSService_CreateDBSnapshot_InstanceNotFound は存在しない DB インスタンスへのスナップショット作成でエラーを返すことを検証します。
func TestRDSService_CreateDBSnapshot_InstanceNotFound(t *testing.T) {
	svc, _ := newTestRDSService(t)

	resp := handleRDSRequest(t, svc, "CreateDBSnapshot", map[string]string{
		"DBSnapshotIdentifier": "snap-none",
		"DBInstanceIdentifier": "nonexistent-db",
	})
	if resp.StatusCode == 200 {
		t.Fatal("CreateDBSnapshot with nonexistent instance: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "DBInstanceNotFound") {
		t.Errorf("CreateDBSnapshot with nonexistent instance: expected DBInstanceNotFound: %s", resp.Body)
	}
}

// TestRDSService_DeleteDBSnapshot はスナップショットを削除できることを検証します。
func TestRDSService_DeleteDBSnapshot(t *testing.T) {
	svc, _ := newTestRDSService(t)

	// DB インスタンスとスナップショットを作成
	handleRDSRequest(t, svc, "CreateDBInstance", map[string]string{
		"DBInstanceIdentifier": "del-snap-db",
		"MasterUserPassword":   "password123",
	})
	handleRDSRequest(t, svc, "CreateDBSnapshot", map[string]string{
		"DBSnapshotIdentifier": "del-snap-1",
		"DBInstanceIdentifier": "del-snap-db",
	})

	delResp := handleRDSRequest(t, svc, "DeleteDBSnapshot", map[string]string{
		"DBSnapshotIdentifier": "del-snap-1",
	})
	if delResp.StatusCode != 200 {
		t.Fatalf("DeleteDBSnapshot: expected 200, got %d. body=%s", delResp.StatusCode, delResp.Body)
	}

	// 削除後は Describe でエラーになることを確認
	descResp := handleRDSRequest(t, svc, "DescribeDBSnapshots", map[string]string{
		"DBSnapshotIdentifier": "del-snap-1",
	})
	if descResp.StatusCode == 200 {
		t.Errorf("DescribeDBSnapshots after delete: expected error, got 200: %s", descResp.Body)
	}
}

// TestRDSService_DeleteDBSnapshot_NotFound は存在しないスナップショットの削除でエラーを返すことを検証します。
func TestRDSService_DeleteDBSnapshot_NotFound(t *testing.T) {
	svc, _ := newTestRDSService(t)

	resp := handleRDSRequest(t, svc, "DeleteDBSnapshot", map[string]string{
		"DBSnapshotIdentifier": "nonexistent-snap",
	})
	if resp.StatusCode == 200 {
		t.Fatal("DeleteDBSnapshot nonexistent: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "DBSnapshotNotFound") {
		t.Errorf("DeleteDBSnapshot nonexistent: expected DBSnapshotNotFound: %s", resp.Body)
	}
}

// TestRDSService_Postgres_CreateDescribeDelete は Engine: "postgres" での CRUD を検証します。
func TestRDSService_Postgres_CreateDescribeDelete(t *testing.T) {
	svc, _ := newTestRDSServiceWithPostgres(t)

	// CreateDBInstance with postgres engine
	createResp := handleRDSRequest(t, svc, "CreateDBInstance", map[string]string{
		"DBInstanceIdentifier": "pg-db-1",
		"DBInstanceClass":      "db.t3.micro",
		"Engine":               "postgres",
		"MasterUserPassword":   "password123",
		"MasterUsername":       "pgadmin",
	})
	if createResp.StatusCode != 200 {
		t.Fatalf("CreateDBInstance (postgres): expected 200, got %d. body=%s", createResp.StatusCode, createResp.Body)
	}
	createBody := string(createResp.Body)
	if !strings.Contains(createBody, "pg-db-1") {
		t.Errorf("CreateDBInstance (postgres): response missing DBInstanceIdentifier: %s", createBody)
	}
	if !strings.Contains(createBody, "available") {
		t.Errorf("CreateDBInstance (postgres): response missing status available: %s", createBody)
	}
	if !strings.Contains(createBody, "postgres") {
		t.Errorf("CreateDBInstance (postgres): response missing engine postgres: %s", createBody)
	}

	// DescribeDBInstances で確認
	descResp := handleRDSRequest(t, svc, "DescribeDBInstances", map[string]string{
		"DBInstanceIdentifier": "pg-db-1",
	})
	if descResp.StatusCode != 200 {
		t.Fatalf("DescribeDBInstances (postgres): expected 200, got %d. body=%s", descResp.StatusCode, descResp.Body)
	}
	descBody := string(descResp.Body)
	if !strings.Contains(descBody, "pg-db-1") {
		t.Errorf("DescribeDBInstances (postgres): instance not found: %s", descBody)
	}
	if !strings.Contains(descBody, "postgres") {
		t.Errorf("DescribeDBInstances (postgres): expected engine postgres in response: %s", descBody)
	}

	// DeleteDBInstance
	delResp := handleRDSRequest(t, svc, "DeleteDBInstance", map[string]string{
		"DBInstanceIdentifier": "pg-db-1",
	})
	if delResp.StatusCode != 200 {
		t.Fatalf("DeleteDBInstance (postgres): expected 200, got %d. body=%s", delResp.StatusCode, delResp.Body)
	}
	if !strings.Contains(string(delResp.Body), "deleting") {
		t.Errorf("DeleteDBInstance (postgres): expected deleting status: %s", delResp.Body)
	}
}

// TestRDSService_InvalidEngine は Engine: "oracle" で InvalidParameterValue エラーを返すことを検証します。
func TestRDSService_InvalidEngine(t *testing.T) {
	svc, _ := newTestRDSService(t)

	resp := handleRDSRequest(t, svc, "CreateDBInstance", map[string]string{
		"DBInstanceIdentifier": "oracle-db",
		"Engine":               "oracle",
		"MasterUserPassword":   "password123",
	})
	if resp.StatusCode == 200 {
		t.Fatal("CreateDBInstance with oracle engine: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "InvalidParameterValue") {
		t.Errorf("CreateDBInstance with oracle engine: expected InvalidParameterValue: %s", resp.Body)
	}
}

// TestRDSService_MySQL_Postgres_Coexistence は同一サービスで MySQL と PostgreSQL インスタンスが共存できることを検証します。
func TestRDSService_MySQL_Postgres_Coexistence(t *testing.T) {
	svc, _ := newTestRDSServiceWithPostgres(t)

	// MySQL インスタンスを作成
	mysqlResp := handleRDSRequest(t, svc, "CreateDBInstance", map[string]string{
		"DBInstanceIdentifier": "coexist-mysql",
		"Engine":               "mysql",
		"MasterUserPassword":   "password123",
	})
	if mysqlResp.StatusCode != 200 {
		t.Fatalf("CreateDBInstance (mysql coexist): expected 200, got %d. body=%s", mysqlResp.StatusCode, mysqlResp.Body)
	}

	// PostgreSQL インスタンスを作成
	pgResp := handleRDSRequest(t, svc, "CreateDBInstance", map[string]string{
		"DBInstanceIdentifier": "coexist-postgres",
		"Engine":               "postgres",
		"MasterUserPassword":   "password123",
	})
	if pgResp.StatusCode != 200 {
		t.Fatalf("CreateDBInstance (postgres coexist): expected 200, got %d. body=%s", pgResp.StatusCode, pgResp.Body)
	}

	// 両方を DescribeDBInstances で確認
	allResp := handleRDSRequest(t, svc, "DescribeDBInstances", map[string]string{})
	if allResp.StatusCode != 200 {
		t.Fatalf("DescribeDBInstances (all): expected 200, got %d. body=%s", allResp.StatusCode, allResp.Body)
	}
	allBody := string(allResp.Body)
	if !strings.Contains(allBody, "coexist-mysql") {
		t.Errorf("DescribeDBInstances: mysql instance not found: %s", allBody)
	}
	if !strings.Contains(allBody, "coexist-postgres") {
		t.Errorf("DescribeDBInstances: postgres instance not found: %s", allBody)
	}
}
