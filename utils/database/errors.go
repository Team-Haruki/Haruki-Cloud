package database

import "errors"

var (
	ErrClientNotInitialized = errors.New("haruki DB API Client is not initialized")
	ErrInvalidDatabaseType  = errors.New("you have entered an invalid database type")
)
