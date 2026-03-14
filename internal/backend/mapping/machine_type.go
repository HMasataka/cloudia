package mapping

import (
	"fmt"

	"github.com/HMasataka/cloudia/pkg/models"
)

// MachineSpec はマシンタイプのスペックを表します。
// CPU は NanoCPUs (1 CPU = 1_000_000_000 NanoCPUs)、
// Memory はバイト単位のメモリサイズです。
type MachineSpec struct {
	CPU    int64 // NanoCPUs
	Memory int64 // bytes
}

// DefaultMachineMap はマシンタイプ名から MachineSpec へのデフォルトマッピングです。
var DefaultMachineMap = map[string]MachineSpec{
	"t2.micro": {
		CPU:    1_000_000_000, // 1 vCPU
		Memory: 1_073_741_824, // 1 GiB
	},
	"t2.small": {
		CPU:    1_000_000_000, // 1 vCPU
		Memory: 2_147_483_648, // 2 GiB
	},
	"t2.medium": {
		CPU:    2_000_000_000, // 2 vCPU
		Memory: 4_294_967_296, // 4 GiB
	},
	"n1-standard-1": {
		CPU:    1_000_000_000, // 1 vCPU
		Memory: 3_840_000_000, // 3.75 GiB
	},
}

// ResolveMachineType はマシンタイプ名を MachineSpec に解決します。
// 未登録のマシンタイプの場合は models.ErrNotFound をラップしたエラーを返します。
func ResolveMachineType(machineType string) (MachineSpec, error) {
	return resolveMachineType(machineType, DefaultMachineMap)
}

func resolveMachineType(machineType string, m map[string]MachineSpec) (MachineSpec, error) {
	v, ok := m[machineType]
	if !ok {
		return MachineSpec{}, fmt.Errorf("machine type %q: %w", machineType, models.ErrNotFound)
	}
	return v, nil
}
