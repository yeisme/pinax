package continuityevidence

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// 小工具函数集中放在这里，避免依赖散落到 report.go。

func itoa(value int) string { return strconv.Itoa(value) }

func fmtSprintf(format string, args ...any) string { return fmt.Sprintf(format, args...) }

func joinStrings(values []string, sep string) string { return strings.Join(values, sep) }

func sortStrings(values []string) { sort.Strings(values) }

func joinSorted(values []string) string {
	sorted := make([]string, len(values))
	copy(sorted, values)
	sort.Strings(sorted)
	return strings.Join(sorted, ",")
}
