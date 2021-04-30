// Package service implements the functionality of the IP Verify service.  It
// is shielded from HTTP specifcs and JSON marshaling by the API layer, and
// it manages the data store, which is mplemented in the store package.
package service

import (
	"fmt"
	"math"
	"net"

	"github.com/gdotgordon/ipverify/store"
	"github.com/gdotgordon/ipverify/types"
	"github.com/oschwald/maxminddb-golang"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

const (
	// Haversine constants
	kmtomiles   = float64(0.621371192)
	earthRadius = float64(6371)
)

// Location is the struct returned by Maxmind DB lookup results.
type Location struct {
	AccuracyRadius uint16  `maxminddb:"accuracy_radius"`
	Latitude       float64 `maxminddb:"latitude"`
	Longitude      float64 `maxminddb:"longitude"`
	MetroCode      uint    `maxminddb:"metro_code"`
	TimeZone       string  `maxminddb:"time_zone"`
}

// Error is used to tag internal server errors to distinguish them from
// other things, such as user errors.
func (e Error) Error() string {
	return string(e)
}

type Error string

// Service defines the sets of functions handled by IP verify service
type Service interface {
	VerifyIP(types.VerifyRequest) (*types.VerifyResponse, error)
	ResetStore() error
}

// VerifyService is the implementation of Service that performs verification
// that a given login attempt is not suspicious, based on the speed criterion.
// It does geolcation lookups of IP address using the Maxmind database, and checks
// the incoming request against previously recorded events in the database,
// determining whether the request is suspicious.
type VerifyService struct {
	mmReader *maxminddb.Reader
	store    store.Store
	log      *zap.SugaredLogger
}

// New creates a new VerifyService, configured with a datastore and logger.
func New(mmDBPath string, store store.Store, log *zap.SugaredLogger) (*VerifyService, error) {
	mmReader, err := maxminddb.Open(mmDBPath)
	if err != nil {
		return nil, Error(err.Error())
	}
	return &VerifyService{mmReader: mmReader, store: store, log: log}, nil
}

// VerifyIP is the main call to check for suspicious activity, given the current
// incoming login.
func (vs *VerifyService) VerifyIP(req types.VerifyRequest) (*types.VerifyResponse, error) {

	// First add the current record to the store.  This will reduce the vulnerability
	// of two nearly simultaneous requests missing each other's new event.
	if err := vs.store.AddRecord(req); err != nil {
		return nil, errors.Wrap(err, "add record to store")
	}

	// A GeoEvent is the data for the previous and next requests relative
	// to the incoming request.  Both may or may bot be present.
	var pge, nge *types.GeoEvent
	var resp types.VerifyResponse

	// Now get the prior and next items (if they exist) from the store.
	prev, nxt, err := vs.store.GetPriorNext(req.Username, req.EventUUID, req.UnixTimestamp)
	if err != nil {
		return nil, errors.Wrap(err, "getting prior and subsequent records")
	}

	// Get the coordinates and radius for the incoming request.
	curLoc, err := lookupIP(req.IPAddress, vs.mmReader, vs.log)
	if err != nil {
		return nil, errors.Wrap(err, "IP lookup")
	}

	// Fill in the part of the response object for the current request.
	resp.CurrentGeo.Lat = curLoc.Latitude
	resp.CurrentGeo.Lon = curLoc.Longitude
	resp.CurrentGeo.Radius = curLoc.AccuracyRadius

	// Compute the speeds and preapre the response section for the previous
	// and next items (if any).
	if prev != nil {
		pge, err = vs.geoEventFromRequest(curLoc, &req, prev)
		if err != nil {
			return nil, errors.Wrap(err, "preparing return data")
		}
	}
	if nxt != nil {
		nge, err = vs.geoEventFromRequest(curLoc, &req, nxt)
		if err != nil {
			return nil, errors.Wrap(err, "calculating verify data")
		}
	}
	resp.PrecedingIPAccess = pge
	resp.SubsequentIPAccess = nge
	return &resp, nil
}

// ResetStore clears the database.
func (vs *VerifyService) ResetStore() error {
	if err := vs.store.Clear(); err != nil {
		return Error(err.Error())
	}
	return nil
}

// Shutdown does cleanup tasks.
func (vs *VerifyService) Shutdown() {
	if err := vs.mmReader.Close(); err != nil {
		vs.log.Warnw("Maxmind shutdown", "error", err)
	}
	vs.store.Shutdown()
}

// geoEventFromRequest prepares either the "previous" and "subsequent" part
// of the response item, given the data.  This is mostly to refactor common
// code.
func (vs *VerifyService) geoEventFromRequest(curLoc Location,
	curEvent, otherEvent *types.VerifyRequest) (*types.GeoEvent, error) {

	otherLoc, err := lookupIP(otherEvent.IPAddress, vs.mmReader, vs.log)
	if err != nil {
		return nil, err
	}

	speed := calculateSpeed(otherLoc.Latitude, otherLoc.Longitude,
		otherEvent.UnixTimestamp, curLoc.Latitude, curLoc.Longitude,
		curEvent.UnixTimestamp)

	// As documented in the readme, we use the special value -1 for the 0 time
	// situation (two events at exactly he same Unix time).
	var suspicious bool
	if speed == -1 || speed > types.MaxSpeed {
		suspicious = true
	}
	ge := types.GeoEvent{
		Speed:            speed,
		SuspiciousTravel: suspicious,
		IP:               otherEvent.IPAddress,
		Lat:              otherLoc.Latitude,
		Lon:              otherLoc.Longitude,
		Radius:           otherLoc.AccuracyRadius,
		Timestamp:        otherEvent.UnixTimestamp,
	}
	return &ge, nil
}

// calculateSpeed uses the two sets of coordinates and corresponding timestamps
// to calculte a rate that is rounded to the nearest integer (as per the sample
// in the assignment).
func calculateSpeed(lat1, lon1 float64, time1 int64, lat2, lon2 float64, time2 int64) int64 {
	dist := haversine(lat1, lon1, lat2, lon2)

	// We don't want to divide by 0, so we use -1 as an indicator for this.
	if time1 == time2 {
		return -1
	}
	t := math.Abs(float64(time2 - time1))

	// This calcuation is distance ((miles) * (sec/hr)) / sec = speed in miles/hr
	// It seems less prone to underflow than (dist/t) * 3600, given that the earth's
	// circumference is around 25 K miles, and a week of seconds is about the same number.
	return int64(math.Round((dist * 3600) / t))
}

// Source: // https://play.golang.org/p/MZVh5bRWqN - basic code similar to these
// packages, but conversion to miles is more precise:
// "github.com/paultag/go-haversine"
// "github.com/umahmood/haversine"
func haversine(latFrom, lonFrom, latTo, lonTo float64) float64 {
	var deltaLat = (latTo - latFrom) * (math.Pi / 180)
	var deltaLon = (lonTo - lonFrom) * (math.Pi / 180)

	var a = math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(latFrom*(math.Pi/180))*math.Cos(latTo*(math.Pi/180))*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	var c = 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return kmtomiles * (earthRadius * c)
}

// lookupIP does a MaxMind lookup, using the more efficient lower-level API.
func lookupIP(ip string, db *maxminddb.Reader, log *zap.SugaredLogger) (Location, error) {
	// Syntactic weirdness due to using recommended low-level API, which
	// requires a struct tag.
	var loc struct {
		Loc Location `maxminddb:"location"`
	}

	ipn := net.ParseIP(ip)
	if ipn == nil {
		log.Errorw("bad IP address not caught by validation", "IPaddr", ip)
		return loc.Loc, fmt.Errorf("invalid IP addr format: %s", ip)
	}
	err := db.Lookup(ipn, &loc)
	if err != nil {
		return loc.Loc, err
	}
	return loc.Loc, nil
}
