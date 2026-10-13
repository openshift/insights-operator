package authorizer

type Error struct {
	Err error
}

func (e Error) Error() string {
	return e.Err.Error()
}

func IsAuthorizationError(err error) bool {
	switch err.(type) {
	case Error:
		return true
	default:
		return false
	}
}
