// Return the list of PID, TID to scan

package pvmi

import (
	"os"
	"path"
	"strconv"
	"sync"
	"time"
)

const (
	PID_LIST_CACHE_PID_ENABLED_FLAG = uint32(1 << 0)
	PID_LIST_CACHE_TID_ENABLED_FLAG = uint32(1 << 1)
	PID_LIST_CACHE_ALL_ENABLED_FLAG = PID_LIST_CACHE_PID_ENABLED_FLAG | PID_LIST_CACHE_TID_ENABLED_FLAG
)

type GetPidsFn func(int) []PidTidPair

var GlobalPidListCache *PidListCache

type PidTidPair struct {
	pid, tid int
}

func (pidTid PidTidPair) GetPidTid() (int, int) {
	return pidTid.pid, pidTid.tid
}

// The task of scanning processes (PIDs) and threads (TIDs) may be divided upon
// multiple goroutines. Rather than having each goroutine scan /proc,
// maintaining a cache with the most recent scan with an expiration date. If
// multiple goroutines ask for the relevant lists within the cache window then
// the results may be re-used.
type PidListCache struct {
	// The number of partitions, N, that divide the workload (the number of
	// worker goroutines, that is). Each goroutine identifies with a number i =
	// 0..(N-1) and handles only PID/TID such that [PT]ID % N == i.
	nPart int

	// If nPart is a power of 2, then use a mask instead of % (modulo). The mask
	// will be set to a negative number if disabled:
	mask int

	// The timestamp of the latest retrieval:
	retrievedTime time.Time

	// How long a scan is valid:
	validFor time.Duration

	// What is being cached:
	flags uint32

	// The actual lists:
	pidLists [][]PidTidPair

	// The root of the file system; typically /proc, but allow this to be set
	// for testing purposes:
	root string

	// Lock protection:
	lock sync.Mutex
}

func NewPidListCache(nPart int, validFor time.Duration, root string, flags uint32) *PidListCache {
	mask := int(-1)
	for m := 1; m < (1 << 30); m = m << 1 {
		if m == nPart {
			mask = m - 1
			break
		}
	}

	return &PidListCache{
		nPart:         nPart,
		mask:          mask,
		retrievedTime: time.Now().Add(-2 * validFor), // this should force a refresh at the next call
		validFor:      validFor,
		flags:         flags,
		root:          root,
	}
}

func getDirNames(dir string) ([]string, error) {
	d, err := os.Open(dir)
	if err != nil {
		Log.Warnf("open(%#v): %s", dir, err)
		return nil, err
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		Log.Warnf("readdir(%#v): %s", dir, err)
	}
	return names, err
}

func (c *PidListCache) RetrievedTime() time.Time {
	return c.retrievedTime
}

func (c *PidListCache) IsEnabledFor(flags uint32) bool {
	return c.flags&flags > 0
}

func (c *PidListCache) Refresh(lockAcquired bool) {
	if !lockAcquired {
		c.lock.Lock()
		defer c.lock.Unlock()
	}

	c.pidLists = make([][]PidTidPair, c.nPart)
	for i := 0; i < c.nPart; i++ {
		c.pidLists[i] = make([]PidTidPair, 0)
	}

	names, err := getDirNames(c.root)
	if err != nil {
		return
	}

	mask, nPart, useMask := c.mask, c.nPart, c.mask >= 0
	isPidEnabled := c.IsEnabledFor(PID_LIST_CACHE_PID_ENABLED_FLAG)
	isTidEnabled := c.IsEnabledFor(PID_LIST_CACHE_TID_ENABLED_FLAG)
	numEntries := 0
	for _, name := range names {
		var part int

		pid64, err := strconv.ParseInt(name, 10, 64)
		if err != nil {
			continue
		}
		pid := int(pid64)

		if isPidEnabled {
			if useMask {
				part = pid & mask
			} else {
				part = pid % nPart
			}
			c.pidLists[part] = append(c.pidLists[part], PidTidPair{pid, 0})
			numEntries += 1
		}
		if isTidEnabled {
			// TID's belonging to PID:
			names, err := getDirNames(path.Join(c.root, name, "task"))
			if err != nil {
				continue
			}
			for _, name := range names {
				tid64, err := strconv.ParseInt(name, 10, 64)
				if err != nil {
					continue
				}
				tid := int(tid64)
				if useMask {
					part = tid & c.mask
				} else {
					part = tid % c.nPart
				}
				c.pidLists[part] = append(c.pidLists[part], PidTidPair{pid, tid})
				numEntries += 1
			}
		}
	}
	c.retrievedTime = time.Now()
}

func (c *PidListCache) GetPids(part int) []PidTidPair {
	if part < 0 || part >= c.nPart {
		return nil
	}
	c.lock.Lock()
	defer c.lock.Unlock()
	if time.Since(c.retrievedTime) > c.validFor {
		c.Refresh(true)
	}
	return c.pidLists[part]
}
