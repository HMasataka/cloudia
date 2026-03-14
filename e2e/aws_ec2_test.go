//go:build e2e

package e2e_test

import (
	"strings"
	"testing"
)

// TestE2E_EC2_BasicOperations はEC2の基本的なインスタンス操作をテストします。
func TestE2E_EC2_BasicOperations(t *testing.T) {
	skipIfToolNotFound(t, "aws")
	skipIfDockerNotAvailable(t)

	t.Cleanup(func() { cleanupOrphans(t) })

	var instanceID string

	// インスタンス起動
	// EC2はDockerコンテナでインスタンスをバックする。Docker Hubへのアクセスができない場合はスキップする。
	t.Run("RunInstances", func(t *testing.T) {
		skipIfToolNotFound(t, "aws")
		out := runAWSCLIBestEffort("ec2", "run-instances",
			"--image-id", "ami-12345678",
			"--instance-type", "t2.micro",
			"--count", "1",
			"--output", "text",
			"--query", "Instances[0].InstanceId",
		)
		if out == "" {
			t.Skip("RunInstances failed (Docker image pull may have failed), skipping EC2 tests")
		}
		instanceID = strings.TrimSpace(out)
		if instanceID == "" {
			t.Skip("RunInstances: empty instance ID (Docker may not be available)")
		}
		t.Logf("started instance: %s", instanceID)
	})

	if instanceID == "" {
		t.Skip("no instance ID available, skipping remaining tests")
	}

	// インスタンス一覧
	t.Run("DescribeInstances", func(t *testing.T) {
		out := runAWSCLI(t, "ec2", "describe-instances",
			"--instance-ids", instanceID,
			"--output", "text",
			"--query", "Reservations[0].Instances[0].InstanceId",
		)
		got := strings.TrimSpace(out)
		if got != instanceID {
			t.Errorf("DescribeInstances: expected %q, got %q", instanceID, got)
		}
	})

	// インスタンス停止 (コンテナのPauseをサポートしていない環境ではスキップ)
	t.Run("StopInstances", func(t *testing.T) {
		out := runAWSCLIBestEffort("ec2", "stop-instances", "--instance-ids", instanceID)
		if out == "" {
			t.Skip("StopInstances failed (container pause may not be supported in this environment)")
		}
	})

	// インスタンス終了
	t.Run("TerminateInstances", func(t *testing.T) {
		runAWSCLI(t, "ec2", "terminate-instances", "--instance-ids", instanceID)
	})

	t.Cleanup(func() {
		if instanceID != "" {
			runAWSCLIIgnoreError("ec2", "terminate-instances", "--instance-ids", instanceID)
		}
	})
}
