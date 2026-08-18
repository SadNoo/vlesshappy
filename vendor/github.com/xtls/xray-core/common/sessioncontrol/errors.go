package sessioncontrol

import "errors"

var ErrControllerRegistered = errors.New("session controller is already registered")
var ErrSourceUnavailable = errors.New("session source is unavailable")
