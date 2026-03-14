package lambda

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	awsprotocol "github.com/HMasataka/cloudia/internal/protocol/aws"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

const (
	defaultTimeout    = 3
	defaultMemorySize = 128
	awsPartition      = "aws"
	awsRegion         = "us-east-1"
	awsAccountID      = "000000000000"

	// maxZipFileBytes は base64 デコード後の zip ファイルサイズ上限 (50MB) です。
	maxZipFileBytes = 50 * 1024 * 1024
	// maxExtractedBytes は zip 展開後のファイル累計サイズ上限 (250MB) です。
	maxExtractedBytes = 250 * 1024 * 1024
	// maxRequestBodyBytes はリクエストボディサイズ上限 (70MB = base64 の 50MB 相当) です。
	maxRequestBodyBytes = 70 * 1024 * 1024
)

// functionDir は関数コードの展開先ディレクトリパスを返します。
func functionDir(functionName string) string {
	return filepath.Join(lambdaCodeDir, functionName)
}

// functionARN は Lambda 関数の ARN を生成します。
func functionARN(functionName string) string {
	return awsprotocol.FormatARN(awsPartition, "lambda", awsRegion, awsAccountID, "function:"+functionName)
}

// handleCreateFunction は CreateFunction API を処理します。
func (s *LambdaService) handleCreateFunction(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	r.Body = io.NopCloser(io.LimitReader(r.Body, maxRequestBodyBytes))

	var req CreateFunctionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "InvalidParameterValueException", "invalid request body: "+err.Error())
		return
	}

	if req.FunctionName == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValueException", "FunctionName is required")
		return
	}

	if req.Runtime == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValueException", "Runtime is required")
		return
	}

	// ランタイム検証
	if _, err := resolveRuntimeImage(req.Runtime); err != nil {
		writeError(w, http.StatusBadRequest, "InvalidRuntimeException",
			fmt.Sprintf("unsupported runtime: %s", req.Runtime))
		return
	}

	// 重複チェック
	existing, err := s.store.Get(ctx, resourceKind, req.FunctionName)
	if err != nil && !errors.Is(err, models.ErrNotFound) {
		writeError(w, http.StatusInternalServerError, "ServiceException", err.Error())
		return
	}
	if existing != nil {
		writeError(w, http.StatusConflict, "ResourceConflictException",
			fmt.Sprintf("Function already exist: %s", functionARN(req.FunctionName)))
		return
	}

	// ZipFile のデコードと展開
	codeSize, codeSha256, err := deployZipFile(req.FunctionName, req.Code.ZipFile)
	if err != nil {
		writeError(w, http.StatusBadRequest, "InvalidParameterValueException", "failed to deploy code: "+err.Error())
		return
	}

	// デフォルト値の補完
	timeout := req.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	memorySize := req.MemorySize
	if memorySize == 0 {
		memorySize = defaultMemorySize
	}

	arn := functionARN(req.FunctionName)
	now := time.Now().UTC().Format(time.RFC3339)

	spec := map[string]interface{}{
		"FunctionName": req.FunctionName,
		"FunctionArn":  arn,
		"Runtime":      req.Runtime,
		"Role":         req.Role,
		"Handler":      req.Handler,
		"CodeSize":     codeSize,
		"CodeSha256":   codeSha256,
		"Description":  req.Description,
		"Timeout":      timeout,
		"MemorySize":   memorySize,
		"LastModified": now,
		"State":        "Active",
	}
	if req.Environment != nil {
		envVars := make(map[string]interface{}, len(req.Environment.Variables))
		for k, v := range req.Environment.Variables {
			envVars[k] = v
		}
		spec["Environment"] = map[string]interface{}{"Variables": envVars}
	}

	resource := &models.Resource{
		Kind:      resourceKind,
		ID:        req.FunctionName,
		Provider:  "aws",
		Service:   "lambda",
		Region:    awsRegion,
		Tags:      req.Tags,
		Spec:      spec,
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := s.store.Put(ctx, resource); err != nil {
		writeError(w, http.StatusInternalServerError, "ServiceException", err.Error())
		return
	}

	cfg := specToFunctionConfiguration(spec)
	writeJSON(w, http.StatusCreated, cfg)
}

