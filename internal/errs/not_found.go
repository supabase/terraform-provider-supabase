package errs

import (
	"errors"
	"fmt"
)

// NotFoundErr indicates that a requested remote object does not exist.
type NotFoundErr struct {
	// Resource describes the kind of object, for example "third-party auth integration".
	Resource string
	// ID identifies the object that was not found. It may be empty.
	ID string
}

// NewNotFoundErr returns a NotFoundErr for the given resource kind and ID.
func NewNotFoundErr(resource, id string) *NotFoundErr {
	return &NotFoundErr{Resource: resource, ID: id}
}

// Error implements the error interface.
func (e *NotFoundErr) Error() string {
	switch {
	case e.Resource == "" && e.ID == "":
		return "not found"
	case e.ID == "":
		return fmt.Sprintf("%s not found", e.Resource)
	case e.Resource == "":
		return fmt.Sprintf("%q not found", e.ID)
	default:
		return fmt.Sprintf("%s %q not found", e.Resource, e.ID)
	}
}

// NotFound reports whether err, or any error it wraps, is a NotFoundErr.
func NotFound(err error) bool {
	var target *NotFoundErr
	return errors.As(err, &target)
}
