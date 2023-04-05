// Count available CPUs based on affinity

//go:build !linux

package pvmi

import (
	"runtime"
)

func CountAvailableCPUs() int {
	return runtime.NumCPU()
}
