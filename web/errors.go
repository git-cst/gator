package web

import "errors"

// Sentinel errors for current user resolution
var (
	errNoUser       = errors.New("no user provided")
	errInvalidUser  = errors.New("invalid user")
	errInternalUser = errors.New("internal server error")
)

var (
	errNoFeeds       = errors.New("no feeds")
	errInternalFeeds = errors.New("internal server error")
)
