package httpx

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	apperrors "github.com/BwCloudWeGo/bw-cli/pkg/errors"
)

// 契约业务错误码（《接口文档 v2.0》3.6 节信封 code）。
const (
	PaxCodeOK                = 0
	PaxCodeInvalidArgument   = 40001
	PaxCodeUnauthorized      = 40101
	PaxCodeNotFound          = 40401
	PaxCodeConflict          = 40901
	PaxCodeRatingExpired     = 41001
	PaxCodeIDNoTaken         = 42201
	PaxCodeRealNameRateLimit = 42901
	PaxCodeInternal          = 50001
)

// PaxBody 是 /api/pax/* 响应的契约信封 {code,message,data}。
// 仅乘客端接口使用；现有 /api/v1 继续用 Response 外壳。
type PaxBody struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// PaxOK 写出 code=0 的成功信封。
func PaxOK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, PaxBody{Code: PaxCodeOK, Message: "success", Data: data})
}

// PaxError 写出指定 HTTP 状态与业务码的错误信封。
func PaxError(c *gin.Context, status, code int, msg string) {
	c.Set("error_code", strconv.Itoa(code))
	c.JSON(status, PaxBody{Code: code, Message: msg, Data: nil})
}

// PaxAppError 将应用错误映射为契约错误信封：
// AppError.Code 为数值业务码（41001/42201/42901 等）时原样透出，
// 否则按 Kind 映射附录 A 默认错误码。
func PaxAppError(c *gin.Context, err error) {
	appErr, ok := apperrors.As(err)
	if !ok {
		appErr = apperrors.Internal("internal_error", "internal error")
	}
	code := paxCode(appErr)
	PaxError(c, apperrors.HTTPStatus(appErr), code, appErr.Message)
}

func paxCode(err *apperrors.AppError) int {
	// 数值型自定义业务码（41001/42201/42901 等）原样透出；拒绝 0/负数防“像成功”的错误信封。
	if n, convErr := strconv.Atoi(err.Code); convErr == nil && n > 0 {
		return n
	}
	switch err.Kind {
	case apperrors.KindInvalidArgument:
		return PaxCodeInvalidArgument
	case apperrors.KindUnauthorized, apperrors.KindForbidden:
		return PaxCodeUnauthorized
	case apperrors.KindNotFound:
		return PaxCodeNotFound
	case apperrors.KindConflict:
		return PaxCodeConflict
	default:
		return PaxCodeInternal
	}
}
