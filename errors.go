package outbox

import "errors"

var (
	ErrDuplicatedKey = errors.New("duplicated key")
)
