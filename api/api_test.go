package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gdotgordon/ipverify/types"
	"go.uber.org/zap"
)

var (
	req1 = types.VerifyRequest{
		Username:      "bob",
		UnixTimestamp: 1514850000,
		EventUUID:     "55ad929a-db03-4bf4-9541-8f728fa12e42",
		IPAddress:     "131.91.101.181",
	}

	validCurrGeo = types.CurrentGeoStat{
		Lat:    26.3796,
		Lon:    -80.1029,
		Radius: 5,
	}

	validGeoEvent = types.GeoEvent{
		IP:         "131.91.101.181",
		Speed:      0,
		Suspicious: false,
		Lat:        26.3796,
		Lon:        -80.1029,
		Radius:     5,
		Timestamp:  1514850000,
	}

	validGeoEvent2 = types.GeoEvent{
		IP:         "135.91.101.181",
		Speed:      0,
		Suspicious: false,
		Lat:        29.3796,
		Lon:        -80.1029,
		Radius:     10,
		Timestamp:  1514850000,
	}
)

func TestStatusEndpoint(t *testing.T) {
	api := apiImpl{log: newTestLogger(t)}
	req, err := http.NewRequest(http.MethodGet, statusURL, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Call the handler for status
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(api.getStatus)
	handler.ServeHTTP(rr, req)

	// Verify the code and expected body
	if status := rr.Code; status != http.StatusOK {
		t.Fatalf("handler returned wrong status code: got %d, expected %d",
			rr.Code, http.StatusOK)
	}
	expected := "{\n" + `  "status": "IP verify service is up and running"` + "\n}"
	body := rr.Body.String()
	if body != expected {
		t.Fatalf("unexpected body: %s, expected %s", body, expected)
	}
}

// TestVerify test that the HTTP hadndler function properly unmarshals
// the request, marshals a response, amd generates proper statsus codes.
// It uses a mock server so we can focus on the HTTP aspects.
func TestVerify(t *testing.T) {
	for i, v := range []struct {
		verifyReq    types.VerifyRequest // VerifyRequest object
		useName      string              // user name drives mock service respo
		badTimestamp bool                // put bad timestamp in request
		badUUID      bool                // put bad UUID in request
		badIPAddr    bool                // put bad IP Addr in request
		expStatus    int                 // expected HTTP return code
		expPrev      bool                // expected "previous" event
		expNext      bool                // expected "next" event
		expErrMsg    string              // error text
	}{
		{
			useName:   "NoPredOrSucc",
			verifyReq: req1,
			expStatus: http.StatusOK,
		},
		{
			useName:   "PredOnly",
			verifyReq: req1,
			expStatus: http.StatusOK,
			expPrev:   true,
		},
		{
			useName:   "SuccOnly",
			verifyReq: req1,
			expStatus: http.StatusOK,
			expNext:   true,
		},
		{
			useName:   "PredAndSucc",
			verifyReq: req1,
			expStatus: http.StatusOK,
			expPrev:   true,
			expNext:   true,
		},
		{
			useName:   "",
			verifyReq: req1,
			expStatus: http.StatusBadRequest,
			expErrMsg: "missing username",
		},
		{
			useName:   "bob",
			badUUID:   true,
			verifyReq: req1,
			expStatus: http.StatusBadRequest,
			expErrMsg: "invalid UUID length: 3",
		},
		{
			useName:   "bob",
			badIPAddr: true,
			verifyReq: req1,
			expStatus: http.StatusBadRequest,
			expErrMsg: "invalid IP address: 3.4",
		},
		{
			useName:      "bob",
			badTimestamp: true,
			verifyReq:    req1,
			expStatus:    http.StatusBadRequest,
			expErrMsg:    "invalid timestamp: -4",
		},
	} {
		ms := &mockService{}
		api := apiImpl{service: ms, log: newTestLogger(t)}
		rr := httptest.NewRecorder()
		handler := http.HandlerFunc(api.verifyIP)

		// Prep the request based on the chosen params.
		vreq := req1
		vreq.Username = v.useName
		if v.badUUID {
			vreq.EventUUID = "XXX"
		}
		if v.badIPAddr {
			vreq.IPAddress = "3.4"
		}
		if v.badTimestamp {
			vreq.UnixTimestamp = -4
		}
		b, err := json.MarshalIndent(vreq, "", "  ")
		if err != nil {
			t.Fatalf("(%d) cannot marshal json: %v", i, err)
		}
		rdr := bytes.NewReader(b)

		// Send the request.
		req, err := http.NewRequest(http.MethodPost, verifyURL, rdr)
		if err != nil {
			t.Fatal(err)
		}
		handler.ServeHTTP(rr, req)
		if status := rr.Code; status != v.expStatus {
			t.Fatalf("(%d) handler returned wrong status code: got %d, expected %d",
				i, rr.Code, v.expStatus)
		}

		// Proceed as per whether a successful return was expected or not
		if v.expStatus == http.StatusOK {
			var resp types.VerifyResponse
			err = json.Unmarshal(rr.Body.Bytes(), &resp)
			if err != nil {
				t.Fatal(err)
			}
			if resp.CurrentGeo != validCurrGeo {
				t.Errorf("(%d) got unexpecgted current geo: %+v", i, resp.CurrentGeo)
			}

			// Check that any missing or present previous and subsequent events
			// are what's expected.
			if !v.expPrev {
				if resp.PrecedingIPAccess != nil {
					t.Errorf("(%d) expected empty prev, got: %+v", i, *resp.PrecedingIPAccess)
				}
			} else {
				if resp.PrecedingIPAccess == nil {
					t.Errorf("(%d) expected non-empty prev", i)
				} else if *resp.PrecedingIPAccess != validGeoEvent {
					t.Errorf("(%d) expected prev %+v, got: %v", i, validGeoEvent,
						*resp.PrecedingIPAccess)
				}
			}

			if !v.expNext {
				if resp.SubsequentIPAccess != nil {
					t.Errorf("(%d) expected empty next, got: %+v", i,
						*resp.SubsequentIPAccess)
				}
			} else {
				if resp.SubsequentIPAccess == nil {
					t.Errorf("(%d) expected non-empty next", i)
				} else if *resp.SubsequentIPAccess != validGeoEvent2 {
					t.Errorf("(%d) expected next %+v, got: %v", i, validGeoEvent2,
						*resp.SubsequentIPAccess)
				}
			}
		} else {
			// Error cases - expect an error struct with a specific message
			status := types.StatusResponse{}
			if err = json.Unmarshal(rr.Body.Bytes(), &status); err != nil {
				t.Fatalf("(%d) can't unmarshal status: %v", i, err)
			}
			if status.Status != v.expErrMsg {
				t.Errorf("expected err message '%s', got '%s", v.expErrMsg, status.Status)
			}
		}
	}
}

func newTestLogger(t *testing.T) *zap.SugaredLogger {
	config := zap.NewProductionConfig()
	config.OutputPaths = []string{"/dev/null"}
	lg, _ := config.Build()
	return lg.Sugar()
}

// The mockService implements the service API but keys on the username of
// the request to determine the response type, for example, wehether the
// response incldues a previous and/or subsequent event.
type mockService struct {
}

func (ms *mockService) VerifyIP(req types.VerifyRequest) (*types.VerifyResponse, error) {
	var resp types.VerifyResponse

	switch req.Username {
	case "NoPredOrSucc":
		resp.CurrentGeo = validCurrGeo
		return &resp, nil
	case "PredOnly":
		resp.CurrentGeo = validCurrGeo
		resp.PrecedingIPAccess = &validGeoEvent
		return &resp, nil
	case "SuccOnly":
		resp.CurrentGeo = validCurrGeo
		resp.SubsequentIPAccess = &validGeoEvent2
		return &resp, nil
	case "PredAndSucc":
		resp.CurrentGeo = validCurrGeo
		resp.PrecedingIPAccess = &validGeoEvent
		resp.SubsequentIPAccess = &validGeoEvent2
		return &resp, nil
	default:
		return nil, nil
	}
}

func (ms *mockService) ResetStore() error {
	return nil
}
