package cmd

// ExitError carries an exit code for CLI termination.
type ExitError struct {
	Code     int
	Err      error
	Internal bool
	Silent   bool
}

func (e *ExitError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return ""
}

func (e *ExitError) Unwrap() error {
	return e.Err
}

func userError(err error) *ExitError {
	return &ExitError{Code: 2, Err: err}
}

func silentExit(code int) *ExitError {
	return &ExitError{Code: code, Silent: true}
}

func internalError(err error) *ExitError {
	return &ExitError{Code: 3, Err: err, Internal: true}
}

func fatalError(err error) *ExitError {
	return &ExitError{Code: 3, Err: err}
}
