package gke

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/HMasataka/cloudia/internal/service"
	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

var clusterNameRegexp = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9-]{0,99}$`)

// clusterStoreID は project+location+name をキーとする store ID を生成します。
func clusterStoreID(project, location, name string) string {
	return fmt.Sprintf("%s/%s/%s", project, location, name)
}

// createCluster は clusters.create を処理します。
func (g *GKEService) createCluster(ctx context.Context, req service.Request, project, location string) (service.Response, error) {
	var body CreateClusterRequest
	if err := json.Unmarshal(req.Body, &body); err != nil {
		return gkeErrorResponse(http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err), "INVALID_ARGUMENT")
	}

	if body.Cluster.Name == "" {
		return gkeErrorResponse(http.StatusBadRequest, "cluster name is required", "INVALID_ARGUMENT")
	}

	if !clusterNameRegexp.MatchString(body.Cluster.Name) {
		return gkeErrorResponse(http.StatusBadRequest,
			"cluster name must match ^[a-zA-Z][a-zA-Z0-9-]{0,99}$",
			"INVALID_ARGUMENT")
	}

	storeID := clusterStoreID(project, location, body.Cluster.Name)

	// 重複チェック
	_, err := g.store.Get(ctx, kindCluster, storeID)
	if err == nil {
		return gkeErrorResponse(http.StatusConflict,
			fmt.Sprintf("cluster %q already exists", body.Cluster.Name),
			"ALREADY_EXISTS")
	}
	if !errors.Is(err, models.ErrNotFound) {
		return gkeErrorResponse(http.StatusInternalServerError, err.Error(), "INTERNAL")
	}

	// k3s バックエンド起動
	backend := g.backendFactory(g.logger)
	if startErr := backend.Start(ctx, g.deps, body.Cluster.Name); startErr != nil {
		g.logger.Warn("gke createCluster: K3sBackend.Start failed",
			zap.String("name", body.Cluster.Name),
			zap.Error(startErr),
		)
		return gkeErrorResponse(http.StatusInternalServerError, "failed to start cluster backend", "INTERNAL")
	}

	initialNodeCount := body.Cluster.InitialNodeCount
	masterVersion := body.Cluster.MasterVersion
	if masterVersion == "" {
		masterVersion = "1.29"
	}

	now := time.Now().UTC()
	resource := &models.Resource{
		Kind:        kindCluster,
		ID:          storeID,
		Provider:    "gcp",
		Service:     "gke",
		Region:      location,
		Status:      statusRunning,
		ContainerID: backend.ContainerID(),
		CreatedAt:   now,
		UpdatedAt:   now,
		Spec: map[string]interface{}{
			"name":             body.Cluster.Name,
			"project":          project,
			"location":         location,
			"endpoint":         backend.Endpoint(),
			"kubeconfig":       backend.Kubeconfig(),
			"caData":           backend.CertificateAuthority(),
			"initialNodeCount": initialNodeCount,
			"masterVersion":    masterVersion,
			"createdAt":        now.Format(time.RFC3339),
		},
	}

	if putErr := g.store.Put(ctx, resource); putErr != nil {
		g.logger.Warn("gke createCluster: store.Put failed",
			zap.String("name", body.Cluster.Name),
			zap.Error(putErr),
		)
		_ = backend.Shutdown(ctx)
		return service.Response{StatusCode: http.StatusInternalServerError}, putErr
	}

	g.mu.Lock()
	g.backends[storeID] = backend
	g.mu.Unlock()

	op := Operation{
		Name:       "operation-create-" + body.Cluster.Name,
		Status:     "DONE",
		TargetLink: fmt.Sprintf("projects/%s/locations/%s/clusters/%s", project, location, body.Cluster.Name),
	}
	return jsonResponse(http.StatusOK, op)
}

// getCluster は clusters.get を処理します。
func (g *GKEService) getCluster(ctx context.Context, project, location, name string) (service.Response, error) {
	storeID := clusterStoreID(project, location, name)
	r, err := g.store.Get(ctx, kindCluster, storeID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return gkeErrorResponse(http.StatusNotFound,
				fmt.Sprintf("cluster %q not found", name),
				"NOT_FOUND")
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	item := clusterItemFromResource(r)
	return jsonResponse(http.StatusOK, item)
}

// listClusters は clusters.list を処理します。
func (g *GKEService) listClusters(ctx context.Context, project, location string) (service.Response, error) {
	all, err := g.store.List(ctx, kindCluster, state.Filter{})
	if err != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	prefix := project + "/" + location + "/"
	var items []ClusterItem
	for _, r := range all {
		if strings.HasPrefix(r.ID, prefix) {
			items = append(items, clusterItemFromResource(r))
		}
	}

	if items == nil {
		items = []ClusterItem{}
	}

	resp := ListClustersResponse{
		Clusters: items,
	}
	return jsonResponse(http.StatusOK, resp)
}

// deleteCluster は clusters.delete を処理します。
func (g *GKEService) deleteCluster(ctx context.Context, project, location, name string) (service.Response, error) {
	storeID := clusterStoreID(project, location, name)
	r, err := g.store.Get(ctx, kindCluster, storeID)
	if err != nil {
		if errors.Is(err, models.ErrNotFound) {
			return gkeErrorResponse(http.StatusNotFound,
				fmt.Sprintf("cluster %q not found", name),
				"NOT_FOUND")
		}
		return service.Response{StatusCode: http.StatusInternalServerError}, err
	}

	g.mu.Lock()
	backend, hasBackend := g.backends[storeID]
	if hasBackend {
		delete(g.backends, storeID)
	}
	g.mu.Unlock()

	if hasBackend {
		if err := backend.Shutdown(ctx); err != nil {
			g.logger.Warn("gke deleteCluster: shutdown backend failed",
				zap.String("name", name),
				zap.Error(err),
			)
		}
	} else if r.ContainerID != "" {
		// バックエンド参照がない場合は直接コンテナを停止
		containerID := r.ContainerID
		if stopErr := g.deps.DockerClient.StopContainer(ctx, containerID, nil); stopErr != nil {
			g.logger.Warn("gke deleteCluster: stop container failed",
				zap.String("name", name),
				zap.String("container_id", containerID),
				zap.Error(stopErr),
			)
		}
		if rmErr := g.deps.DockerClient.RemoveContainer(ctx, containerID); rmErr != nil {
			g.logger.Warn("gke deleteCluster: remove container failed",
				zap.String("name", name),
				zap.String("container_id", containerID),
				zap.Error(rmErr),
			)
		}
	}

	if delErr := g.store.Delete(ctx, kindCluster, storeID); delErr != nil {
		return service.Response{StatusCode: http.StatusInternalServerError}, delErr
	}

	op := Operation{
		Name:       "operation-delete-" + name,
		Status:     "DONE",
		TargetLink: fmt.Sprintf("projects/%s/locations/%s/clusters/%s", project, location, name),
	}
	return jsonResponse(http.StatusOK, op)
}

// clusterItemFromResource は models.Resource から ClusterItem を構築します。
func clusterItemFromResource(r *models.Resource) ClusterItem {
	name, _ := r.Spec["name"].(string)
	endpoint, _ := r.Spec["endpoint"].(string)
	caData, _ := r.Spec["caData"].(string)
	masterVersion, _ := r.Spec["masterVersion"].(string)
	initialNodeCountF, _ := r.Spec["initialNodeCount"].(float64)
	initialNodeCount := int(initialNodeCountF)

	return ClusterItem{
		Name:             name,
		Status:           r.Status,
		Endpoint:         endpoint,
		MasterAuth:       MasterAuth{ClusterCaCertificate: caData},
		InitialNodeCount: initialNodeCount,
		MasterVersion:    masterVersion,
	}
}
