package pvmi

import (
	"bytes"
	"testing"
)

func TestBufPoolBuildPool(t *testing.T) {
	maxSize := 23
	p := NewBufferPool(maxSize)
	if p.Size() != 0 {
		t.Fatalf("p.Size(): want=%d, got=%d", 0, p.Size())
	}
	if maxSize != p.MaxSize() {
		t.Fatalf("p.MaxSize(): want=%d, got=%d", maxSize, p.MaxSize())
	}
	if p.Head() != nil {
		t.Fatalf("p.Head(): want=%v, got=%v", nil, p.Head())
	}
}

func TestBufPoolPoolCap(t *testing.T) {
	maxSize := 2
	p := NewBufferPool(maxSize)
	for k := int(0); k <= maxSize; k++ {
		p.ReturnBuffer(nil)
	}
	if p.Size() != p.MaxSize() {
		t.Fatalf("p.Size(): want=%d, got=%d", p.MaxSize(), p.Size())
	}
}

func TestBufPoolGetBuffer(t *testing.T) {
	p := NewBufferPool(2)
	b := p.GetBuffer()
	if b.Cap() != 0 {
		t.Fatalf("b.Cap():  want=%d, got=%d", 0, b.Cap())
	}
	testData := "TestGetBuffer"
	b.WriteString(testData)
	p.ReturnBuffer(b)
	b = p.GetBuffer()
	// The buffer should have been reset:
	if b.Len() != 0 {
		t.Fatalf("b.Len(): want=%d, got=%d", 0, b.Len())
	}
	// but it should retain the original data:
	if b.Cap() < len(testData) {
		t.Fatalf("b.Cap(): want>=%d, got=%d", len(testData), b.Cap())
	}
	got := string(b.Bytes()[:len(testData)])
	if got != testData {
		t.Fatalf("b: want=%#v, got=%#v", testData, got)
	}
}

func TestBufPoolSetMaxSizeUp(t *testing.T) {
	initialMaxSize := 16
	p := NewBufferPool(initialMaxSize)
	for i := 0; i < initialMaxSize; i++ {
		b := bytes.NewBuffer([]byte{byte(i)})
		p.ReturnBuffer(b)
	}
	// At this point p.Head() -> b(15) -> b(14) -> ... -> b(0)
	newMaxSize := initialMaxSize + 2
	p.SetMaxSize(newMaxSize)
	if p.MaxSize() != newMaxSize {
		t.Fatalf("p.MaxSize(): want=%d, got=%d", newMaxSize, p.MaxSize())
	}
	if p.Size() != initialMaxSize {
		t.Fatalf("p.Size(): want=%d, got=%d", newMaxSize, p.Size())
	}
	// Test that no buffers were removed from the list:
	for i := initialMaxSize - 1; i >= 0; i-- {
		b := p.GetBuffer().Bytes()[:1]
		want := byte(i)
		if b[0] != want {
			t.Fatalf("GetBuffer# %d, b[0]: want=%d, got=%d", newMaxSize-i, want, b[0])
		}
	}

	// The next get buffer should return an empty one:
	b := p.GetBuffer()
	if b.Cap() != 0 {
		t.Fatalf("b.Cap():  want=%d, got=%d", 0, b.Cap())
	}
}

func TestBufPoolSetMaxSizeDown(t *testing.T) {
	initialMaxSize := 16
	p := NewBufferPool(initialMaxSize)
	for i := 0; i < initialMaxSize; i++ {
		b := bytes.NewBuffer([]byte{byte(i)})
		p.ReturnBuffer(b)
	}

	// At this point p.Head() -> b(15) -> b(14) -> ... -> b(0)
	newMaxSize := initialMaxSize - 2
	p.SetMaxSize(newMaxSize)
	if p.MaxSize() != newMaxSize {
		t.Fatalf("p.MaxSize(): want=%d, got=%d", newMaxSize, p.MaxSize())
	}
	if p.Size() != newMaxSize {
		t.Fatalf("p.Size(): want=%d, got=%d", newMaxSize, p.Size())
	}

	// Test that the most recent buffers were indeed removed from the list;
	// expected: p.Head() -> b(11) -> b(10) -> ... -> b(0)
	for i := newMaxSize - 1; i >= 0; i-- {
		b := p.GetBuffer().Bytes()[:1]
		want := byte(i)
		if b[0] != want {
			t.Fatalf("GetBuffer# %d, b[0]: want=%d, got=%d", newMaxSize-i, want, b[0])
		}
	}

	// The next get buffer should return an empty one:
	b := p.GetBuffer()
	if b.Cap() != 0 {
		t.Fatalf("b.Cap():  want=%d, got=%d", 0, b.Cap())
	}
}

func TestBufPoolCheckoutCount(t *testing.T) {
	maxSize := 16
	p := NewBufferPool(maxSize)
	for i := 1; i <= 2*maxSize; i++ {
		p.GetBuffer()
		if p.checkedOutCount != i {
			t.Errorf("check out count: want %d, got %d", i, p.checkedOutCount)
		}
	}
	for i := 2*maxSize - 1; i >= 0; i-- {
		p.ReturnBuffer(nil)
		if p.checkedOutCount != i {
			t.Errorf("check in count: want %d, got %d", i, p.checkedOutCount)
		}
	}
}
