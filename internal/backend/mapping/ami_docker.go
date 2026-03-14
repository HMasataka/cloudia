package mapping

const defaultDockerImage = "ubuntu:22.04"

var defaultAMIDockerMap = map[string]string{
	"ami-0c02fb55956c7d316": "ubuntu:22.04",
	"ami-0c55b159cbfafe1f0": "amazonlinux:2",
	"ami-09a41e26df96c4aef": "debian:11",
}

func ResolveDockerImage(amiID string) string {
	return resolveDockerImage(amiID, defaultAMIDockerMap)
}

func resolveDockerImage(amiID string, m map[string]string) string {
	if v, ok := m[amiID]; ok {
		return v
	}
	return defaultDockerImage
}
