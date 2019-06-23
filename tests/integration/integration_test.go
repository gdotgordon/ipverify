// +build integration

// Run as: go test -tags=integration
package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/gdotgordon/ipverify/types"
	"github.com/google/uuid"
)

const (
	BrownAddr    = "128.148.252.151"
	FAUAddr      = "131.91.101.181"
	UCLAAddr     = "128.97.27.37"
	ArkansasAddr = "130.184.5.181"
)

var (
	verifyAddr   string
	verifyClient *http.Client

	BrownCoords    = coords{41.8244, -71.408}
	FAUCoords      = coords{26.3796, -80.1029}
	UCLACoords     = coords{34.0648, -118.4414}
	ArkansasCoords = coords{36.0557, -94.1567}
)

type coords struct {
	lat float64
	lon float64
}

func TestMain(m *testing.M) {
	// call flag.Parse() here if TestMain uses flags
	verifyAddr, _ = getAppAddr("8080", "ipverify_ipverify_1", "ipverify", "ipverify_ipverify")
	verifyClient = http.DefaultClient
	os.Exit(m.Run())
}

func TestStatus(t *testing.T) {
	resp, err := http.Get("http://" + verifyAddr + "/v1/status")
	if err != nil {
		t.Fatalf("status failed: %s", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Unexpected return code: %d", resp.StatusCode)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		t.Fatal("error reading response body", err)
	}
	var statResp map[string]string
	if err := json.Unmarshal(b, &statResp); err != nil {
		t.Fatal("error deserializing JSON", err)
	}
	if statResp["status"] != "IP verify service is up and running" {
		t.Fatal("unexpected status repsonse", statResp["status"])
	}
}

// What I've done here is "port" the unit test from service, so instead of directly
// calling the service API, we are going end-to-end over HTTP.  But since there is
// no API to simply add a user to the DB, we are calling the verify() AP{I, but ignoring
// the repsonse.  So the effect is to add the record to the DB and then everything is in
// place for the verify call where we check the specifics.
func TestVerify(t *testing.T) {
	now := time.Now().Unix()

	for _, v := range []struct {
		description string                // Test description
		seed        []types.VerifyRequest // Rows to seed in the datadase
		payload     types.VerifyRequest   // Payload to send
		expCurr     types.CurrentGeoStat  // Data from current request returned
		expPrev     *types.GeoEvent       // Previous event returned
		expSucc     *types.GeoEvent       // Subsequent event returned
		expCode     int                   // HTTP response code
		expErrMsg   string                // Force an error to happen
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
			description: "Predecessor for user, faulty request error",
			seed: []types.VerifyRequest{
				{
					Username:      "Bob",
					UnixTimestamp: ago(72*time.Hour, now),
					EventUUID:     "4b1971d6-da85-467f-b52e-528eb71b13f1",
					IPAddress:     BrownAddr,
				}},
			payload: types.VerifyRequest{
				Username:      "Bob",
				UnixTimestamp: now,
				EventUUID:     "4b1971d6-da85-467f-b52e-528eb71b13f1",
				IPAddress:     BrownAddr,
			},
			expCode:   400,
			expErrMsg: "add record to store: UNIQUE constraint failed: items.Uuid",
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
		{
			description: "Predecessor valid distance, successor valid distance, including other users",
			seed: []types.VerifyRequest{
				makeReq("Annie", ArkansasAddr, ago(100*time.Hour, now)),
				makeReq("Angie", ArkansasAddr, ago(200*time.Hour, now)), // *** Should choose this as predecessor
				makeReq("Angie", UCLAAddr, ago(72*time.Hour, now)),
				makeReq("Angie", UCLAAddr, ago(146*time.Hour, now)), // *** Should choose this as successor
				makeReq("Joanne", ArkansasAddr, ago(96*time.Hour, now)),
				makeReq("Joanne", BrownAddr, ago(32*time.Hour, now)),
			},
			payload: makeReq("Angie", ArkansasAddr, ago(150*time.Hour, now)),
			expCurr: makeCurrGeo(ArkansasCoords, 5),
			expPrev: makeGeoEvent(ArkansasAddr, 0, false, ArkansasCoords, 5, ago(200*time.Hour, now)),
			expSucc: makeGeoEvent(UCLAAddr, 344, false, UCLACoords, 10, ago(146*time.Hour, now)),
		},
		{
			description: "Predecessor invalid distance, successor valid distance, including other users",
			seed: []types.VerifyRequest{
				makeReq("Annie", ArkansasAddr, ago(100*time.Hour, now)),
				makeReq("Angie", ArkansasAddr, ago(300*time.Hour, now)),
				makeReq("Angie", BrownAddr, ago(151*time.Hour, now)), // *** Should choose this as predecessor
				makeReq("Angie", UCLAAddr, ago(72*time.Hour, now)),
				makeReq("Angie", UCLAAddr, ago(146*time.Hour, now)), // *** Should choose this as successor
				makeReq("Joanne", ArkansasAddr, ago(96*time.Hour, now)),
				makeReq("Joanne", BrownAddr, ago(32*time.Hour, now)),
			},
			payload: makeReq("Angie", ArkansasAddr, ago(150*time.Hour, now)),
			expCurr: makeCurrGeo(ArkansasCoords, 5),
			expPrev: makeGeoEvent(BrownAddr, 1281, true, BrownCoords, 5, ago(151*time.Hour, now)),
			expSucc: makeGeoEvent(UCLAAddr, 344, false, UCLACoords, 10, ago(146*time.Hour, now)),
		},
		{
			description: "Record with equal timestamp should be predecessor",
			seed: []types.VerifyRequest{
				makeReq("Ralph", UCLAAddr, ago(150*time.Hour, now)),
				makeReq("Annie", ArkansasAddr, ago(100*time.Hour, now)),
				makeReq("Angie", BrownAddr, ago(150*time.Hour, now)), // *** Should choose this (dup time) as predecessor
				makeReq("Angie", UCLAAddr, ago(72*time.Hour, now)),
				makeReq("Angie", UCLAAddr, ago(148*time.Hour, now)), // *** Should choose this as successor
				makeReq("Joanne", ArkansasAddr, ago(96*time.Hour, now)),
				makeReq("Joanne", BrownAddr, ago(32*time.Hour, now)),
			},
			payload: makeReq("Angie", ArkansasAddr, ago(150*time.Hour, now)),
			expCurr: makeCurrGeo(ArkansasCoords, 5),
			expPrev: makeGeoEvent(BrownAddr, -1, true, BrownCoords, 5, ago(150*time.Hour, now)),
			expSucc: makeGeoEvent(UCLAAddr, 688, true, UCLACoords, 10, ago(148*time.Hour, now)),
		},
		{
			description: "Matching timestamp, but different user",
			seed: []types.VerifyRequest{
				makeReq("Ralph", UCLAAddr, ago(150*time.Hour, now)),
				makeReq("Annie", ArkansasAddr, ago(100*time.Hour, now)),
				makeReq("Angie", UCLAAddr, ago(72*time.Hour, now)),
				makeReq("Angie", UCLAAddr, ago(148*time.Hour, now)), // *** Should choose this as successor
				makeReq("Joanne", ArkansasAddr, ago(96*time.Hour, now)),
				makeReq("Joanne", BrownAddr, ago(32*time.Hour, now)),
			},
			payload: makeReq("Angie", ArkansasAddr, ago(150*time.Hour, now)),
			expCurr: makeCurrGeo(ArkansasCoords, 5),
			expSucc: makeGeoEvent(UCLAAddr, 688, true, UCLACoords, 10, ago(148*time.Hour, now)),
		},
	} {
		invokeReset(t)
		for _, r := range v.seed {
			_, _, err := invokeVerify(r)
			if err != nil {
				t.Errorf("'%s': error ", v.description)
			}
		}
		resp, code, err := invokeVerify(v.payload)
		if v.expErrMsg != "" {
			if v.expCode != 0 && code != v.expCode {
				t.Errorf("expecting code %d, got %d\n", v.expCode, code)
			}
			if err == nil {
				t.Errorf("'%s': expected error '%s', but got no error", v.description,
					v.expErrMsg)
			} else if err.Error() != v.expErrMsg {
				t.Errorf("'%s': expected error string '%s', got '%s'", v.description,
					v.expErrMsg, err.Error())
			}
			continue
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

// The purpose of this test is simply to make sure we can submit requests simultaeously
// and not have things blow up.  Becuase the timing is unpredictable, it is difficult
// to analyze specific results.
func TestConcurrency(t *testing.T) {
	userCount := 20
	users := make([]string, userCount)
	for i := 0; i < userCount; i++ {
		users[i] = makeName()
	}

	rand.Seed(time.Now().Unix())
	invokeReset(t)

	now := time.Now().Unix()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < 10; j++ {
				req := makeReq(users[rand.Intn(20)], "128.148.252.151", now-int64(rand.Intn(3600*5)))
				_, code, err := invokeVerify(req)
				if err != nil || code != http.StatusOK {
					t.Errorf("failed: code: %d, err; %v", code, err)
				}
			}
		}()
	}
	wg.Wait()
}

func getAppAddr(port string, app ...string) (string, error) {
	var err error
	var res []byte
	for _, a := range app {
		res, err = exec.Command("docker", "port", a, port).CombinedOutput()
		if err == nil {
			break
		}
	}

	if err != nil {
		log.Fatalf("docker-compose error: failed to get exposed port: %v", err)
	}
	return string(res[:len(res)-1]), nil
}

func makeName() string {
	var buf bytes.Buffer
	buf.WriteByte("ABCDEFGHIJKLMNOPQRSTUVWXYZ"[rand.Int()%26])
	len := rand.Int()%10 + 2
	for i := 0; i < len; i++ {
		buf.WriteByte("abcdefghijklmnopqrstuvwxyz"[rand.Int()%26])
	}
	return buf.String()
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

// Invoke the verify API.
func invokeVerify(request types.VerifyRequest) (*types.VerifyResponse, int, error) {
	b, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequest(http.MethodPost, "http://"+verifyAddr+"/v1/verify",
		bytes.NewReader(b))
	if err != nil {
		return nil, 0, err
	}

	resp, err := verifyClient.Do(req)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		var status types.StatusResponse
		decoder := json.NewDecoder(resp.Body)
		if err := decoder.Decode(&status); err != nil {
			return nil, resp.StatusCode, err
		}
		return nil, resp.StatusCode, errors.New(status.Status)
	}

	var vresp types.VerifyResponse
	decoder := json.NewDecoder(resp.Body)
	if err := decoder.Decode(&vresp); err != nil {
		return nil, resp.StatusCode, err
	}

	return &vresp, resp.StatusCode, nil
}

func invokeReset(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://"+verifyAddr+"/v1/reset", nil)
	if err != nil {
		t.Fatal("reset error creating request", err)
	}

	resp, err := verifyClient.Do(req)
	if err != nil {
		t.Fatal("reset returned unexpcted error", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatal("Bad status code resetting db", resp.StatusCode)
	}
}
