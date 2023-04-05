// Custom bytes.Buffer Pool
//
// Unlike sync.Pool this pool can be capped to a max size upon buffer
// return, making the high memory usage transient.

package pvmi

import (
	"bytes"
	"sync"
)

const (
	DEFAULT_BUF_POOL_MAX_SIZE = 128
	BUF_POOL_MAX_SIZE_UNBOUND = 0
	// Metrics will be added to a buffer until >= max size, at which point the
	// buffer will be sent to the metrics system.
	BUF_MAX_SIZE = 0x10000 // 64k
)

type BufferPoolEntry struct {
	b    *bytes.Buffer
	next *BufferPoolEntry
}

type BufferPool struct {
	lock            *sync.Mutex
	head            *BufferPoolEntry
	size            int
	maxSize         int
	checkedOutCount int
}

func (p *BufferPool) GetBuffer() *bytes.Buffer {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.checkedOutCount++
	if p.head == nil {
		return new(bytes.Buffer)
	}
	poolEntry := p.head
	p.head = poolEntry.next
	p.size--
	b := poolEntry.b
	b.Reset()
	return b
}

func (p *BufferPool) ReturnBuffer(b *bytes.Buffer) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.checkedOutCount--
	if p.maxSize != BUF_POOL_MAX_SIZE_UNBOUND && p.size >= p.maxSize {
		return
	}
	p.head = &BufferPoolEntry{
		b:    b,
		next: p.head,
	}
	p.size++
}

func (p *BufferPool) SetMaxSize(maxSize int) {
	p.lock.Lock()
	defer p.lock.Unlock()
	p.maxSize = maxSize
	if p.maxSize == BUF_POOL_MAX_SIZE_UNBOUND {
		return
	}
	for p.size > p.maxSize {
		p.head = p.head.next
		p.size--
	}
}

func (p *BufferPool) Head() *BufferPoolEntry {
	return p.head
}

func (p *BufferPool) Size() int {
	return p.size
}

func (p *BufferPool) MaxSize() int {
	return p.maxSize
}

func (p *BufferPool) CheckedOutCount() int {
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.checkedOutCount
}

func NewBufferPool(maxSize int) *BufferPool {
	return &BufferPool{
		lock:    new(sync.Mutex),
		head:    nil,
		size:    0,
		maxSize: maxSize,
	}
}

var GlobalBufPool *BufferPool = NewBufferPool(DEFAULT_BUF_POOL_MAX_SIZE)

func SetGlobalBufferPoolFromArgs() {
	GlobalBufPool.SetMaxSize(*BufPoolArgsMaxSize)
	Log.Infof("Global buffer pool max size: %d", GlobalBufPool.MaxSize())
}