// handleGetFunction は GetFunction API を処理します。
func (s *LambdaService) handleGetFunction(w http.ResponseWriter, r *http.Request, functionName string) {
	ctx := r.Context()

	resource, err := s.store.Get(ctx, resourceKind, functionName)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			writeError(w, http.StatusNotFound, "ResourceNotFoundException",
				fmt.Sprintf("Function not found: %s", functionARN(functionName)))
			return
		}
		writeError(w, http.StatusInternalServerError, "ServiceException", err.Error())
		return
	}

	cfg := specToFunctionConfiguration(resource.Spec)
	resp := GetFunctionResponse{Configuration: cfg}
	writeJSON(w, http.StatusOK, resp)
}

// handleDeleteFunction は DeleteFunction API を処理します。
func (s *LambdaService) handleDeleteFunction(w http.ResponseWriter, r *http.Request, functionName string) {
	ctx := r.Context()

	if _, err := s.store.Get(ctx, resourceKind, functionName); err != nil {
		if errors.Is(err, models.ErrNotFound) {
			writeError(w, http.StatusNotFound, "ResourceNotFoundException",
				fmt.Sprintf("Function not found: %s", functionARN(functionName)))
			return
		}
		writeError(w, http.StatusInternalServerError, "ServiceException", err.Error())
		return
	}

	// コンテナが起動中であれば停止・削除
	s.poolMu.Lock()
	entry, hasEntry := s.pool[functionName]
	if hasEntry {
		if entry.hostPort != 0 {
			s.ports.Release(entry.hostPort)
		}
		delete(s.pool, functionName)
	}
	s.poolMu.Unlock()

	if hasEntry && entry.containerID != "" {
		if err := s.docker.StopContainer(ctx, entry.containerID, nil); err != nil {
			s.logger.Sugar().Warnf("lambda: stop container %s: %v", entry.containerID, err)
		}
		if err := s.docker.RemoveContainer(ctx, entry.containerID); err != nil {
			s.logger.Sugar().Warnf("lambda: remove container %s: %v", entry.containerID, err)
		}
	}

	// State Store から削除
	if err := s.store.Delete(ctx, resourceKind, functionName); err != nil {
		writeError(w, http.StatusInternalServerError, "ServiceException", err.Error())
		return
	}

	// 一時ファイル削除
	if err := os.RemoveAll(functionDir(functionName)); err != nil {
		s.logger.Sugar().Warnf("lambda: remove function dir %s: %v", functionName, err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// handleListFunctions は ListFunctions API を処理します。
func (s *LambdaService) handleListFunctions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	resources, err := s.store.List(ctx, resourceKind, state.Filter{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ServiceException", err.Error())
		return
	}

	functions := make([]FunctionConfiguration, 0, len(resources))
	for _, res := range resources {
		functions = append(functions, specToFunctionConfiguration(res.Spec))
	}

	writeJSON(w, http.StatusOK, ListFunctionsResponse{Functions: functions})
}

// handleUpdateFunctionCode は UpdateFunctionCode API を処理します。
func (s *LambdaService) handleUpdateFunctionCode(w http.ResponseWriter, r *http.Request, functionName string) {
	ctx := r.Context()

	resource, err := s.store.Get(ctx, resourceKind, functionName)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			writeError(w, http.StatusNotFound, "ResourceNotFoundException",
				fmt.Sprintf("Function not found: %s", functionARN(functionName)))
			return
		}
		writeError(w, http.StatusInternalServerError, "ServiceException", err.Error())
		return
	}

	r.Body = io.NopCloser(io.LimitReader(r.Body, maxRequestBodyBytes))

	var req UpdateFunctionCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "InvalidParameterValueException", "invalid request body: "+err.Error())
		return
	}

	if req.ZipFile == "" {
		writeError(w, http.StatusBadRequest, "InvalidParameterValueException", "ZipFile is required")
		return
	}

	// 既存コードを削除してから再展開
	if err := os.RemoveAll(functionDir(functionName)); err != nil {
		s.logger.Sugar().Warnf("lambda: remove old code dir %s: %v", functionName, err)
	}

	codeSize, codeSha256, err := deployZipFile(functionName, req.ZipFile)
	if err != nil {
		writeError(w, http.StatusBadRequest, "InvalidParameterValueException", "failed to deploy code: "+err.Error())
		return
	}

	// コンテナが起動中の場合は停止 (次回 Invoke で再起動)
	s.poolMu.Lock()
	updateEntry, hasUpdateEntry := s.pool[functionName]
	if hasUpdateEntry {
		if updateEntry.hostPort != 0 {
			s.ports.Release(updateEntry.hostPort)
		}
		delete(s.pool, functionName)
	}
	s.poolMu.Unlock()

	if hasUpdateEntry && updateEntry.containerID != "" {
		if err := s.docker.StopContainer(ctx, updateEntry.containerID, nil); err != nil {
			s.logger.Sugar().Warnf("lambda: stop container for update %s: %v", updateEntry.containerID, err)
		}
		if err := s.docker.RemoveContainer(ctx, updateEntry.containerID); err != nil {
			s.logger.Sugar().Warnf("lambda: remove container for update %s: %v", updateEntry.containerID, err)
		}
	}

	now := time.Now().UTC().Format(time.RFC3339)
	resource.Spec["CodeSize"] = codeSize
	resource.Spec["CodeSha256"] = codeSha256
	resource.Spec["LastModified"] = now
	resource.UpdatedAt = time.Now()

	if err := s.store.Put(ctx, resource); err != nil {
		writeError(w, http.StatusInternalServerError, "ServiceException", err.Error())
		return
	}

	cfg := specToFunctionConfiguration(resource.Spec)
	writeJSON(w, http.StatusOK, cfg)
}

