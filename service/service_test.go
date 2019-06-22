package service

import (
	"fmt"
	"reflect"
	"testing"
	"time"

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
	defer db.Close()

	for i, v := range []struct {
		ipAddr    string
		expLat    float64
		expLong   float64
		expRadius uint16
		expErr    bool
		expErrStr string
	}{
		{
			ipAddr:    "128.148.252.151",
			expLat:    41.8244,
			expLong:   -71.408,
			expRadius: 5,
		},
		{
			ipAddr:    "131.91.101.181",
			expLat:    26.3796,
			expLong:   -80.1029,
			expRadius: 5,
		},
		{
			ipAddr:    "131.91.101",
			expErr:    true,
			expErrStr: "Invalid IP addr format: 131.91.101",
		},
	} {
		loc, err := lookupIP(v.ipAddr, db)
		if err != nil {
			if !v.expErr {
				t.Errorf("(%d) expected no error, got %v", i, err)
			} else if err.Error() != v.expErrStr {
				t.Errorf("(%d) expected error '%t', got '%s'", i, v.expErr, err)
			}
		} else if loc.Latitude != v.expLat {
			t.Errorf("(%d) expected latitude %f, got %f", i, v.expLat, loc.Latitude)
		}
		if loc.Longitude != v.expLong {
			t.Errorf("(%d) expected longitude %f, got %f", i, v.expLong, loc.Longitude)
		}
		if loc.AccuracyRadius != v.expRadius {
			t.Errorf("(%d) expected radius %d, got %d", i, v.expRadius, loc.AccuracyRadius)
		}
	}
}

func TestSpeed(t *testing.T) {
	for i, v := range []struct {
		lat1     float64
		long1    float64
		lat2     float64
		long2    float64
		t1       int64
		t2       int64
		expSpeed int64
	}{
		{
			lat1:     41.8244,
			long1:    -71.408,
			lat2:     26.3796,
			long2:    -80.1029,
			t1:       1514851200,
			t2:       1514858400,
			expSpeed: 588,
		},
		{
			lat1:     51.45,
			long1:    -1.15,
			lat2:     45.04,
			long2:    7.42,
			t1:       1514858400,
			t2:       1514851200,
			expSpeed: 296,
		},
		{
			lat1:     45.04,
			long1:    7.42,
			lat2:     51.45,
			long2:    -1.15,
			t1:       1514851200,
			t2:       1514858400,
			expSpeed: 296,
		},
	} {
		res := calculateSpeed(v.lat1, v.long1, v.t1, v.lat2, v.long2, v.t2)
		if res != v.expSpeed {
			t.Errorf("(%d) Expected speed %d, got %d", i, v.expSpeed, res)
		}
	}
}

type coords struct {
	lat float64
	lon float64
}

