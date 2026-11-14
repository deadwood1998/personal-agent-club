package extension

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/nwverse/personal-agent-club/internal/core"
	"github.com/nwverse/personal-agent-club/internal/protocol"
)

// call answers method in the default space through core.Club.Call, the path every shell uses, and decodes its answer
// into out (nil: dropped).
func call(ctx context.Context, club *core.Club, method string, params, out any) error {
	raw, err := json.Marshal(params)
	if err != nil {
		return err
	}
	res, err := club.Call(ctx, core.DefaultSpace, method, raw)
	if err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	b, err := json.Marshal(res)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

// reason is an error in words for the page and the assistant: the protocol code said plainly, then the core's own
// message. secret, when given, is never repeated (a credential the user just typed).
func reason(l lang, err error, secret string) string {
	msg := err.Error()
	if secret != "" && strings.Contains(msg, secret) {
		msg = strings.ReplaceAll(msg, secret, "***")
	}
	var pe *protocol.Error
	if errors.As(err, &pe) {
		switch pe.Code {
		case protocol.CodeUnsupported:
			return l.t("不支持：", "Not supported: ") + msg
		case protocol.CodeInvalid:
			return l.t("无效：", "Invalid: ") + msg
		case protocol.CodeNotFound:
			return l.t("找不到：", "Not found: ") + msg
		case protocol.CodeConflict:
			return l.t("冲突：", "Conflict: ") + msg
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return l.t("超时了，稍后再试", "Timed out; try again later")
	}
	return l.t("失败：", "Failed: ") + msg
}

// lang picks the words for a locale: Chinese for zh-*, and the hub's default (zh-Hans) when there is none; English
// otherwise.
type lang string

func (l lang) t(zh, en string) string {
	if l == "" || strings.HasPrefix(string(l), "zh") {
		return zh
	}
	return en
}
