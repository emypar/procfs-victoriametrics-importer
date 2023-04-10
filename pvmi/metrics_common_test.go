// Common definitions for metrics tests.

package pvmi

import "path"

const (
	PVMI_TOP_DIR = ".."
)

var TestdataProcfsRoot = path.Join(PVMI_TOP_DIR, "testdata/proc")
var TestdataTestCasesDir = path.Join(PVMI_TOP_DIR, "testdata/testcases")
var TestClktckSec = float64(0.01)
var TestHostname = "test-host"
var TestSource = "testpvmi"