func TestVerify(t *testing.T) {
	// For the test data, we incude some known locations from running "dig",
	// so we can judge the accuracy of the results.
	// "128.148.252.151": brown.edu - Brown University, Providence, RI
	// "131.91.101.181": fau.edu - Florida Atlantic U, Boca Ration FL
	// Air distance between the above two is approx 1117 mi., so 2 days
	// @500 MPH is fine, but 3 days is suspicious:
	// https://www.distance-cities.com/distance-providence-ri-to-boca-raton-fl
	// "128.97.27.37": ucla.edu - UCLA, Los Angeles CA
	// distance LA->Providence: 2,575.81 mi
	// https://www.distance.to/Providence/Los-Angeles
	// Distance LA-> Boca Raton: 2,326.44 mi
	//Distsance Boca Raton -> Little Rock, AR 929 Miles -> 2 hours will be suspicious
	// NOTE: these distances are close to the actual distance for where these
	// Universities are, as they are genenric city-to-city-distances.
	const (
		BrownAddr    = "128.148.252.151"
		FAUAddr      = "131.91.101.181"
		UCLAAddr     = "128.97.27.37"
		ArkansasAddr = "130.184.5.181"
	)

	BrownCoords := coords{41.8244, -71.408}
	FAUCoords := coords{26.3796, -80.1029}
	UCLACoords := coords{34.0648, -118.4414}
	ArkansasCoords := coords{36.0557, -94.1567}

	now := time.Now().Unix()
	l := newTestLogger(t)
	store, err := NewSQLiteStore(":memory:", l)
	if err != nil {
		t.Errorf("error creating store: %v", err)
	}
	srv, err := New("../mmdb/GeoLite2-City.mmdb", store, l)
	if err != nil {
		t.Errorf("error creating service: %v", err)
	}
	defer srv.Shutdown()

	for _, v := range []struct {
		description string                // Test description
		seed        []types.VerifyRequest // Rows to seed in the datadase
		payload     types.VerifyRequest   // Payload to send
		expCurr     types.CurrentGeoStat  // Data from current request returned
		expPrev     *types.GeoEvent       // Previous event returned
		expSucc     *types.GeoEvent       // Subsequent event returned
		expErrMsg   string                // Expected error message
	}{
		{
			description: "To empty DB",
			payload:     makeReq("Bob", BrownAddr, now),
			expCurr:     makeCurrGeo(BrownCoords, 5),
		},
		{
			description: "Predecessor for user, valid distance",
			seed:        []types.VerifyRequest{makeReq("Bob", FAUAddr, ago(72*time.Hour, now))},
			payload:     makeReq("Bob", BrownAddr, now),
			expCurr:     makeCurrGeo(BrownCoords, 5),
			expPrev:     makeGeoEvent(FAUAddr, 16, false, FAUCoords, 5, ago(72*time.Hour, now)),
		},
		{
			description: "Predecessor for user, invalid distance",
			seed:        []types.VerifyRequest{makeReq("Bob", FAUAddr, ago(time.Hour, now))},
			payload:     makeReq("Bob", BrownAddr, now),
			expCurr:     makeCurrGeo(BrownCoords, 5),
			expPrev:     makeGeoEvent(FAUAddr, 1176, true, FAUCoords, 5, ago(time.Hour, now)),
		},
		{
			description: "Predecessor for user, 0 distance",
			seed:        []types.VerifyRequest{makeReq("Jane", FAUAddr, ago(time.Hour, now))},
			payload:     makeReq("Jane", FAUAddr, now),
			expCurr:     makeCurrGeo(FAUCoords, 5),
			expPrev:     makeGeoEvent(FAUAddr, 0, false, FAUCoords, 5, ago(time.Hour, now)),
		},
		{
			description: "Successor for user, valid distance",
			seed:        []types.VerifyRequest{makeReq("Bob", FAUAddr, now)},
			payload:     makeReq("Bob", BrownAddr, ago(72*time.Hour, now)),
			expCurr:     makeCurrGeo(BrownCoords, 5),
			expSucc:     makeGeoEvent(FAUAddr, 16, false, FAUCoords, 5, now),
		},
		{
			description: "Successor for user, invalid distance",
			seed:        []types.VerifyRequest{makeReq("Bob", FAUAddr, now)},
			payload:     makeReq("Bob", BrownAddr, ago(time.Hour, now)),
			expCurr:     makeCurrGeo(BrownCoords, 5),
			expSucc:     makeGeoEvent(FAUAddr, 1176, true, FAUCoords, 5, now),
		},
		{
			description: "DB with no other record for user",
			seed: []types.VerifyRequest{
				makeReq("Steve", UCLAAddr, now),
				makeReq("Roger", UCLAAddr, ago(3*time.Hour, now)),
			},
			payload: makeReq("Bob", BrownAddr, ago(2*time.Hour, now)),
			expCurr: makeCurrGeo(BrownCoords, 5),
		},
		{
			description: "Predecessor for user, valid distance with other users",
			seed: []types.VerifyRequest{
				makeReq("Annie", ArkansasAddr, ago(100*time.Hour, now)),
				makeReq("Bob", UCLAAddr, ago(72*time.Hour, now)), // *** Should choose this one
				makeReq("Bob", FAUAddr, ago(144*time.Hour, now)),
				makeReq("Joanne", ArkansasAddr, ago(96*time.Hour, now)),
				makeReq("Joanne", BrownAddr, ago(32*time.Hour, now)),
			},
			payload: makeReq("Bob", BrownAddr, now),
			expCurr: makeCurrGeo(BrownCoords, 5),
			expPrev: makeGeoEvent(UCLAAddr, 36, false, UCLACoords, 10, ago(72*time.Hour, now)),
		},
		{
			description: "Successor for user, invalid distance including other users",
			seed: []types.VerifyRequest{
				makeReq("Annie", ArkansasAddr, ago(100*time.Hour, now)),
				makeReq("Bob", UCLAAddr, ago(72*time.Hour, now)),
				makeReq("Bob", ArkansasAddr, ago(142*time.Hour, now)), // *** Should choose this one
				makeReq("Joanne", ArkansasAddr, ago(96*time.Hour, now)),
				makeReq("Joanne", BrownAddr, ago(32*time.Hour, now)),
			},
			payload: makeReq("Bob", FAUAddr, ago(144*time.Hour, now)),
			expCurr: makeCurrGeo(FAUCoords, 5),
			expSucc: makeGeoEvent(ArkansasAddr, 532, true, ArkansasCoords, 5, ago(142*time.Hour, now)),
		},
		{
			description: "Predecessor valid distance, successor invalid distance, including other users",
			seed: []types.VerifyRequest{
				makeReq("Annie", ArkansasAddr, ago(100*time.Hour, now)),
				makeReq("Angie", BrownAddr, ago(200*time.Hour, now)), // *** Should choose this as predecessor
				makeReq("Angie", UCLAAddr, ago(72*time.Hour, now)),
				makeReq("Angie", UCLAAddr, ago(148*time.Hour, now)), // *** Should choose this as successor
				makeReq("Joanne", ArkansasAddr, ago(96*time.Hour, now)),
				makeReq("Joanne", BrownAddr, ago(32*time.Hour, now)),
			},
			payload: makeReq("Angie", ArkansasAddr, ago(150*time.Hour, now)),
			expCurr: makeCurrGeo(ArkansasCoords, 5),
			expPrev: makeGeoEvent(BrownAddr, 26, false, BrownCoords, 5, ago(200*time.Hour, now)),
			expSucc: makeGeoEvent(UCLAAddr, 688, true, UCLACoords, 10, ago(148*time.Hour, now)),
		},
	} {
		if err := srv.ResetStore(); err != nil {
			t.Fatalf("'%s': error resetting DB: %v", v.description, err)
		}

		x, _ := store.GetAllRows()
		fmt.Printf("All rows: %v\n", x)
		for _, r := range v.seed {
			if err := srv.store.AddRecord(r); err != nil {
				t.Errorf("'%s': error ", v.description)
			}
		}
		resp, err := srv.VerifyIP(v.payload)
		if v.expErrMsg != "" {
			if err == nil {
				t.Errorf("'%s': expected error '%s', but got no error", v.description,
					v.expErrMsg)
			} else if err.Error() != v.expErrMsg {
				t.Errorf("'%s': expected error string '%s', got '%s", v.description,
					v.expErrMsg, err.Error())
			}
		} else if err != nil {
			t.Errorf("'%s' got unexpected error '%v'", v.description, err)
		}

		var expResp types.VerifyResponse
		expResp.CurrentGeo = v.expCurr
		expResp.PrecedingIPAccess = v.expPrev
		expResp.SubsequentIPAccess = v.expSucc
		if !(reflect.DeepEqual(*resp, expResp)) {
			t.Errorf("'%s': Expected response: %v, got: %v", v.description, expResp, resp)
		}
	}
}

