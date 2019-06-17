// Package types defines types that are used throughout the
// service, primarily struct types with JSON mappings and
// custom types used within those types, that are used for
// REST requests and respones.
package types

// StatusResponse is the JSON returned for a liveness check as well as
// for other status notifications such as a successful delete.
type StatusResponse struct {
	Status string `json:"status"`
}
