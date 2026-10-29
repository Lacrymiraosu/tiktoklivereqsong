package app

// UserError is an error the dashboard shows to the streamer. Code (and Vars)
// are translated there as T["e_"+code]; Msg is the English text for the log.
type UserError struct {
	Code string
	Vars map[string]string
	Msg  string
}

func (e *UserError) Error() string { return e.Msg }

// ErrCode returns the code and variables for translation.
func (e *UserError) ErrCode() (string, map[string]string) { return e.Code, e.Vars }

func uerr(code, msg string, vars map[string]string) error {
	return &UserError{Code: code, Vars: vars, Msg: msg}
}
