package docker

const (
	LabelManaged    = "cloudia.managed"
	LabelService    = "cloudia.service"
	LabelProvider   = "cloudia.provider"
	LabelResourceID = "cloudia.resource-id"
)

// ManagedLabels returns a map of labels marking the resource as managed by cloudia.
func ManagedLabels(service, provider string) map[string]string {
	return map[string]string{
		LabelManaged:  "true",
		LabelService:  service,
		LabelProvider: provider,
	}
}
