//go:build e2e

package e2e_test

import (
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
)

// TestE2E_Terraform_GCP はhashicorp/googleプロバイダを使ったTerraformのapply/destroyをテストします。
func TestE2E_Terraform_GCP(t *testing.T) {
	skipIfToolNotFound(t, "terraform")
	skipIfDockerNotAvailable(t)

	t.Cleanup(func() { cleanupOrphans(t) })

	_, testFile, _, _ := runtime.Caller(0)
	tfDir := filepath.Join(filepath.Dir(testFile), "tftest", "gcp")

	// terraform init
	t.Run("Init", func(t *testing.T) {
		runTerraform(t, tfDir, "init", "-no-color", "-upgrade")
	})

	// terraform apply
	// GCS サービス(MinIO)が利用できない場合はスキップ
	t.Run("Apply", func(t *testing.T) {
		if out := runAWSCLIBestEffort("s3api", "list-buckets"); out == "" {
			t.Skip("GCS/S3 service unavailable (MinIO container may not have started), skipping Terraform GCP apply")
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
}
