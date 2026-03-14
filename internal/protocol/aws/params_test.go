package aws

import (
	"reflect"
	"testing"
)

func TestParseFilters(t *testing.T) {
	t.Run("正常系: 単一フィルタと単一値を解析する", func(t *testing.T) {
		params := map[string]string{
			"Filter.1.Name":    "vpc-id",
			"Filter.1.Value.1": "vpc-123",
		}
		got := ParseFilters(params)
		want := []Filter{
			{Name: "vpc-id", Values: []string{"vpc-123"}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ParseFilters() = %v, want %v", got, want)
		}
	})

	t.Run("正常系: 単一フィルタと複数値を解析する", func(t *testing.T) {
		params := map[string]string{
			"Filter.1.Name":    "instance-state-name",
			"Filter.1.Value.1": "running",
			"Filter.1.Value.2": "stopped",
		}
		got := ParseFilters(params)
		want := []Filter{
			{Name: "instance-state-name", Values: []string{"running", "stopped"}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ParseFilters() = %v, want %v", got, want)
		}
	})

	t.Run("正常系: 複数フィルタを解析する", func(t *testing.T) {
		params := map[string]string{
			"Filter.1.Name":    "vpc-id",
			"Filter.1.Value.1": "vpc-123",
			"Filter.2.Name":    "state",
			"Filter.2.Value.1": "available",
			"Filter.2.Value.2": "pending",
		}
		got := ParseFilters(params)
		want := []Filter{
			{Name: "vpc-id", Values: []string{"vpc-123"}},
			{Name: "state", Values: []string{"available", "pending"}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ParseFilters() = %v, want %v", got, want)
		}
	})

	t.Run("正常系: フィルタが存在しない場合は空スライスを返す", func(t *testing.T) {
		params := map[string]string{
			"Action":  "DescribeInstances",
			"Version": "2016-11-15",
		}
		got := ParseFilters(params)
		if len(got) != 0 {
			t.Errorf("ParseFilters() = %v, want empty slice", got)
		}
	})

	t.Run("正常系: フィルタに値がない場合も Name は保持される", func(t *testing.T) {
		params := map[string]string{
			"Filter.1.Name": "tag-key",
		}
		got := ParseFilters(params)
		want := []Filter{
			{Name: "tag-key", Values: nil},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ParseFilters() = %v, want %v", got, want)
		}
	})
}
