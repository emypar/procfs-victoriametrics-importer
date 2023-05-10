module github.com/eparparita/procfs-victoriametrics-importer

go 1.19

require (
	github.com/google/go-cmp v0.5.9
	github.com/gookit/goutil v0.6.6
	github.com/prometheus/procfs v0.9.0
	github.com/sirupsen/logrus v1.9.0
	github.com/tklauser/go-sysconf v0.3.11
	golang.org/x/sys v0.7.0
)

replace github.com/prometheus/procfs => ../procfs

require (
	github.com/gookit/color v1.5.2 // indirect
	github.com/tklauser/numcpus v0.6.0 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	golang.org/x/text v0.7.0 // indirect
)
