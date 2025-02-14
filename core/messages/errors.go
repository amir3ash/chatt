package messages

import "fmt"

type ErrNotFound struct {
	Type string
	ID   string
}

func (e ErrNotFound) Error() string {
	return fmt.Sprintf("type '%s' with id '%s' not found", e.Type, e.ID)
}

type ErrNotAuthorized struct {
	Subject     string
	ResorceType string
	ResorceId   string
}

func (e ErrNotAuthorized) Error() string {
	return fmt.Sprintf("subject %s can not access %s with id %s", e.Subject, e.ResorceType, e.ResorceId)
}
