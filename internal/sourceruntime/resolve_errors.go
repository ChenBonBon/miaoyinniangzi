package sourceruntime

import "errors"

var (
	ErrResolveTimeout   = errors.New("音源解析超时，请重试")
	ErrResolveCanceled  = errors.New("音源解析已取消")
	ErrResolveResources = errors.New("音源隔离进程超出资源限制")
	ErrResolveProtocol  = errors.New("音源解析协议异常，当前脚本可能不兼容")
	ErrResolveProcess   = errors.New("音源隔离进程异常退出，请重试")
	ErrResolveMedia     = errors.New("音源返回的媒体地址不符合安全要求")
	ErrResolveMediaDNS  = errors.New("无法校验媒体地址的公网解析，请检查网络后重试")
)

// Only these fixed errors may cross the source manager boundary. Never forward
// arbitrary interpreter, proxy, Keychain or OS errors containing URLs/keys.
func PublicResolveError(err error) (string, bool) {
	for _, known := range []error{ErrResolveTimeout, ErrResolveCanceled, ErrResolveResources, ErrResolveProtocol, ErrResolveProcess, ErrResolveMedia, ErrResolveMediaDNS} {
		if errors.Is(err, known) {
			return known.Error(), true
		}
	}
	return "", false
}
