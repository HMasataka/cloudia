//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
)

// TestE2E_DynamoDB_BasicOperations はDynamoDBの基本的なテーブル・アイテム操作をテストします。
func TestE2E_DynamoDB_BasicOperations(t *testing.T) {
	skipIfToolNotFound(t, "aws")
	skipIfDockerNotAvailable(t)
	// DynamoDBはDynamoDB Localコンテナに依存するため、利用可能か確認する
	// ListTablesがエラーを返す場合はDynamoDB Localが起動していないためスキップする
	if out := runAWSCLIBestEffort("dynamodb", "list-tables"); out == "" {
		t.Skip("DynamoDB service unavailable (Docker image pull may have failed)")
	}

	t.Cleanup(func() { cleanupOrphans(t) })

	const tableName = "e2e-test-table"

	// テーブル作成
	t.Run("CreateTable", func(t *testing.T) {
		out := runAWSCLI(t, "dynamodb", "create-table",
			"--table-name", tableName,
			"--attribute-definitions", "AttributeName=id,AttributeType=S",
			"--key-schema", "AttributeName=id,KeyType=HASH",
			"--billing-mode", "PAY_PER_REQUEST",
			"--output", "text",
			"--query", "TableDescription.TableName",
		)
		got := strings.TrimSpace(out)
		if got != tableName {
			t.Errorf("CreateTable: expected %q, got %q", tableName, got)
		}
	})

	// テーブル一覧
	t.Run("ListTables", func(t *testing.T) {
		out := runAWSCLI(t, "dynamodb", "list-tables",
			"--output", "text",
		)
		if !strings.Contains(out, tableName) {
			t.Errorf("ListTables: expected %q in output, got: %s", tableName, out)
		}
	})

	// テーブル詳細
	t.Run("DescribeTable", func(t *testing.T) {
		out := runAWSCLI(t, "dynamodb", "describe-table",
			"--table-name", tableName,
			"--output", "text",
			"--query", "Table.TableName",
		)
		got := strings.TrimSpace(out)
		if got != tableName {
			t.Errorf("DescribeTable: expected %q, got %q", tableName, got)
		}
	})

	// アイテム追加
	t.Run("PutItem", func(t *testing.T) {
		runAWSCLI(t, "dynamodb", "put-item",
			"--table-name", tableName,
			"--item", `{"id":{"S":"item1"},"value":{"S":"hello"}}`,
		)
	})

	// アイテム取得
	t.Run("GetItem", func(t *testing.T) {
		out := runAWSCLI(t, "dynamodb", "get-item",
			"--table-name", tableName,
			"--key", `{"id":{"S":"item1"}}`,
			"--output", "text",
		)
		if !strings.Contains(out, "hello") {
			t.Errorf("GetItem: expected 'hello' in output, got: %s", out)
		}
	})

	// アイテム削除
	t.Run("DeleteItem", func(t *testing.T) {
		runAWSCLI(t, "dynamodb", "delete-item",
			"--table-name", tableName,
			"--key", `{"id":{"S":"item1"}}`,
		)
	})

	// テーブル削除
	t.Run("DeleteTable", func(t *testing.T) {
		runAWSCLI(t, "dynamodb", "delete-table",
			"--table-name", tableName,
		)
	})

	t.Cleanup(func() {
		runAWSCLIIgnoreError("dynamodb", "delete-table", "--table-name", tableName)
	})
}
