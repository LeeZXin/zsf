package bizerr

import "fmt"

type Err struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Err) Error() string {
	return fmt.Sprintf("ErrCode: %d, Message: %s", e.Code, e.Message)
}

func NewBizErr(code int, message string) *Err {
	return &Err{
		Code:    code,
		Message: message,
	}
}
