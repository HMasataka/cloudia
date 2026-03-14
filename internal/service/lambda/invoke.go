package lambda

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/HMasataka/cloudia/internal/backend/docker"
	"github.com/HMasataka/cloudia/pkg/models"
	"go.uber.org/zap"
)

const (
	rieContainerPort    = "8080"
	rieInvocationsPath  = "/2015-03-31/functions/function/invocations"
	healthCheckInterval = time.Second
	healthCheckMaxTries = 30
)

// startupEntry はコンテナの単一起動を保証するためのエントリです。
type startupEntry struct {
	mu sync.Mutex
}

// startupPool は関数名 → 起動エントリのマップです。
var (
	startupMu   sync.Mutex
	startupPool = make(map[string]*startupEntry)
)

// getOrCreateStartupEntry は関数名に対応する startupEntry を返します。
func getOrCreateStartupEntry(functionName string) *startupEntry {
	startupMu.Lock()
	defer startupMu.Unlock()
	if e, ok := startupPool[functionName]; ok {
		return e
	}
	e := &startupEntry{}
	startupPool[functionName] = e
	return e
}

// deleteStartupEntry は startupPool から関数名のエントリを削除します。
func deleteStartupEntry(functionName string) {
	startupMu.Lock()
	defer startupMu.Unlock()
	delete(startupPool, functionName)
}

// handleInvoke は Lambda Invoke API を処理します。
// 同期 (RequestResponse) と非同期 (Event) の両方をサポートします。
func (s *LambdaService) handleInvoke(w http.ResponseWriter, r *http.Request, functionName string) {
	ctx := r.Context()

	// 関数の存在確認
	resource, err := s.store.Get(ctx, resourceKind, functionName)
	if err != nil {
		if isNotFound(err) {
			writeError(w, http.StatusNotFound, "ResourceNotFoundException",
				fmt.Sprintf("Function not found: %s", functionARN(functionName)))
			return
		}
		writeError(w, http.StatusInternalServerError, "ServiceException", err.Error())
		return
	}

	// 非同期 Invoke の判定
	invocationType := r.Header.Get("X-Amz-Invocation-Type")
	if invocationType == "Event" {
		// ボディを事前に読み取っておく
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "ServiceException", "failed to read request body: "+err.Error())
			return
		}
		w.WriteHeader(http.StatusAccepted)
		// バックグラウンドで実行
		go func() {
			bgCtx := context.Background()
			if _, err := s.invokeFunction(bgCtx, functionName, resource, payload); err != nil {
				s.logger.Warn("lambda: async invoke failed",
					zap.String("function", functionName),
					zap.Error(err),
				)
			}
		}()
		return
	}

	// 同期 Invoke
	payload, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ServiceException", "failed to read request body: "+err.Error())
		return
	}

	// タイムアウト設定
	timeoutSec := defaultTimeout
	if t, ok := resource.Spec["Timeout"]; ok {
		switch v := t.(type) {
		case int:
			timeoutSec = v
		case float64:
			timeoutSec = int(v)
		}
	}

	invokeCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	respBody, err := s.invokeFunction(invokeCtx, functionName, resource, payload)
	if err != nil {
		if errors.Is(invokeCtx.Err(), context.DeadlineExceeded) {
			writeError(w, http.StatusGatewayTimeout, "FunctionError",
				fmt.Sprintf("Task timed out after %d seconds", timeoutSec))
			return
		}
		writeError(w, http.StatusInternalServerError, "ServiceException", err.Error())
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(http.StatusOK)
	w.Write(respBody) //nolint:errcheck
}

// invokeFunction はコンテナを起動(または再利用)して RIE に Invoke を送信します。
func (s *LambdaService) invokeFunction(ctx context.Context, functionName string, resource *models.Resource, payload []byte) ([]byte, error) {
	// コンテナ数上限確認 (初回起動前のみ意味を持つ)
	s.poolMu.Lock()
	_, exists := s.pool[functionName]
	s.poolMu.Unlock()

	if !exists {
		if err := s.limits.CheckContainerLimit(ctx); err != nil {
			return nil, fmt.Errorf("container limit exceeded: %w", err)
		}
	}

	baseURL, err := s.ensureContainer(ctx, functionName, resource)
	if err != nil {
		return nil, fmt.Errorf("ensure container: %w", err)
	}

	// RIE に Invoke を送信
	return s.callRIE(ctx, baseURL, payload)
}

