package eks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/backend/k3s"
	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

// defaultBackendFactory は本番用の ClusterBackend ファクトリです。
func defaultBackendFactory(logger *zap.Logger) ClusterBackend {
	return k3s.NewK3sBackend(logger)
}

// createCluster は EKS クラスタを作成します。
// ClusterBackend を起動し、Store に Cluster リソースを保存します。
func (s *EKSService) createCluster(ctx context.Context, req service.Request) (service.Response, error) {
	var createReq CreateClusterRequest
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &createReq); err != nil {
			return jsonErrorResponse(http.StatusBadRequest, "InvalidParameterException",
				"invalid request body: "+err.Error())
		}
	}

	if createReq.Name == "" {
		return jsonErrorResponse(http.StatusBadRequest, "InvalidParameterException",
			"name is required")
	}

	// 重複チェック
	existing, err := s.store.Get(ctx, kindCluster, createReq.Name)
	if err != nil && !errors.Is(err, models.ErrNotFound) {
		return jsonErrorResponse(http.StatusInternalServerError, "ServiceException", err.Error())
	}
	if existing != nil {
		return jsonErrorResponse(http.StatusConflict, "ResourceInUseException",
			fmt.Sprintf("Cluster already exists: %s", createReq.Name))
	}

	// ClusterBackend を起動
	backend := s.backendFactory(s.logger)
	if err := backend.Start(ctx, s.deps, createReq.Name); err != nil {
		s.logger.Error("eks: start k3s backend failed",
			zap.String("cluster", createReq.Name),
			zap.Error(err),
		)
		return jsonErrorResponse(http.StatusInternalServerError, "ServiceException",
			"failed to start k3s cluster: "+err.Error())
	}

	version := createReq.Version
	if version == "" {
		version = "1.29"
	}

	arn := clusterARN(createReq.Name)
	endpoint := backend.Endpoint()
	kubeconfig := backend.Kubeconfig()
	caData := backend.CertificateAuthority()

	spec := map[string]interface{}{
		"name":       createReq.Name,
		"arn":        arn,
		"status":     "ACTIVE",
		"endpoint":   endpoint,
		"kubeconfig": kubeconfig,
		"caData":     caData,
		"version":    version,
		"roleArn":    createReq.RoleArn,
	}

	resource := &models.Resource{
		Kind:        kindCluster,
		ID:          createReq.Name,
		Provider:    "aws",
		Service:     "eks",
		Region:      awsRegion,
		ContainerID: backend.ContainerID(),
		Spec:        spec,
		Status:      "active",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := s.store.Put(ctx, resource); err != nil {
		return jsonErrorResponse(http.StatusInternalServerError, "ServiceException", err.Error())
	}

	cluster := specToCluster(spec)
	return jsonResponse(http.StatusOK, ClusterResponse{Cluster: cluster})
}

// describeCluster は EKS クラスタ情報を取得します。
func (s *EKSService) describeCluster(ctx context.Context, req service.Request) (service.Response, error) {
	name := clusterNameFromAction(req.Action)

	resource, err := s.store.Get(ctx, kindCluster, name)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return jsonErrorResponse(http.StatusNotFound, "ResourceNotFoundException",
				fmt.Sprintf("No cluster found for name: %s", name))
		}
		return jsonErrorResponse(http.StatusInternalServerError, "ServiceException", err.Error())
	}

	cluster := specToCluster(resource.Spec)
	return jsonResponse(http.StatusOK, ClusterResponse{Cluster: cluster})
}

// deleteCluster は EKS クラスタを削除します。
// コンテナを停止・削除し、Store からリソースを削除します。
func (s *EKSService) deleteCluster(ctx context.Context, req service.Request) (service.Response, error) {
	name := clusterNameFromAction(req.Action)

	resource, err := s.store.Get(ctx, kindCluster, name)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return jsonErrorResponse(http.StatusNotFound, "ResourceNotFoundException",
				fmt.Sprintf("No cluster found for name: %s", name))
		}
		return jsonErrorResponse(http.StatusInternalServerError, "ServiceException", err.Error())
	}

	// コンテナが存在する場合は停止・削除
	if resource.ContainerID != "" {
		if err := s.deps.DockerClient.StopContainer(ctx, resource.ContainerID, nil); err != nil {
			s.logger.Warn("eks: stop container failed",
				zap.String("cluster", name),
				zap.String("containerID", resource.ContainerID),
				zap.Error(err),
			)
		}
		if err := s.deps.DockerClient.RemoveContainer(ctx, resource.ContainerID); err != nil {
			s.logger.Warn("eks: remove container failed",
				zap.String("cluster", name),
				zap.String("containerID", resource.ContainerID),
				zap.Error(err),
			)
		}
	}

	if err := s.store.Delete(ctx, kindCluster, name); err != nil {
		return jsonErrorResponse(http.StatusInternalServerError, "ServiceException", err.Error())
	}

	cluster := specToCluster(resource.Spec)
	return jsonResponse(http.StatusOK, ClusterResponse{Cluster: cluster})
}

// listClusters は EKS クラスタ一覧を取得します。
func (s *EKSService) listClusters(ctx context.Context, _ service.Request) (service.Response, error) {
	resources, err := s.store.List(ctx, kindCluster, state.Filter{})
	if err != nil {
		return jsonErrorResponse(http.StatusInternalServerError, "ServiceException", err.Error())
	}

	names := make([]string, 0, len(resources))
	for _, r := range resources {
		names = append(names, r.ID)
	}

	return jsonResponse(http.StatusOK, ListClustersResponse{Clusters: names})
}

// specToCluster は Resource.Spec から Cluster を生成します。
func specToCluster(spec map[string]interface{}) Cluster {
	return Cluster{
		Name:              stringFromSpec(spec, "name"),
		Arn:               stringFromSpec(spec, "arn"),
		Status:            stringFromSpec(spec, "status"),
		Endpoint:          stringFromSpec(spec, "endpoint"),
		KubernetesVersion: stringFromSpec(spec, "version"),
		RoleArn:           stringFromSpec(spec, "roleArn"),
		CertificateAuthority: CertificateAuthority{
			Data: stringFromSpec(spec, "caData"),
		},
	}
}

// stringFromSpec は spec から文字列値を取り出します。
func stringFromSpec(spec map[string]interface{}, key string) string {
	if v, ok := spec[key]; ok {
		if sv, ok := v.(string); ok {
			return sv
		}
	}
	return ""
}
