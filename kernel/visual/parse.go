package visual

import (
	"strconv"
	"strings"
)

// parseInt 解析整数（去空白）。
func parseInt(s string) (int, error) {
	return strconv.Atoi(strings.TrimSpace(s))
}

// parseInt64 解析 int64。
func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
}

// parseFloat 解析浮点。
func parseFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.TrimSpace(s), 64)
}