func makeReq(username, ipaddr string, timestamp int64) types.VerifyRequest {
	return types.VerifyRequest{
		Username:      username,
		UnixTimestamp: timestamp,
		EventUUID:     uuid.New().String(),
		IPAddress:     ipaddr,
	}
}

func makeCurrGeo(c coords, radius uint16) types.CurrentGeoStat {
	return types.CurrentGeoStat{
		Lat:    c.lat,
		Lon:    c.lon,
		Radius: radius,
	}
}

func makeGeoEvent(ip string, speed int64, suspicious bool,
	c coords, radius uint16, timestamp int64) *types.GeoEvent {
	return &types.GeoEvent{
		IP:         ip,
		Speed:      speed,
		Suspicious: suspicious,
		Lat:        c.lat,
		Lon:        c.lon,
		Radius:     radius,
		Timestamp:  timestamp,
	}
}

func ago(d time.Duration, now int64) int64 {
	return time.Unix(now, 0).Add(-1 * d).Unix()
}

func TestOVerify(t *testing.T) {
	l := newTestLogger(t)
	store, err := NewSQLiteStore(":memory:", l)
	if err != nil {
		t.Errorf("error creating store: %v", err)
	}
	srv, err := New("../mmdb/GeoLite2-City.mmdb", store, l)
	if err != nil {
		t.Errorf("error creating service: %v", err)
	}
	rec2 := types.VerifyRequest{
		Username:      "bob",
		UnixTimestamp: 1514859999,
		EventUUID:     "85ad929a-db03-4bf4-9541-8f728fa12e42",
		IPAddress:     "128.148.252.151",
	}
	rec1 := types.VerifyRequest{
		Username:      "bob",
		UnixTimestamp: 1514850000,
		EventUUID:     "55ad929a-db03-4bf4-9541-8f728fa12e42",
		IPAddress:     "131.91.101.181",
	}
	rec3 := types.VerifyRequest{
		Username:      "bob",
		UnixTimestamp: 1514866666,
		EventUUID:     "65ad929a-db03-4bf4-9541-8f728fa12e42",
		IPAddress:     "131.91.101.181",
	}
	rec4 := types.VerifyRequest{
		Username:      "bob",
		UnixTimestamp: 1514866667,
		EventUUID:     "25ad929a-db03-4bf4-9541-8f728fa12e42",
		IPAddress:     "128.148.252.151",
	}
	rec5 := types.VerifyRequest{
		Username:      "bob",
		UnixTimestamp: 1514850300,
		EventUUID:     "35ad929a-db03-4bf4-9541-8f728fa12e42",
		IPAddress:     "131.91.101.181",
	}
	res, err := srv.VerifyIP(rec1)
	fmt.Println("res:", res, "err:", err)
	res, err = srv.VerifyIP(rec2)
	fmt.Printf("res: %+v, err: %v\n", res, err)
	res, err = srv.VerifyIP(rec3)
	fmt.Printf("res: %+v, err: %v\n", res, err)
	res, err = srv.VerifyIP(rec4)
	fmt.Printf("res: %+v, err: %v\n", res, err)
	res, err = srv.VerifyIP(rec5)
	fmt.Printf("res: %+v, err: %v\n", res, err)
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
