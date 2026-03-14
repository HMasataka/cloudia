package aws

import "fmt"

// FormatARN は AWS ARN 文字列を生成します。
// 形式: arn:{partition}:{service}:{region}:{accountID}:{resource}
func FormatARN(partition, service, region, accountID, resource string) string {
	return fmt.Sprintf("arn:%s:%s:%s:%s:%s", partition, service, region, accountID, resource)
}
