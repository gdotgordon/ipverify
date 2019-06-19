// Package service implements the functionality of the IP Verify service.  It
// is the intermediary between the api package, which handles HTTP specifcs
// JSON marshaling, and the store pacakge, which is the data store.
package service

import (
	"fmt"
	"math"
	"net"

	"github.com/gdotgordon/ipverify/types"
	"github.com/oschwald/maxminddb-golang"
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

// Service defines the sets of functions handled by IP verify service
type Service interface {
	VerifyIP(types.VerifyRequest) (types.VerifyResponse, error)
}

type VerifyService struct {
	mmReader *maxminddb.Reader
	store    Store
	log      *zap.SugaredLogger
}

func New(log *zap.SugaredLogger) (*VerifyService, error) {
	mmReader, err := maxminddb.Open("./mmdb/GeoLite2-City.mmdb")
	if err != nil {
		return nil, err
	}
	store, err := NewSQLiteStore("requests.db", log)
	if err != nil {
		return nil, err
	}
	return &VerifyService{mmReader: mmReader, store: store, log: log}, nil
}

func (vs *VerifyService) VerifyIP(types.VerifyRequest) (types.VerifyResponse, error) {
	return types.VerifyResponse{}, nil
}

func (vs *VerifyService) Shutdown() {
	if err := vs.mmReader.Close(); err != nil {
		vs.log.Warnw("Maxmind shutdown", "error", err)
	}
	vs.store.Shutdown()
}

func speed(lon1, lat1 float64, time1 int64, lon2, lat2 float64, time2 int64) int64 {
	dist := haversine(lon1, lat1, lon2, lat2)
	fmt.Printf("distance: %f\n", dist)
	t := math.Abs(float64(time2 - time1))
	return int64(math.Round((dist / t) * 3600))
}

// Source: // https://play.golang.org/p/MZVh5bRWqN - bsaic code similar to these
// packages, but conversion to miles is more precise:
// "github.com/paultag/go-haversine"
// "github.com/umahmood/haversine"
func haversine(lonFrom float64, latFrom float64, lonTo float64, latTo float64) float64 {
	var deltaLat = (latTo - latFrom) * (math.Pi / 180)
	var deltaLon = (lonTo - lonFrom) * (math.Pi / 180)

	var a = math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(latFrom*(math.Pi/180))*math.Cos(latTo*(math.Pi/180))*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	var c = 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return kmtomiles * (earthRadius * c)
}

// Do a MaxMind lookup.
func lookupIP(ip string, db *maxminddb.Reader) (Location, error) {
	// Syntactic weirdness due to using recommended low-level API, which
	// requires a struct tag.
	var loc struct {
		Loc Location `maxminddb:"location"`
	}

	ipn := net.ParseIP(ip)
	if ipn == nil {
		return loc.Loc, fmt.Errorf("Invalid IP addr format: %s", ip)
	}
	err := db.Lookup(ipn, &loc)
	if err != nil {
		return loc.Loc, err
	}
	return loc.Loc, nil
}
