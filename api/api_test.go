package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gdotgordon/ipverify/types"
	"go.uber.org/zap"
)

func TestStatusEndpoint(t *testing.T) {
	api := apiImpl{log: newLogger(t)}
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

func newLogger(t *testing.T) *zap.SugaredLogger {
	lg, err := zap.NewDevelopment()
	if err != nil {
		t.Fatalf("cannot create logger: %v", err)
	}
	return lg.Sugar()
}

type MockService struct {
}

func (ms *MockService) VerifyIP(req types.VerifyRequest) (types.VerifyResponse, error) {
	switch req.Username {
	case "Vernon":
		return types.VerifyResponse{}, errors.New("Vernon error")
	case "PredSucc":
		return types.VerifyResponse{
			Lat:               0.1,
			Lon:               0.2,
			Radius:            1,
			PrecedingIPAccess: &types.GeoEvent{},
		}, nil
	}
	return types.VerifyResponse{}, nil
}
