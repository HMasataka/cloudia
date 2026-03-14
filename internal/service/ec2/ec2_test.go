package ec2_test

import (
	"context"
	"strings"
	"testing"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/internal/config"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/service/ec2"
	"github.com/HMasataka/cloudia/internal/state"
	"go.uber.org/zap"
)

// stubContainerRunner は ContainerRunner のスタブ実装です。
type stubContainerRunner struct {
	containerIDCounter int
}

func (s *stubContainerRunner) RunContainer(_ context.Context, _ docker.ContainerConfig) (string, error) {
	s.containerIDCounter++
	return "container-stub-id", nil
}

func (s *stubContainerRunner) StopContainer(_ context.Context, _ string, _ *int) error {
	return nil
}

func (s *stubContainerRunner) RemoveContainer(_ context.Context, _ string) error {
	return nil
}

func (s *stubContainerRunner) StartContainer(_ context.Context, _ string) error {
	return nil
}

func (s *stubContainerRunner) PauseContainer(_ context.Context, _ string) error {
	return nil
}

func (s *stubContainerRunner) UnpauseContainer(_ context.Context, _ string) error {
	return nil
}

func (s *stubContainerRunner) InspectContainer(_ context.Context, _ string) (docker.ContainerInfo, error) {
	return docker.ContainerInfo{State: "running", IPAddress: "172.17.0.2"}, nil
}

