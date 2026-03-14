package docker

const (
	LabelManaged    = "cloudia.managed"
	LabelService    = "cloudia.service"
	LabelProvider   = "cloudia.provider"
	LabelResourceID = "cloudia.resource-id"
	LabelKind       = "cloudia.kind"
	LabelRegion     = "cloudia.region"
)

// ManagedLabels returns a map of labels marking the resource as managed by cloudia.
func ManagedLabels(service, provider, kind, region string) map[string]string {
	return map[string]string{
		LabelManaged:    "true",
		LabelService:    service,
		LabelProvider:   provider,
		LabelResourceID: "",
		LabelKind:       kind,
		LabelRegion:     region,
	}
}
