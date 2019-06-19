package service

import (
	"fmt"
	"testing"

	"github.com/oschwald/maxminddb-golang"
)

func TestMaxMind(t *testing.T) {
	db, err := maxminddb.Open("../mmdb/GeoLite2-City.mmdb")
	if err != nil {
		t.Error(err)
	}
	loc, err := lookupIP("128.148.252.151", db) // Brown
	fmt.Println(loc)
}