// deployZipFile は base64 エンコードされた zip ファイルをデコードし、
// /tmp/cloudia/lambda/functions/{functionName}/ に展開します。
// 展開後のコードサイズと SHA256 を返します。
func deployZipFile(functionName, zipFileBase64 string) (int64, string, error) {
	if zipFileBase64 == "" {
		return 0, "", fmt.Errorf("ZipFile is empty")
	}

	zipBytes, err := base64.StdEncoding.DecodeString(zipFileBase64)
	if err != nil {
		return 0, "", fmt.Errorf("base64 decode: %w", err)
	}

	// zip サイズ上限チェック
	if int64(len(zipBytes)) > maxZipFileBytes {
		return 0, "", fmt.Errorf("ZipFile exceeds maximum allowed size of %d bytes", maxZipFileBytes)
	}

	// SHA256 計算
	hash := sha256.Sum256(zipBytes)
	codeSha256 := fmt.Sprintf("%x", hash)

	destDir := functionDir(functionName)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return 0, "", fmt.Errorf("create dest dir: %w", err)
	}

	// zip 展開
	zipReader, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return 0, "", fmt.Errorf("open zip: %w", err)
	}

	var totalSize int64
	for _, f := range zipReader.File {
		fi := f.FileInfo()

		// シンボリックリンクをスキップ
		if fi.Mode()&os.ModeSymlink != 0 {
			continue
		}

		destPath := filepath.Join(destDir, filepath.Clean(f.Name))

		// ディレクトリトラバーサル防止
		if !isPathSafe(destDir, destPath) {
			return 0, "", fmt.Errorf("unsafe path in zip: %s", f.Name)
		}

		if fi.IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return 0, "", fmt.Errorf("create dir %s: %w", destPath, err)
			}
			continue
		}

		// 親ディレクトリを作成
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return 0, "", fmt.Errorf("create parent dir: %w", err)
		}

		rc, err := f.Open()
		if err != nil {
			return 0, "", fmt.Errorf("open file in zip %s: %w", f.Name, err)
		}

		outFile, err := os.Create(destPath)
		if err != nil {
			rc.Close()
			return 0, "", fmt.Errorf("create file %s: %w", destPath, err)
		}

		// 展開サイズ上限チェックしながらコピー
		n, err := io.Copy(outFile, io.LimitReader(rc, maxExtractedBytes-totalSize+1))
		outFile.Close()
		rc.Close()
		if err != nil {
			return 0, "", fmt.Errorf("write file %s: %w", destPath, err)
		}

		totalSize += n
		if totalSize > maxExtractedBytes {
			return 0, "", fmt.Errorf("extracted files exceed maximum allowed size of %d bytes", maxExtractedBytes)
		}
	}

	return totalSize, codeSha256, nil
}

