// Count available CPUs based on affinity

//go:build linux

package pvmi

import (
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

// For linux count available CPUs based on CPU affinity, w/ a fallback on runtime:
func CountAvailableCPUs() int {
	cpuSet := unix.CPUSet{}
	err := unix.SchedGetaffinity(os.Getpid(), &cpuSet)
	if err != nil {
		Log.Warn(err)
		return runtime.NumCPU()
	}
	count := 0
	for _, cpuMask := range cpuSet {
		for cpuMask != 0 {
			count++
			cpuMask &= (cpuMask - 1)
		}
	}
	return count
}
