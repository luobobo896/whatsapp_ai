package apperror

import "net/http"

type Error struct {
	Code    string
	Status  int
	Message string
	Details any
	Cause   error
}

func E(code string, status int, message string, cause error) *Error {
	return &Error{Code: code, Status: status, Message: message, Cause: cause}
}

func (err *Error) Error() string { return err.Message }

func (err *Error) Unwrap() error { return err.Cause }

func AuthRequired() *Error {
	return E("AUTH_REQUIRED", http.StatusUnauthorized, "Authentication is required.", nil)
}

func SessionExpired() *Error {
	return E("SESSION_EXPIRED", http.StatusUnauthorized, "Your session has expired.", nil)
}

func AuthInvalid() *Error {
	return E("AUTH_INVALID", http.StatusUnauthorized, "Email or password is incorrect.", nil)
}

func Forbidden() *Error {
	return E("FORBIDDEN", http.StatusForbidden, "You do not have permission to perform this action.", nil)
}

func TenantSuspended() *Error {
	return E("TENANT_SUSPENDED", http.StatusForbidden, "This tenant is suspended.", nil)
}

func Conflict(message string) *Error {
	if message == "" {
		message = "The requested change conflicts with the current state."
	}
	return E("CONFLICT", http.StatusConflict, message, nil)
}

func Validation(message string, details any) *Error {
	if message == "" {
		message = "The request is invalid."
	}
	err := E("VALIDATION_FAILED", http.StatusUnprocessableEntity, message, nil)
	err.Details = details
	return err
}

func RateLimited() *Error {
	return E("RATE_LIMITED", http.StatusTooManyRequests, "Too many requests. Try again later.", nil)
}

func NotFound() *Error {
	return E("NOT_FOUND", http.StatusNotFound, "The requested resource was not found.", nil)
}

func Internal(cause error) *Error {
	return E("INTERNAL_ERROR", http.StatusInternalServerError, "An internal error occurred.", cause)
}