// ensureContainer はコンテナが起動済みであれば baseURL を返し、
// 未起動であれば起動してからヘルスチェック後に baseURL を返します。
// 並行 Invoke による多重起動は startupEntry で防止します。
func (s *LambdaService) ensureContainer(ctx context.Context, functionName string, resource *models.Resource) (string, error) {
	// 既存エントリを確認
	s.poolMu.Lock()
	entry, exists := s.pool[functionName]
	if exists && entry.status == "ready" {
		url := entry.baseURL
		s.poolMu.Unlock()
		return url, nil
	}
	s.poolMu.Unlock()

	// 単一起動保証
	se := getOrCreateStartupEntry(functionName)
	se.mu.Lock()
	defer se.mu.Unlock()

	// ロック取得後に再確認
	s.poolMu.Lock()
	entry, exists = s.pool[functionName]
	if exists && entry.status == "ready" {
		url := entry.baseURL
		s.poolMu.Unlock()
		return url, nil
	}
	s.poolMu.Unlock()

	// コンテナを起動
	containerID, hostPort, baseURL, err := s.startContainer(ctx, functionName, resource)
	if err != nil {
		deleteStartupEntry(functionName)
		return "", err
	}

	// プールに登録
	s.poolMu.Lock()
	s.pool[functionName] = &containerEntry{
		containerID: containerID,
		hostPort:    hostPort,
		baseURL:     baseURL,
		status:      "ready",
	}
	s.poolMu.Unlock()

	return baseURL, nil
}

// startContainer はコンテナを起動してヘルスチェックを行います。
func (s *LambdaService) startContainer(ctx context.Context, functionName string, resource *models.Resource) (containerID string, hostPort int, baseURL string, err error) {
	runtime := ""
	if v, ok := resource.Spec["Runtime"]; ok {
		if sv, ok := v.(string); ok {
			runtime = sv
		}
	}

	image, err := resolveRuntimeImage(runtime)
	if err != nil {
		return "", 0, "", err
	}

	// ポート割り当て
	hostPort, err = s.ports.Allocate(0, "lambda-"+functionName)
	if err != nil {
		return "", 0, "", fmt.Errorf("allocate port: %w", err)
	}

	// 環境変数構築
	envVars := map[string]string{
		"AWS_LAMBDA_FUNCTION_NAME": functionName,
	}
	if envRaw, ok := resource.Spec["Environment"]; ok {
		if envMap, ok := envRaw.(map[string]interface{}); ok {
			if varsRaw, ok := envMap["Variables"]; ok {
				if varsMap, ok := varsRaw.(map[string]interface{}); ok {
					for k, v := range varsMap {
						if sv, ok := v.(string); ok {
							envVars[k] = sv
						}
					}
				}
			}
		}
	}

	// コンテナ起動
	containerID, err = s.docker.RunContainer(ctx, docker.ContainerConfig{
		Image: image,
		Name:  "cloudia-lambda-" + functionName,
		Labels: map[string]string{
			docker.LabelService: "lambda-" + functionName,
		},
		Env:  envVars,
		Ports: map[string]string{
			rieContainerPort: strconv.Itoa(hostPort),
		},
		Network: lambdaNetwork,
		Binds:   []string{functionDir(functionName) + ":/var/task:ro"},
	})
	if err != nil {
		s.ports.Release(hostPort)
		return "", 0, "", fmt.Errorf("run container: %w", err)
	}

	baseURL = fmt.Sprintf("http://localhost:%d", hostPort)

	// ヘルスチェック
	if err := s.waitRIEHealthy(ctx, baseURL); err != nil {
		// ヘルスチェック失敗時はコンテナを停止・削除
		stopCtx := context.Background()
		_ = s.docker.StopContainer(stopCtx, containerID, nil)
		_ = s.docker.RemoveContainer(stopCtx, containerID)
		s.ports.Release(hostPort)
		return "", 0, "", fmt.Errorf("health check: %w", err)
	}

	return containerID, hostPort, baseURL, nil
}

// waitRIEHealthy は RIE の 8080 ポートが応答するまで待機します。
func (s *LambdaService) waitRIEHealthy(ctx context.Context, baseURL string) error {
	url := baseURL + rieInvocationsPath
	for i := 0; i < healthCheckMaxTries; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader([]byte("{}")))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", contentType)

		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
			// RIE は正常時 200 を返す。4xx も RIE が起動していれば OK
			if resp.StatusCode < http.StatusInternalServerError {
				return nil
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(healthCheckInterval):
		}
	}
	return fmt.Errorf("RIE did not become ready after %d attempts", healthCheckMaxTries)
}

// callRIE は RIE に Invoke リクエストを送信してレスポンスボディを返します。
func (s *LambdaService) callRIE(ctx context.Context, baseURL string, payload []byte) ([]byte, error) {
	url := baseURL + rieInvocationsPath

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call RIE: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read RIE response: %w", err)
	}

	return body, nil
}

// isNotFound は error が models.ErrNotFound かどうか判定します。
func isNotFound(err error) bool {
	return errors.Is(err, models.ErrNotFound)
}
