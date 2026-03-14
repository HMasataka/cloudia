//go:build e2e

package e2e_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestE2E_Terraform_AWS はhashicorp/awsプロバイダを使ったTerraformのapply/destroyをテストします。
func TestE2E_Terraform_AWS(t *testing.T) {
	skipIfToolNotFound(t, "terraform")
	skipIfDockerNotAvailable(t)

	t.Cleanup(func() { cleanupOrphans(t) })

	// テストファイルのディレクトリからtfDirを計算する
	_, testFile, _, _ := runtime.Caller(0)
	tfDir := filepath.Join(filepath.Dir(testFile), "tftest", "aws")

	// terraform init
	t.Run("Init", func(t *testing.T) {
		runTerraform(t, tfDir, "init", "-no-color", "-upgrade")
	})

	// terraform apply
	// S3 サービス(MinIO)が利用できない場合はスキップ
	t.Run("Apply", func(t *testing.T) {
		if out := runAWSCLIBestEffort("s3api", "list-buckets"); out == "" {
			t.Skip("S3 service unavailable (MinIO container may not have started), skipping Terraform apply")
		}
		runTerraform(t, tfDir, "apply",
			"-auto-approve",
			"-no-color",
			fmt.Sprintf("-var=cloudia_endpoint=%s", globalServer.baseURL),
		)
	})

	// terraform destroy
	t.Cleanup(func() {
		runTerraformIgnoreError(tfDir, "destroy",
			"-auto-approve",
			"-no-color",
			fmt.Sprintf("-var=cloudia_endpoint=%s", globalServer.baseURL),
		)
	})

	// Apply後にリソースが存在することを確認
	t.Run("VerifyS3Bucket", func(t *testing.T) {
		skipIfToolNotFound(t, "aws")
		if out := runAWSCLIBestEffort("s3api", "list-buckets"); out == "" {
			t.Skip("S3 service unavailable")
		}
		out := runAWSCLI(t, "s3", "ls")
		if !strings.Contains(out, "tf-e2e-test-bucket") {
			t.Errorf("expected tf-e2e-test-bucket in S3 list, got: %s", out)
		}
	})
}

// runTerraform はterraformコマンドを実行します。
func runTerraform(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("terraform", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"TF_LOG=",
		"AWS_ACCESS_KEY_ID=test",
		"AWS_SECRET_ACCESS_KEY=test",
		"AWS_DEFAULT_REGION=us-east-1",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("terraform %s failed: %v\nstdout: %s\nstderr: %s",
			strings.Join(args, " "), err, stdout.String(), stderr.String())
	}

	return stdout.String()
}

// runTerraformIgnoreError はterraformコマンドを実行してエラーを無視します（クリーンアップ用）。
func runTerraformIgnoreError(dir string, args ...string) {
	if _, err := exec.LookPath("terraform"); err != nil {
		return
	}
	cmd := exec.Command("terraform", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"TF_LOG=",
		"AWS_ACCESS_KEY_ID=test",
		"AWS_SECRET_ACCESS_KEY=test",
		"AWS_DEFAULT_REGION=us-east-1",
	)
	_ = cmd.Run()
}
