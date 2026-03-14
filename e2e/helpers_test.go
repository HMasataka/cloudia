//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)


// skipIfToolNotFound はツールが未インストールの場合にテストをスキップします。
func skipIfToolNotFound(t *testing.T, tool string) {
	t.Helper()
	if _, err := exec.LookPath(tool); err != nil {
		t.Skipf("tool %q not found in PATH, skipping: %v", tool, err)
	}
}

// skipIfDockerNotAvailable はDockerが利用できない場合にテストをスキップします。
func skipIfDockerNotAvailable(t *testing.T) {
	t.Helper()
	if globalServer.dockerClient == nil {
		t.Skip("docker not available, skipping test")
	}
}

// skipIfServiceUnhealthy はサービスのヘルスエンドポイントが200を返さない場合にテストをスキップします。
// DockerベースのサービスはMinioなどの依存サービスが起動していないと500を返す場合があります。
func skipIfServiceUnhealthy(t *testing.T, probeURL string) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, probeURL, nil)
	if err != nil {
		t.Skipf("service health probe failed to create request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer test-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("service health probe failed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		t.Skipf("service returned %d, service may be unavailable (Docker image pull may have failed)", resp.StatusCode)
	}
}

// runAWSCLI はAWS CLIコマンドを実行してstdoutを返します。
// --endpoint-urlは自動的に付与されます。
func runAWSCLI(t *testing.T, args ...string) string {
	t.Helper()
	skipIfToolNotFound(t, "aws")

	fullArgs := append(args, "--endpoint-url", globalServer.baseURL)
	cmd := exec.Command("aws", fullArgs...)
	cmd.Env = append(os.Environ(),
		"AWS_ACCESS_KEY_ID=test",
		"AWS_SECRET_ACCESS_KEY=test",
		"AWS_DEFAULT_REGION=us-east-1",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("aws %s failed: %v\nstdout: %s\nstderr: %s",
			strings.Join(args, " "), err, stdout.String(), stderr.String())
	}

	return stdout.String()
}

// runAWSCLIBestEffort はAWS CLIコマンドを実行してエラーの場合は空文字を返します。
func runAWSCLIBestEffort(args ...string) string {
	if _, err := exec.LookPath("aws"); err != nil {
		return ""
	}
	fullArgs := append(args, "--endpoint-url", globalServer.baseURL)
	cmd := exec.Command("aws", fullArgs...)
	cmd.Env = append(os.Environ(),
		"AWS_ACCESS_KEY_ID=test",
		"AWS_SECRET_ACCESS_KEY=test",
		"AWS_DEFAULT_REGION=us-east-1",
	)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return ""
	}
	return stdout.String()
}

// runAWSCLIIgnoreError はAWS CLIコマンドを実行してエラーを無視します（クリーンアップ用）。
func runAWSCLIIgnoreError(args ...string) {
	if _, err := exec.LookPath("aws"); err != nil {
		return
	}
	fullArgs := append(args, "--endpoint-url", globalServer.baseURL)
	cmd := exec.Command("aws", fullArgs...)
	cmd.Env = append(os.Environ(),
		"AWS_ACCESS_KEY_ID=test",
		"AWS_SECRET_ACCESS_KEY=test",
		"AWS_DEFAULT_REGION=us-east-1",
	)
	_ = cmd.Run()
}

// runGCloudCLI はgcloud CLIコマンドを実行してstdoutを返します。
func runGCloudCLI(t *testing.T, args ...string) string {
	t.Helper()
	skipIfToolNotFound(t, "gcloud")

	cmd := exec.Command("gcloud", args...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("CLOUDSDK_API_ENDPOINT_OVERRIDES_STORAGE=%s/storage/v1/", globalServer.baseURL),
		fmt.Sprintf("CLOUDSDK_API_ENDPOINT_OVERRIDES_COMPUTE=%s/compute/v1/", globalServer.baseURL),
		"CLOUDSDK_CORE_PROJECT=cloudia-local",
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("gcloud %s failed: %v\nstdout: %s\nstderr: %s",
			strings.Join(args, " "), err, stdout.String(), stderr.String())
	}

	return stdout.String()
}

// httpDo はHTTPリクエストを実行してResponseを返します。
func httpDo(t *testing.T, method, path string, body io.Reader) *http.Response {
	t.Helper()
	url := globalServer.baseURL + path
	req, err := http.NewRequestWithContext(context.Background(), method, url, body)
	if err != nil {
		t.Fatalf("http.NewRequest %s %s: %v", method, url, err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http.Do %s %s: %v", method, url, err)
	}
	return resp
}

// cleanupOrphans はテスト後の孤立リソースをクリーンアップします。
func cleanupOrphans(t *testing.T) {
	t.Helper()
	if globalServer.dockerClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := globalServer.dockerClient.CleanupOrphans(ctx); err != nil {
		t.Logf("cleanup orphans warning: %v", err)
	}
}

// measureLatency はリクエストのレイテンシを計測します。
func measureLatency(t *testing.T, fn func()) time.Duration {
	t.Helper()
	start := time.Now()
	fn()
	return time.Since(start)
}

// percentile はソート済みスライスのパーセンタイル値を返します。
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

// sortDurations はtime.Durationのスライスをソートします。
func sortDurations(durations []time.Duration) {
	for i := 1; i < len(durations); i++ {
		for j := i; j > 0 && durations[j] < durations[j-1]; j-- {
			durations[j], durations[j-1] = durations[j-1], durations[j]
		}
	}
}

// openFileForWrite はファイルを書き込みモードで開きます（新規作成）。
func openFileForWrite(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
}
