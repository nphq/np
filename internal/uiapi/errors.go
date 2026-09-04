package uiapi

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nphq/np/internal/config"
	"github.com/nphq/np/internal/secure"
)

// ErrorCode 是统一的 IPC 错误码。
type ErrorCode string

const (
	CodeInvalidInput ErrorCode = "invalid_input"
	CodeNotFound     ErrorCode = "not_found"
	CodeDuplicate    ErrorCode = "duplicate"
	CodeInternal     ErrorCode = "internal"
)

// Error 是返回给前端的统一错误结构。
type Error struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

// Error 让 *Error 实现 error 接口（用于内部 errors.As / fmt.Errorf("%w") 等）。
// **nil 安全**：Wails 调度器在 case-1/case-2 路径上会对返回值做 `.(error)`
// 类型断言并调用 .Error()；当方法返回 nil *Error（成功）时，断言拿到的是
// typed-nil，调用 .Error() 会解引用 nil 指针 panic。nil 时返回 "" 让该路径
// 安全降级（前端 if(err) 仍按 "" 处理为 falsy）。
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Wrap 把任意 error 归一到 Error（保持原 Code 若已是 *Error）。
func Wrap(err error) *Error {
	if err == nil {
		return nil
	}
	var ie *Error
	if errors.As(err, &ie) {
		return ie
	}
	switch {
	case errors.Is(err, config.ErrClusterNotFound):
		return &Error{Code: CodeNotFound, Message: err.Error()}
	case errors.Is(err, config.ErrClusterExists):
		return &Error{Code: CodeDuplicate, Message: err.Error()}
	case errors.Is(err, secure.ErrTokenNotFound):
		return &Error{Code: CodeNotFound, Message: "token not found in keychain"}
	default:
		return mapNomadError(err)
	}
}

// mapNomadError 把 Nomad HTTP/API 错误映射到前端可区分的码：
// 404/“not found” → not_found；401/403/ACL 拒绝 → invalid_input（需换 token/提权）；
// 其余 → internal。避免前端把“无权限”“不存在”“服务端挂了”混为一谈。
func mapNomadError(err error) *Error {
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(msg, "404"),
		strings.Contains(lower, "not found"),
		strings.Contains(lower, "no such"):
		return &Error{Code: CodeNotFound, Message: msg}
	case strings.Contains(msg, "401"),
		strings.Contains(msg, "403"),
		strings.Contains(lower, "permission denied"),
		strings.Contains(lower, "acl"),
		strings.Contains(lower, "forbidden"),
		strings.Contains(lower, "unauthorized"):
		return &Error{Code: CodeInvalidInput, Message: "forbidden (check token/ACL): " + msg}
	default:
		return &Error{Code: CodeInternal, Message: msg}
	}
}

// NewError 构造错误。
func NewError(code ErrorCode, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
