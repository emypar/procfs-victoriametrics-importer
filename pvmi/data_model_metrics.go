// Generic metrics generators for data model based metrics

package pvmi

import (
	"bytes"
	"fmt"
	"strconv"
	"time"

	"github.com/prometheus/procfs"
)

// Generate metrics for dual buffer FixedLayoutDataModel:
type FixedLayoutDataModelParserFn func(*procfs.FixedLayoutDataModel) (*procfs.FixedLayoutDataModel, error)
type BuildFldmFmtListFn func(*procfs.FixedLayoutDataModel) []string
type BuildFldmMetricListFn func(*procfs.FixedLayoutDataModel) []string

func buildFldmFmtList(fldm *procfs.FixedLayoutDataModel, metricNameMap map[string]string, hostname, job string) []string {
	fmtList := make([]string, len(fldm.Names))
	for i, name := range fldm.Names {
		metricName := metricNameMap[name]
		if metricName == "" {
			fmtList[i] = ""
		} else {
			fmtList[i] = fmt.Sprintf(
				`%s{%s="%s",%s="%s"} %%s %%s`+"\n",
				metricName, HOSTNAME_LABEL_NAME, hostname, JOB_LABEL_NAME, job,
			)
		}
	}
	return fmtList
}

func buildFldmMetricList(fldm *procfs.FixedLayoutDataModel, metricNameMap map[string]string, hostname, job string) []string {
	metricList := make([]string, len(fldm.Names))
	for i, name := range fldm.Names {
		metricName := metricNameMap[name]
		if metricName == "" {
			metricList[i] = ""
		} else {
			metricList[i] = fmt.Sprintf(
				`%s{%s="%s",%s="%s"} `, // note ending space such that the metric value can be concatenated directly
				metricName, HOSTNAME_LABEL_NAME, hostname, JOB_LABEL_NAME, job,
			)
		}
	}
	return metricList
}

func GenerateFldmDataModelMetrics(
	fldms []*procfs.FixedLayoutDataModel,
	crtIndex int,
	parserFn FixedLayoutDataModelParserFn,
	metricList *[]string,
	buildMetricListFn BuildFldmMetricListFn,
	fullMetricsFactor int,
	refreshCycleNum int,
	timeNow func() time.Time,
	buf *bytes.Buffer,
) (metricCount int, err error) {
	crtFldm, prevFldm := fldms[crtIndex], fldms[1-crtIndex]
	if crtFldm == nil && prevFldm != nil {
		crtFldm = prevFldm.ShallowCopy(false)
		fldms[crtIndex] = crtFldm
	}
	if crtFldm == nil {
		crtFldm, err = parserFn(nil)
		fldms[crtIndex] = crtFldm
	} else {
		_, err = parserFn(crtFldm)
	}
	if err != nil {
		return
	}
	promTs := strconv.FormatInt(timeNow().UnixMilli(), 10)
	suffix := " " + promTs + "\n"
	metrics := *metricList
	if metrics == nil {
		metrics = buildMetricListFn(crtFldm)
		*metricList = metrics
	}

	if fullMetricsFactor <= 1 || prevFldm == nil {
		for i, val := range crtFldm.Values {
			metric := metrics[i]
			if metric != "" {
				buf.WriteString(metric + val + suffix)
				metricCount++
			}
		}
	} else {
		metricCycleNum := 0
		prevValues := prevFldm.Values
		for i, val := range crtFldm.Values {
			metric := metrics[i]
			if metric != "" && (metricCycleNum == refreshCycleNum || val != prevValues[i]) {
				buf.WriteString(metric + val + suffix)
				metricCount++
			}
			if metricCycleNum++; metricCycleNum >= fullMetricsFactor {
				metricCycleNum = 0
			}
		}
	}

	if metricCount > 0 {
		buf.WriteByte('\n')
	}
	return
}
