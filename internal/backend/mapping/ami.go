package mapping

import (
	"fmt"

	"github.com/HMasataka/cloudia/pkg/models"
)

// DefaultAMIMap は AMI ID から実際の AMI 識別子へのデフォルトマッピングです。
var DefaultAMIMap = map[string]string{
	"ami-ubuntu-22.04": "ami-0c02fb55956c7d316",
	"ami-amazon-linux2": "ami-0c55b159cbfafe1f0",
	"ami-debian-11":    "ami-09a41e26df96c4aef",
}

// ResolveAMI は AMI ID を実際の AMI 識別子に解決します。
// 未登録の AMI ID の場合は models.ErrNotFound をラップしたエラーを返します。
func ResolveAMI(amiID string) (string, error) {
	return resolveAMI(amiID, DefaultAMIMap)
}

func resolveAMI(amiID string, m map[string]string) (string, error) {
	v, ok := m[amiID]
	if !ok {
		return "", fmt.Errorf("AMI %q: %w", amiID, models.ErrNotFound)
	}
	return v, nil
}