func newTestEC2Service(t *testing.T) (*ec2.EC2Service, *state.MemoryStore, *stubContainerRunner) {
	t.Helper()
	store := state.NewMemoryStore()
	runner := &stubContainerRunner{}
	svc := ec2.NewEC2Service(config.AWSAuthConfig{}, zap.NewNop())
	if err := svc.Init(context.Background(), service.ServiceDeps{
		Store:        store,
		DockerClient: runner,
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return svc, store, runner
}

func handleEC2Request(t *testing.T, svc *ec2.EC2Service, action string, params map[string]string) service.Response {
	t.Helper()
	resp, err := svc.HandleRequest(context.Background(), service.Request{
		Provider: "aws",
		Service:  "ec2",
		Action:   action,
		Params:   params,
	})
	if err != nil {
		t.Fatalf("HandleRequest(%s): unexpected error: %v", action, err)
	}
	return resp
}

// TestEC2Service_Name は Name() が "ec2" を返すことを検証します。
func TestEC2Service_Name(t *testing.T) {
	svc := ec2.NewEC2Service(config.AWSAuthConfig{}, zap.NewNop())
	if got := svc.Name(); got != "ec2" {
		t.Errorf("Name() = %q, want %q", got, "ec2")
	}
}

// TestEC2Service_Provider は Provider() が "aws" を返すことを検証します。
func TestEC2Service_Provider(t *testing.T) {
	svc := ec2.NewEC2Service(config.AWSAuthConfig{}, zap.NewNop())
	if got := svc.Provider(); got != "aws" {
		t.Errorf("Provider() = %q, want %q", got, "aws")
	}
}

// TestEC2Service_SupportedActions は SupportedActions() が8アクションを含むことを検証します。
func TestEC2Service_SupportedActions(t *testing.T) {
	svc := ec2.NewEC2Service(config.AWSAuthConfig{}, zap.NewNop())
	actions := svc.SupportedActions()
	if len(actions) != 8 {
		t.Errorf("SupportedActions() returned %d actions, want 8: %v", len(actions), actions)
	}
	expected := []string{
		"RunInstances",
		"TerminateInstances",
		"DescribeInstances",
		"StartInstances",
		"StopInstances",
		"CreateTags",
		"DeleteTags",
		"DescribeTags",
	}
	actionSet := make(map[string]struct{}, len(actions))
	for _, a := range actions {
		actionSet[a] = struct{}{}
	}
	for _, e := range expected {
		if _, ok := actionSet[e]; !ok {
			t.Errorf("SupportedActions() missing action %q", e)
		}
	}
}

// TestEC2Service_RunInstances は ImageId + InstanceType 指定でインスタンスが起動することを検証します。
func TestEC2Service_RunInstances(t *testing.T) {
	svc, _, _ := newTestEC2Service(t)

	resp := handleEC2Request(t, svc, "RunInstances", map[string]string{
		"ImageId":      "ami-12345678",
		"InstanceType": "t2.micro",
		"MinCount":     "1",
		"MaxCount":     "1",
	})
	if resp.StatusCode != 200 {
		t.Fatalf("RunInstances: expected 200, got %d. body=%s", resp.StatusCode, resp.Body)
	}
	body := string(resp.Body)
	if !strings.Contains(body, "<instanceId>i-") {
		t.Errorf("RunInstances: response missing instanceId: %s", body)
	}
	if !strings.Contains(body, "<name>running</name>") {
		t.Errorf("RunInstances: response missing instanceState running: %s", body)
	}
	if !strings.Contains(body, "<code>16</code>") {
		t.Errorf("RunInstances: response missing state code 16: %s", body)
	}
}

// TestEC2Service_RunInstances_MissingImageId は ImageId 未指定で MissingParameter エラーを返すことを検証します。
func TestEC2Service_RunInstances_MissingImageId(t *testing.T) {
	svc, _, _ := newTestEC2Service(t)

	resp := handleEC2Request(t, svc, "RunInstances", map[string]string{
		"InstanceType": "t2.micro",
		"MinCount":     "1",
		"MaxCount":     "1",
	})
	if resp.StatusCode == 200 {
		t.Fatal("RunInstances without ImageId: expected error, got 200")
	}
	if !strings.Contains(string(resp.Body), "MissingParameter") {
		t.Errorf("RunInstances without ImageId: expected MissingParameter in response: %s", resp.Body)
	}
}

// TestEC2Service_DescribeInstances は全件取得を検証します。
func TestEC2Service_DescribeInstances(t *testing.T) {
	svc, _, _ := newTestEC2Service(t)

	// インスタンスを2つ起動
	handleEC2Request(t, svc, "RunInstances", map[string]string{
		"ImageId":      "ami-aaaaaaaa",
		"InstanceType": "t2.micro",
		"MinCount":     "1",
		"MaxCount":     "1",
	})
	handleEC2Request(t, svc, "RunInstances", map[string]string{
		"ImageId":      "ami-bbbbbbbb",
		"InstanceType": "t2.micro",
		"MinCount":     "1",
		"MaxCount":     "1",
	})

	resp := handleEC2Request(t, svc, "DescribeInstances", map[string]string{})
	if resp.StatusCode != 200 {
		t.Fatalf("DescribeInstances: expected 200, got %d. body=%s", resp.StatusCode, resp.Body)
	}
	body := string(resp.Body)
	if !strings.Contains(body, "ami-aaaaaaaa") {
		t.Errorf("DescribeInstances: missing ami-aaaaaaaa in response: %s", body)
	}
	if !strings.Contains(body, "ami-bbbbbbbb") {
		t.Errorf("DescribeInstances: missing ami-bbbbbbbb in response: %s", body)
	}
}

// TestEC2Service_DescribeInstances_Filter は instance-id フィルタを検証します。
func TestEC2Service_DescribeInstances_Filter(t *testing.T) {
	svc, _, _ := newTestEC2Service(t)

	// インスタンスを起動してIDを取得
	runResp := handleEC2Request(t, svc, "RunInstances", map[string]string{
		"ImageId":      "ami-12345678",
		"InstanceType": "t2.micro",
		"MinCount":     "1",
		"MaxCount":     "1",
	})
	body := string(runResp.Body)
	idStart := strings.Index(body, "<instanceId>")
	idEnd := strings.Index(body, "</instanceId>")
	if idStart < 0 || idEnd < 0 {
		t.Fatalf("could not extract instanceId from RunInstances response: %s", body)
	}
	instanceID := body[idStart+12 : idEnd]

	// 2つ目のインスタンス
	handleEC2Request(t, svc, "RunInstances", map[string]string{
		"ImageId":      "ami-99999999",
		"InstanceType": "t2.micro",
		"MinCount":     "1",
		"MaxCount":     "1",
	})

	// instance-id フィルタで絞り込み
	descResp := handleEC2Request(t, svc, "DescribeInstances", map[string]string{
		"Filter.1.Name":    "instance-id",
		"Filter.1.Value.1": instanceID,
	})
	descBody := string(descResp.Body)
	if !strings.Contains(descBody, instanceID) {
		t.Errorf("DescribeInstances filter: expected instanceID %s in body: %s", instanceID, descBody)
	}
	if strings.Contains(descBody, "ami-99999999") {
		t.Errorf("DescribeInstances filter: unexpected ami-99999999 in filtered response: %s", descBody)
	}
}

// TestEC2Service_TerminateInstances はコンテナ削除・状態が terminated/48 になることを検証します。
func TestEC2Service_TerminateInstances(t *testing.T) {
	svc, _, _ := newTestEC2Service(t)

	// インスタンス起動
	runResp := handleEC2Request(t, svc, "RunInstances", map[string]string{
		"ImageId":      "ami-12345678",
		"InstanceType": "t2.micro",
		"MinCount":     "1",
		"MaxCount":     "1",
	})
	body := string(runResp.Body)
	idStart := strings.Index(body, "<instanceId>")
	idEnd := strings.Index(body, "</instanceId>")
	if idStart < 0 || idEnd < 0 {
		t.Fatalf("could not extract instanceId: %s", body)
	}
	instanceID := body[idStart+12 : idEnd]

	// Terminate
	termResp := handleEC2Request(t, svc, "TerminateInstances", map[string]string{
		"InstanceId.1": instanceID,
	})
	if termResp.StatusCode != 200 {
		t.Fatalf("TerminateInstances: expected 200, got %d. body=%s", termResp.StatusCode, termResp.Body)
	}
	termBody := string(termResp.Body)
	if !strings.Contains(termBody, "<name>terminated</name>") {
		t.Errorf("TerminateInstances: missing terminated state: %s", termBody)
	}
	if !strings.Contains(termBody, "<code>48</code>") {
		t.Errorf("TerminateInstances: missing state code 48: %s", termBody)
	}
}

// TestEC2Service_TerminateInstances_Idempotent は同一 ID に2回呼んで両方成功することを検証します。
func TestEC2Service_TerminateInstances_Idempotent(t *testing.T) {
	svc, _, _ := newTestEC2Service(t)

	runResp := handleEC2Request(t, svc, "RunInstances", map[string]string{
		"ImageId":      "ami-12345678",
		"InstanceType": "t2.micro",
		"MinCount":     "1",
		"MaxCount":     "1",
	})
	body := string(runResp.Body)
	idStart := strings.Index(body, "<instanceId>")
	idEnd := strings.Index(body, "</instanceId>")
	if idStart < 0 || idEnd < 0 {
		t.Fatalf("could not extract instanceId: %s", body)
	}
	instanceID := body[idStart+12 : idEnd]

	// 1回目
	resp1 := handleEC2Request(t, svc, "TerminateInstances", map[string]string{
		"InstanceId.1": instanceID,
	})
	if resp1.StatusCode != 200 {
		t.Fatalf("TerminateInstances (1st): expected 200, got %d. body=%s", resp1.StatusCode, resp1.Body)
	}

	// 2回目（冪等）
	resp2 := handleEC2Request(t, svc, "TerminateInstances", map[string]string{
		"InstanceId.1": instanceID,
	})
	if resp2.StatusCode != 200 {
		t.Fatalf("TerminateInstances (2nd): expected 200, got %d. body=%s", resp2.StatusCode, resp2.Body)
	}
	if !strings.Contains(string(resp2.Body), "<name>terminated</name>") {
		t.Errorf("TerminateInstances idempotent: missing terminated state: %s", resp2.Body)
	}
}

// TestEC2Service_StopInstances は stopped/80 に遷移することを検証します。
func TestEC2Service_StopInstances(t *testing.T) {
	svc, _, _ := newTestEC2Service(t)

	runResp := handleEC2Request(t, svc, "RunInstances", map[string]string{
		"ImageId":      "ami-12345678",
		"InstanceType": "t2.micro",
		"MinCount":     "1",
		"MaxCount":     "1",
	})
	body := string(runResp.Body)
	idStart := strings.Index(body, "<instanceId>")
	idEnd := strings.Index(body, "</instanceId>")
	if idStart < 0 || idEnd < 0 {
		t.Fatalf("could not extract instanceId: %s", body)
	}
	instanceID := body[idStart+12 : idEnd]

	stopResp := handleEC2Request(t, svc, "StopInstances", map[string]string{
		"InstanceId.1": instanceID,
	})
	if stopResp.StatusCode != 200 {
		t.Fatalf("StopInstances: expected 200, got %d. body=%s", stopResp.StatusCode, stopResp.Body)
	}
	stopBody := string(stopResp.Body)
	if !strings.Contains(stopBody, "<name>stopped</name>") {
		t.Errorf("StopInstances: missing stopped state: %s", stopBody)
	}
	if !strings.Contains(stopBody, "<code>80</code>") {
		t.Errorf("StopInstances: missing state code 80: %s", stopBody)
	}
}

// TestEC2Service_StartInstances は stopped から running/16 に復帰することを検証します。
func TestEC2Service_StartInstances(t *testing.T) {
	svc, _, _ := newTestEC2Service(t)

	runResp := handleEC2Request(t, svc, "RunInstances", map[string]string{
		"ImageId":      "ami-12345678",
		"InstanceType": "t2.micro",
		"MinCount":     "1",
		"MaxCount":     "1",
	})
	body := string(runResp.Body)
	idStart := strings.Index(body, "<instanceId>")
	idEnd := strings.Index(body, "</instanceId>")
	if idStart < 0 || idEnd < 0 {
		t.Fatalf("could not extract instanceId: %s", body)
	}
	instanceID := body[idStart+12 : idEnd]

	// まず停止
	handleEC2Request(t, svc, "StopInstances", map[string]string{
		"InstanceId.1": instanceID,
	})

	// 起動
	startResp := handleEC2Request(t, svc, "StartInstances", map[string]string{
		"InstanceId.1": instanceID,
	})
	if startResp.StatusCode != 200 {
		t.Fatalf("StartInstances: expected 200, got %d. body=%s", startResp.StatusCode, startResp.Body)
	}
	startBody := string(startResp.Body)
	if !strings.Contains(startBody, "<name>running</name>") {
		t.Errorf("StartInstances: missing running state: %s", startBody)
	}
	if !strings.Contains(startBody, "<code>16</code>") {
		t.Errorf("StartInstances: missing state code 16: %s", startBody)
	}
}
