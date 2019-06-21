// Package types defines types that are used throughout the
// service, primarily struct types with JSON mappings and
// custom types used within those types, that are used for
// REST requests and respones.
package types

import (
	"encoding/json"
)

const (
	MaxSpeed = 500
)

// StatusResponse is the JSON returned for a liveness check as well as
// for other status notifications such as a successful delete.
type StatusResponse struct {
	Status string `json:"status"`
}

//VerifyRequest is the struct corresponding to the JSON sent
// by the user to record a login and check suspicion.
type VerifyRequest struct {
	Username      string `json:"username"`
	UnixTimestamp int64  `json:"unix_timestamp"`
	EventUUID     string `json:"event_uuid"`
	IPAddress     string `json:"ip_address"`
}

type CurrentGeoStat struct {
	Lat    float64 `json:"lat"`
	Lon    float64 `json:"lon"`
	Radius uint16  `json:"radius"`
}

// GeoEvent is used in the Verify response, as either the preceding
// or subsequent location.  It also indicates the "speed", and whether
// it is considered suspicious
type GeoEvent struct {
	IP         string  `json:"ip"`
	Speed      int64   `json:"speed"`
	Suspicious bool    `json:"suspicious"`
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
	Radius     uint16  `json:"radius"`
	Timestamp  int64   `json:"timestamp"`
}

// VerifyResponse corresponds to the serialized JSON response.  Note both
// the preceding subsequent access items are pointers, so they may be omitted
// from the JSON if not p[resent.]
type VerifyResponse struct {
	CurrentGeo         CurrentGeoStat `json:"currentGeo"`
	PrecedingIPAccess  *GeoEvent      `json:"precedingIpAccess,omitempty"`
	SubsequentIPAccess *GeoEvent      `json:"subsequentIpAccess,omitempty"`
}

func (v VerifyResponse) String() string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
