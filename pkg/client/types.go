package client

import (
	"encoding/json"
	"fmt"
)

type ErrorType string

const (
	ErrBadData		ErrorType	= "bad_data"
	ErrTimeout					= "timeout"
	ErrCanceled					= "canceled"
	ErrExec						= "execution"
	ErrBadResponse				= "bad_response"
)

type Error struct {
	Type	ErrorType
	Msg		string
}

func (e *Error) Error() string {
	_logClusterCodePath()
	defer _logClusterCodePath()
	return fmt.Sprintf("%s: %s", e.Type, e.Msg)
}

type ResponseStatus string

const (
	ResponseSucceeded	ResponseStatus	= "succeeded"
	ResponseError						= "error"
)

type APIResponse struct {
	Status		ResponseStatus	`json:"status"`
	Data		json.RawMessage	`json:"data"`
	ErrorType	ErrorType		`json:"errorType"`
	Error		string			`json:"error"`
}
