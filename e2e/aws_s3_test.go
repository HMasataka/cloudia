//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
)

// TestE2E_S3_BasicOperations はS3の基本的なバケット・オブジェクト操作をテストします。
func TestE2E_S3_BasicOperations(t *testing.T) {
	skipIfToolNotFound(t, "aws")
	skipIfDockerNotAvailable(t)
	// S3はMinIOコンテナに依存するため、利用可能か確認する
	// list-bucketsは空でも{"Buckets":[]}を返すので、空文字の場合のみスキップ
	if out := runAWSCLIBestEffort("s3api", "list-buckets"); out == "" {
		t.Skip("S3 service unavailable (MinIO container may not have started)")
	}

	t.Cleanup(func() { cleanupOrphans(t) })

	const bucketName = "e2e-test-bucket-s3"

	// バケット作成
	t.Run("CreateBucket", func(t *testing.T) {
		out := runAWSCLI(t, "s3", "mb", "s3://"+bucketName)
		if !strings.Contains(out, bucketName) {
			t.Errorf("CreateBucket: expected bucket name in output, got: %s", out)
		}
	})

	// バケット一覧
	t.Run("ListBuckets", func(t *testing.T) {
		out := runAWSCLI(t, "s3", "ls")
		if !strings.Contains(out, bucketName) {
			t.Errorf("ListBuckets: expected %q in output, got: %s", bucketName, out)
		}
	})

	// オブジェクトのアップロード
	t.Run("PutObject", func(t *testing.T) {
		// 一時ファイル作成
		tmpFile := t.TempDir() + "/testfile.txt"
		writeTestFile(t, tmpFile, "hello from e2e test")
		out := runAWSCLI(t, "s3", "cp", tmpFile, "s3://"+bucketName+"/testfile.txt")
		if !strings.Contains(out, "testfile.txt") {
			t.Errorf("PutObject: expected testfile.txt in output, got: %s", out)
		}
	})

	// オブジェクト一覧
	t.Run("ListObjects", func(t *testing.T) {
		out := runAWSCLI(t, "s3", "ls", "s3://"+bucketName)
		if !strings.Contains(out, "testfile.txt") {
			t.Errorf("ListObjects: expected testfile.txt in output, got: %s", out)
		}
	})

	// オブジェクト削除
	t.Run("DeleteObject", func(t *testing.T) {
		runAWSCLI(t, "s3", "rm", "s3://"+bucketName+"/testfile.txt")
	})

	// バケット削除
	t.Run("DeleteBucket", func(t *testing.T) {
		runAWSCLI(t, "s3", "rb", "s3://"+bucketName)
	})

	t.Cleanup(func() {
		runAWSCLIIgnoreError("s3", "rb", "s3://"+bucketName, "--force")
	})
}

// writeTestFile はテスト用ファイルを書き込みます。
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := writeFile(path, []byte(content)); err != nil {
		t.Fatalf("writeTestFile %s: %v", path, err)
	}
}

// writeFile はファイルにバイト列を書き込みます。
func writeFile(path string, data []byte) error {
	f, err := openFileForWrite(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}
