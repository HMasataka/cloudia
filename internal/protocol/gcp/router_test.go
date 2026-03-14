package gcp

import (
	"testing"
)

func TestResolveGCPService(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		path             string
		wantService      string
		wantResourcePath string
		wantErr          bool
	}{
		{
			name:             "storage path",
			path:             "/storage/v1/b/my-bucket/o/file.txt",
			wantService:      "storage",
			wantResourcePath: "b/my-bucket/o/file.txt",
		},
		{
			name:             "compute path",
			path:             "/compute/v1/projects/my-proj/zones/us-central1-a/instances",
			wantService:      "compute",
			wantResourcePath: "projects/my-proj/zones/us-central1-a/instances",
		},
		{
			name:             "cloudsql path via instances keyword",
			path:             "/v1/projects/my-proj/instances/my-instance",
			wantService:      "cloudsql",
			wantResourcePath: "my-proj/instances/my-instance",
		},
		{
			name:             "gke path via clusters keyword",
			path:             "/v1/projects/my-proj/locations/us-central1/clusters/my-cluster",
			wantService:      "gke",
			wantResourcePath: "my-proj/locations/us-central1/clusters/my-cluster",
		},
		{
			name:             "memorystore path via instances keyword with memorystore context",
			path:             "/v1/projects/my-proj/locations/us-central1/instances/my-cache",
			wantService:      "cloudsql",
			wantResourcePath: "my-proj/locations/us-central1/instances/my-cache",
		},
		{
			name:    "v1/projects unknown service",
			path:    "/v1/projects/my-proj/something",
			wantErr: true,
		},
		{
			name:    "unknown path",
			path:    "/unknown/path",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc, resourcePath, err := ResolveGCPService(tt.path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ResolveGCPService(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if svc != tt.wantService {
				t.Errorf("service = %q, want %q", svc, tt.wantService)
			}
			if resourcePath != tt.wantResourcePath {
				t.Errorf("resourcePath = %q, want %q", resourcePath, tt.wantResourcePath)
			}
		})
	}
}
