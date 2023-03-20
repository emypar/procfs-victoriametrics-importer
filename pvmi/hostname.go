package pvmi

import (
	"os"
)

const (
	HOSTNAME_LABEL_NAME = "hostname"
)

var Hostname string

func init() {
	var err error
	Hostname, err = os.Hostname()
	if err != nil {
		Log.Warn(err)
		Hostname = ""
	}
}
