// Framework for testing metrics generators.
//
// A metrics generator will assemble newline ("\n") separated metrics into
// bytes.Buffer and it will write the latter, as they fill up, to a channel.
// This file provides a function for reading the channel and for comparing the
// generated metrics against the expected ones, the later registered prior to
// generator invocation. The function will report all the unexpected/missing
// metrics using the testing.T methods.

package testutils

import (
	"bytes"
	"sort"
	"strings"
	"sync"
	"testing"
)

type MetricsSet map[string]bool
type ExpectedMetrics struct {
	expectedSet MetricsSet
}

func NewExpectedMetrics() *ExpectedMetrics {
	return &ExpectedMetrics{
		expectedSet: make(MetricsSet),
	}
}

func (em *ExpectedMetrics) AddExpectedMetric(m string) {
	em.expectedSet[m] = true
}

func (em *ExpectedMetrics) AddExpectedMetrics(mList []string) {
	for _, m := range mList {
		em.expectedSet[m] = true
	}
}

func (em *ExpectedMetrics) VerifyMetric(m string) bool {
	_, ok := em.expectedSet[m]
	if ok {
		// Desireable side effect: this will discover duplicates.
		delete(em.expectedSet, m)
	}
	return ok
}

func (em *ExpectedMetrics) VerifyBuf(buf *bytes.Buffer, t *testing.T) bool {
	ok := true
	for _, bMetric := range bytes.Split(buf.Bytes(), []byte("\n")) {
		metric := strings.TrimSpace(string(bMetric))
		if metric != "" && !em.VerifyMetric(metric) {
			t.Errorf("unexpected metric: `%s' for %s", metric, t.Name())
			ok = false
		}
	}
	// What is left in em are missing metrics:
	for metric, _ := range em.expectedSet {
		t.Errorf("missing metric: `%s' for %s", metric, t.Name())
		ok = false
	}
	return ok
}

// Invoke metrics generator function mgf and compare the metrics; mgf should be
// a wrapper around the actual generator with all args but wChan predefined.
// Use brf for returning metrics buffers to the pool.
func (em *ExpectedMetrics) RunMetricsGenerator(mgf func(wChan chan *bytes.Buffer), brf func(*bytes.Buffer), t *testing.T) {
	wChan := make(chan *bytes.Buffer, 256)
	var wg sync.WaitGroup
	wg.Add(1)

	// Start a goroutine reading the channel from mgf and verifying metric by metric:
	unexpectedMetrics := make(MetricsSet)
	go func() {
		defer wg.Done()
		for buf := range wChan {
			for _, bMetric := range bytes.Split(buf.Bytes(), []byte("\n")) {
				metric := strings.TrimSpace(string(bMetric))
				if metric != "" && !em.VerifyMetric(metric) {
					unexpectedMetrics[metric] = true
				}
			}
			brf(buf)
		}
	}()

	// Invoke the metrics generator:
	mgf(wChan)

	// Signal the channel reading goroutine to exit:
	close(wChan)
	wg.Wait()

	// What is left in em are missing metrics:
	for metric, _ := range em.expectedSet {
		t.Errorf("missing metric: `%s' for %s", metric, t.Name())
	}
	// Report unexpected:
	for metric, _ := range unexpectedMetrics {
		t.Errorf("unexpected metric: `%s' for %s", metric, t.Name())
	}
}

func DuplicateStrings(src []string, sorted bool) []string {
	dst := make([]string, len(src))
	copy(dst, src)
	if sorted {
		sort.Strings(dst)
	}
	return dst
}

func BufToStrings(buf *bytes.Buffer, sorted bool) []string {
	dst := make([]string, 0)
	for _, bMetric := range bytes.Split(buf.Bytes(), []byte("\n")) {
		metric := strings.TrimSpace(string(bMetric))
		if metric != "" {
			dst = append(dst, metric)
		}
	}
	if sorted {
		sort.Strings(dst)
	}
	return dst
}

// Invoke metrics generator function mgf and collect all  the metrics; mgf
// should be a wrapper around the actual generator with all args but the buffer
// pool and wChan predefined. The collector may be passed a function for
// returning the buffers to the pool.
func CollectMetrics(
	mgf func(wChan chan *bytes.Buffer),
	brf func(*bytes.Buffer),
) []string {
	wChan := make(chan *bytes.Buffer, 256)
	var wg sync.WaitGroup
	wg.Add(1)

	// Start a goroutine reading the channel from mgf and verifying metric by metric:
	collected := make([]string, 0)
	go func() {
		defer wg.Done()
		for buf := range wChan {
			collected = append(collected, BufToStrings(buf, false)...)
			if brf != nil {
				brf(buf)
			}
		}
	}()

	// Invoke the metrics generator:
	mgf(wChan)

	// Signal the channel reading goroutine to exit:
	close(wChan)
	wg.Wait()

	// Return the collected metrics:
	return collected
}
