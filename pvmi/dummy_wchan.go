// dummy wChan for exercising metrics generators (consume and discard)

package pvmi

import (
	"bytes"
	"fmt"
)

func DummyWChan(bufPool *BufferPool, capacity int, display_metrics bool) chan *bytes.Buffer {
	wChan := make(chan *bytes.Buffer, capacity)

	go func() {
		for buf := range wChan {
			if display_metrics {
				fmt.Print(buf.String())
			}
			bufPool.ReturnBuffer(buf)
		}
	}()

	return wChan
}

func DefaultDummyWChan(display_metrics bool) chan *bytes.Buffer {
	return DummyWChan(GlobalBufPool, 256, display_metrics)
}
