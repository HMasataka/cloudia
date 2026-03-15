package admin

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strconv"

	"github.com/HMasataka/cloudia/internal/state"
	"github.com/HMasataka/cloudia/pkg/models"
)

//go:embed static templates
var embeddedFS embed.FS

// templateFuncs はテンプレートで使用するカスタム関数マップです。
var templateFuncs = template.FuncMap{
	"prev": func(p int) int {
		if p > 1 {
			return p - 1
		}
		return 1
	},
	"next": func(p int) int { return p + 1 },
	"end_resource_index": func(page, perPage int) int {
		return page * perPage
	},
}

// StaticFS は静的アセット配信用の http.FileSystem を返します。
func (h *Handler) StaticFS() http.FileSystem {
	sub, err := fs.Sub(embeddedFS, "static")
	if err != nil {
		return http.FS(embeddedFS)
	}
	return http.FS(sub)
}

// renderTemplate は templates/ からテンプレートをロードして bytes.Buffer に書き込みます。
// HX-Request ヘッダがある場合は content ブロックのみをレンダリングします。
func (h *Handler) renderTemplate(w http.ResponseWriter, r *http.Request, name string, data any) {
	tmplFiles := []string{
		"templates/layout.html",
		"templates/" + name,
	}

	isHXRequest := r.Header.Get("HX-Request") == "true"
	if isHXRequest {
		tmplFiles = []string{"templates/" + name}
	}

	tmpl, err := template.New("").Funcs(templateFuncs).ParseFS(embeddedFS, tmplFiles...)
	if err != nil {
		http.Error(w, "template parse error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	entryTemplate := "layout.html"
	if isHXRequest {
		entryTemplate = name
	}

	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, entryTemplate, data); err != nil {
		http.Error(w, "template render error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(buf.Bytes()) //nolint:errcheck
}

// basePage はすべてのページデータに共通するフィールドです（ナビゲーションのアクティブ状態用）。
type basePage struct {
	ActivePage string
}

// dashboardPage はダッシュボードのテンプレートデータです。
type dashboardPage struct {
	basePage
	TotalResources  int
	TotalServices   int
	TotalContainers int
	HealthyCount    int
	Services        []serviceInfo
}

// DashboardPage は GET /admin/ui のハンドラです。
func (h *Handler) DashboardPage(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	var services []serviceInfo
	var healthyCount int
	if h.registry != nil {
		metas := h.registry.ListServices()
		statuses := h.registry.HealthAll(ctx)
		services = make([]serviceInfo, 0, len(metas))
		for key, meta := range metas {
			status := statuses[key]
			if status.Healthy {
				healthyCount++
			}
			services = append(services, serviceInfo{
				Key:      key,
				Provider: meta.Provider,
				Name:     meta.Name,
				Health:   status,
			})
		}
	}

	var totalResources int
	if h.store != nil {
		resources, err := h.store.List(ctx, "", state.Filter{})
		if err == nil {
			totalResources = len(resources)
		}
	}

	var totalContainers int
	if h.dockerClient != nil {
		containers, err := h.dockerClient.ListManagedContainers(ctx)
		if err == nil {
			totalContainers = len(containers)
		}
	}

	h.renderTemplate(w, r, "dashboard.html", dashboardPage{
		basePage:        basePage{ActivePage: "dashboard"},
		TotalResources:  totalResources,
		TotalServices:   len(services),
		TotalContainers: totalContainers,
		HealthyCount:    healthyCount,
		Services:        services,
	})
}

// resourceFilter はリソースフィルタ条件を保持します。
type resourceFilter struct {
	Provider string
	Service  string
	Kind     string
}

// resourcesPage はリソースブラウザのテンプレートデータです。
type resourcesPage struct {
	basePage
	Resources   []*models.Resource
	Page        int
	PerPage     int
	Total       int
	Filter      resourceFilter
	QueryString string
}

// ResourcesPage は GET /admin/ui/resources のハンドラです。
func (h *Handler) ResourcesPage(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	q := r.URL.Query()
	provider := q.Get("provider")
	svc := q.Get("service")
	kind := q.Get("kind")

	page := 1
	if p := q.Get("page"); p != "" {
		if v, err := strconv.Atoi(p); err == nil && v > 0 {
			page = v
		}
	}
	perPage := defaultPerPage
	if pp := q.Get("per_page"); pp != "" {
		if v, err := strconv.Atoi(pp); err == nil && v > 0 {
			perPage = v
		}
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}

	filter := state.Filter{}
	if provider != "" {
		filter["Provider"] = provider
	}
	if svc != "" {
		filter["Service"] = svc
	}

	var resources []*models.Resource
	var total int
	if h.store != nil {
		res, err := h.store.List(ctx, kind, filter)
		if err == nil {
			total = len(res)
			start := (page - 1) * perPage
			if start < total {
				end := start + perPage
				if end > total {
					end = total
				}
				resources = res[start:end]
			} else {
				resources = []*models.Resource{}
			}
		}
	}

	// QueryString はページネーションリンク構築用（page パラメータなし）
	qs := url.Values{}
	if provider != "" {
		qs.Set("provider", provider)
	}
	if svc != "" {
		qs.Set("service", svc)
	}
	if kind != "" {
		qs.Set("kind", kind)
	}
	if perPage != defaultPerPage {
		qs.Set("per_page", fmt.Sprintf("%d", perPage))
	}

	h.renderTemplate(w, r, "resources.html", resourcesPage{
		basePage:    basePage{ActivePage: "resources"},
		Resources:   resources,
		Page:        page,
		PerPage:     perPage,
		Total:       total,
		Filter:      resourceFilter{Provider: provider, Service: svc, Kind: kind},
		QueryString: qs.Encode(),
	})
}

// resourceDetailPage はリソース詳細のテンプレートデータです。
type resourceDetailPage struct {
	basePage
	Resource *models.Resource
}

// ResourceDetailPage は GET /admin/ui/resources/{kind}/{id} のハンドラです。
func (h *Handler) ResourceDetailPage(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	kind := r.PathValue("kind")
	id := r.PathValue("id")

	if h.store == nil {
		http.Error(w, "store not available", http.StatusServiceUnavailable)
		return
	}

	resource, err := h.store.Get(ctx, kind, id)
	if err != nil {
		http.Error(w, "resource not found", http.StatusNotFound)
		return
	}

	h.renderTemplate(w, r, "resource_detail.html", resourceDetailPage{
		basePage: basePage{ActivePage: "resources"},
		Resource: resource,
	})
}

// containersPage はコンテナビューのテンプレートデータです。
type containersPage struct {
	basePage
	Containers interface{}
	Error      string
}

// ContainersPage は GET /admin/ui/containers のハンドラです。
func (h *Handler) ContainersPage(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	data := containersPage{basePage: basePage{ActivePage: "containers"}}

	if h.dockerClient != nil {
		containers, err := h.dockerClient.ListManagedContainers(ctx)
		if err != nil {
			data.Error = err.Error()
		} else {
			data.Containers = containers
		}
	}

	h.renderTemplate(w, r, "containers.html", data)
}

// configPage は設定ビューのテンプレートデータです。
type configPage struct {
	basePage
	Config maskedConfig
}

// ConfigPage は GET /admin/ui/config のハンドラです。
func (h *Handler) ConfigPage(w http.ResponseWriter, r *http.Request) {
	if h.config == nil {
		http.Error(w, "config not available", http.StatusServiceUnavailable)
		return
	}

	mc := maskedConfig{
		Server:  h.config.Server,
		Logging: h.config.Logging,
		Docker:  h.config.Docker,
		Limits:  h.config.Limits,
		State:   h.config.State,
		Cleanup: h.config.Cleanup,
		Metrics: h.config.Metrics,
		Ports:   h.config.Ports,
		Auth: maskedAuthConfig{
			Mode: h.config.Auth.Mode,
			AWS: maskedAWSConfig{
				AccessKey: masked,
				SecretKey: masked,
				AccountID: h.config.Auth.AWS.AccountID,
				Region:    h.config.Auth.AWS.Region,
			},
			GCP: maskedGCPConfig{
				CredentialsFile: masked,
				Project:         h.config.Auth.GCP.Project,
				Zone:            h.config.Auth.GCP.Zone,
			},
		},
		Endpoints: h.config.Endpoints,
	}

	h.renderTemplate(w, r, "config.html", configPage{
		basePage: basePage{ActivePage: "config"},
		Config:   mc,
	})
}
