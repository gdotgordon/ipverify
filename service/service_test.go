package service

import (
	"fmt"
	"testing"

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
	fmt.Println(speed(-71.408, 41.8244, 1514851200, -80.1029, 26.3796, 1514858400))
	fmt.Println(speed(-1.15, 51.45, 1514851200, 7.42, 45.04, 1514858400))
}

func newTestLogger(t *testing.T) *zap.SugaredLogger {
	lg, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("cannot create logger: %v", err)
	}
	return lg.Sugar()
}
