package aws

import (
	"fmt"
)

// Filter は AWS Query プロトコルのフィルタを表します。
type Filter struct {
	Name   string
	Values []string
}

// ParseFilters は AWS Query プロトコルの params から Filter スライスを解析します。
// キーのパターンは `Filter.N.Name` (フィルタ名) および `Filter.N.Value.M` (フィルタ値) です。
// N および M は 1-indexed の連番です。
func ParseFilters(params map[string]string) []Filter {
	filters := []Filter{}

	for n := 1; ; n++ {
		nameKey := fmt.Sprintf("Filter.%d.Name", n)
		name, ok := params[nameKey]
		if !ok {
			break
		}

		var values []string
		for m := 1; ; m++ {
			valueKey := fmt.Sprintf("Filter.%d.Value.%d", n, m)
			v, ok := params[valueKey]
			if !ok {
				break
			}
			values = append(values, v)
		}

		filters = append(filters, Filter{
			Name:   name,
			Values: values,
		})
	}

	return filters
}
