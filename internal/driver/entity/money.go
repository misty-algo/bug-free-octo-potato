package entity

import (
	"fmt"
	"strings"

	"github.com/BwCloudWeGo/bw-cli/pkg/contracts"
)

// contractAmount 是跨模块契约中的金额结构（单位：整数分）。
type contractAmount = contracts.TaskAmount

// FenToYuan 分转元（仅用于展示与文案，内部计算一律用分）。
func FenToYuan(fen int64) float64 { return float64(fen) / 100.0 }

// FormatFen 输出元的字符串形式（保留两位小数）。
func FormatFen(fen int64) string { return fmt.Sprintf("%.2f", FenToYuan(fen)) }

func fmtFen(format string, fen int64) string { return fmt.Sprintf(format, FormatFen(fen)) }

func trimRightZeros(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	return s
}
