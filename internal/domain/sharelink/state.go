package sharelink

import "strings"

type State string

const (
	StateActive  State = "active"
	StateExpired State = "expired"
	StateRevoked State = "revoked"
	StateUnknown State = "unknown"
)

func ParseState(value string) State {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case string(StateActive):
		return StateActive
	case string(StateExpired):
		return StateExpired
	case string(StateRevoked):
		return StateRevoked
	default:
		return StateUnknown
	}
}
