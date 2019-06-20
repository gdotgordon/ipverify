package service

import (
	"fmt"
	"testing"

	"github.com/gdotgordon/ipverify/types"
	"github.com/google/uuid"
	"github.com/oschwald/maxminddb-golang"
	"go.uber.org/zap"
)

func TestMaxMind(t *testing.T) {
	db, err := maxminddb.Open("../mmdb/GeoLite2-City.mmdb")
	if err != nil {
		t.Error(err)
	}
	loc, err := lookupIP("128.148.252.151", db) // Brown
	fmt.Println(loc)
	loc, err = lookupIP("131.91.101.181", db) // FAU
	fmt.Println(loc)
}

func TestSpeed(t *testing.T) {
	fmt.Println(calculateSpeed(41.8244, -71.408, 1514851200, 26.3796, -80.1029, 1514858400))
	fmt.Println(calculateSpeed(51.45, -1.15, 1514851200, 45.04, 7.42, 1514858400))
}

func makeRecord(un string, ts int64, ip string) types.VerifyRequest {
	return types.VerifyRequest{
		Username:      un,
		UnixTimestamp: ts,
		EventUUID:     uuid.New().String(),
		IPAddress:     ip,
	}
}

func newTestLogger(t *testing.T) *zap.SugaredLogger {
	lg, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("cannot create logger: %v", err)
	}
	return lg.Sugar()
}