// isPathSafe はパスが baseDir の外に出ないことを確認します。
func isPathSafe(baseDir, destPath string) bool {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return false
	}
	absDest, err := filepath.Abs(destPath)
	if err != nil {
		return false
	}
	// absDest が absBase で始まるか確認
	rel, err := filepath.Rel(absBase, absDest)
	if err != nil {
		return false
	}
	return !filepath.IsAbs(rel) && !startsWithDotDot(rel)
}

// startsWithDotDot はパスが ".." で始まるかを確認します。
func startsWithDotDot(path string) bool {
	if path == ".." {
		return true
	}
	if len(path) >= 3 && path[:3] == "../" {
		return true
	}
	return false
}

// specToFunctionConfiguration は Resource.Spec から FunctionConfiguration を生成します。
func specToFunctionConfiguration(spec map[string]interface{}) FunctionConfiguration {
	cfg := FunctionConfiguration{
		FunctionName: stringFromSpec(spec, "FunctionName"),
		FunctionArn:  stringFromSpec(spec, "FunctionArn"),
		Runtime:      stringFromSpec(spec, "Runtime"),
		Role:         stringFromSpec(spec, "Role"),
		Handler:      stringFromSpec(spec, "Handler"),
		Description:  stringFromSpec(spec, "Description"),
		CodeSha256:   stringFromSpec(spec, "CodeSha256"),
		LastModified: stringFromSpec(spec, "LastModified"),
		State:        stringFromSpec(spec, "State"),
	}

	if v, ok := spec["CodeSize"]; ok {
		switch val := v.(type) {
		case int64:
			cfg.CodeSize = val
		case float64:
			cfg.CodeSize = int64(val)
		case int:
			cfg.CodeSize = int64(val)
		}
	}

	if v, ok := spec["Timeout"]; ok {
		switch val := v.(type) {
		case int:
			cfg.Timeout = val
		case float64:
			cfg.Timeout = int(val)
		}
	}

	if v, ok := spec["MemorySize"]; ok {
		switch val := v.(type) {
		case int:
			cfg.MemorySize = val
		case float64:
			cfg.MemorySize = int(val)
		}
	}

	if envRaw, ok := spec["Environment"]; ok {
		if envMap, ok := envRaw.(map[string]interface{}); ok {
			if varsRaw, ok := envMap["Variables"]; ok {
				if varsMap, ok := varsRaw.(map[string]interface{}); ok {
					vars := make(map[string]string, len(varsMap))
					for k, v := range varsMap {
						if sv, ok := v.(string); ok {
							vars[k] = sv
						}
					}
					cfg.Environment = &EnvConfig{Variables: vars}
				}
			}
		}
	}

	return cfg
}

// stringFromSpec は spec から文字列値を取り出します。
func stringFromSpec(spec map[string]interface{}, key string) string {
	if v, ok := spec[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

