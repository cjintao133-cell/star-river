package secret

import "errors"

// errNoDelegate 未注入平台 Delegate 时返回。
var errNoDelegate = errors.New("secret: no platform delegate configured")
