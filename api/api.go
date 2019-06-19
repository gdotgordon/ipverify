// Package api is the endpoint implementation for the produce service.
// The HTTP endpoint implmentations are here.  This package deals with
// unmarshaling and marshaling payloads, dispatching to the service (which is
// itself contains an instance of the store), processing those errors,
// and implementing proper REST semantics.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"

	"github.com/gdotgordon/ipverify/service"
	"github.com/gdotgordon/ipverify/types"
	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// Definitions for the supported URLs.
const (
	statusURL = "/v1/status"
	verifyURL = "/v1/verify"
	resetURL  = "/v1/reset"
)

// API is the item that dispatches to the endpoint implementations
type apiImpl struct {
	service service.Service
	log     *zap.SugaredLogger
}

// Init sets up the endpoint processing.  There is nothing returned, other
// than potntial errors, because the endpoint handling is configured in
// the passed-in muxer.
func Init(ctx context.Context, r *mux.Router, service service.Service, log *zap.SugaredLogger) error {
	ap := apiImpl{service: service, log: log}
	r.HandleFunc(statusURL, ap.getStatus).Methods(http.MethodGet)
	r.HandleFunc(verifyURL, ap.verify).Methods(http.MethodPost)
	//r.HandleFunc(resetURL, ap.handleReset).Methods(http.MethodPost)

	var wrapContext = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rc := r.WithContext(ctx)
			next.ServeHTTP(w, rc)
		})
	}

	var loggingMiddleware = func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Infow("Handling URL", "url", r.URL)
			next.ServeHTTP(w, r)
		})
	}
	r.Use(loggingMiddleware)
	r.Use(wrapContext)
	return nil
}

// Liveness check endpoint
func (a apiImpl) getStatus(w http.ResponseWriter, r *http.Request) {
	if r.Body != nil {
		defer r.Body.Close()
	}

	sr := types.StatusResponse{Status: "IP verify service is up and running"}
	b, err := json.MarshalIndent(sr, "", "  ")
	if err != nil {
		a.writeErrorResponse(w, http.StatusInternalServerError, err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

// Verify a potential suspicious IP address
func (a apiImpl) verify(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		a.writeErrorResponse(w, http.StatusBadRequest, errors.New("No body for POST"))
		return
	}
	defer r.Body.Close()

	var request types.VerifyRequest
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil {
		a.writeErrorResponse(w, http.StatusBadRequest, err)
	}
	if err := validateVerifyRequest(request); err != nil {
		a.writeErrorResponse(w, http.StatusBadRequest, err)
	}

	response, err := a.service.VerifyIP(request)
	if err != nil {
		a.writeErrorResponse(w, http.StatusBadRequest, err)
	}

	b, err := json.MarshalIndent(response, "", "  ")
	if err != nil {
		a.writeErrorResponse(w, http.StatusInternalServerError, err)
	}
	_, err = w.Write(b)
	if err != nil {
		a.writeErrorResponse(w, http.StatusInternalServerError, err)
	}
}

func validateVerifyRequest(request types.VerifyRequest) error {
	if request.Username == "" {
		return errors.New("missing username")
	}
	if request.UnixTimestamp == 0 {
		return errors.New("zero timestamp")
	}
	if _, err := uuid.Parse(request.EventUUID); err != nil {
		return err
	}
	if net.ParseIP(request.IPAddress) == nil {
		return errors.New("invalid IP address")
	}
	return nil
}

// For HTTP bad request repsonses, serialize a JSON status message with
// the cause.
func (a apiImpl) writeErrorResponse(w http.ResponseWriter, code int, err error) {
	a.log.Errorw("invoke error", "error", err, "code", code)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(code)
	b, _ := json.MarshalIndent(types.StatusResponse{Status: err.Error()}, "", "  ")
	w.Write(b)
}

// Map a Go eror to an HTTP status type
func errorToStatusCode(err error, nilCode int) int {
	switch err.(type) {
	/*
		case service.InternalError:
			return http.StatusInternalServerError
		case service.FormatError:
			return http.StatusBadRequest
		case store.AlreadyExistsError:
			return http.StatusConflict
		case store.NotFoundError:
			return http.StatusNotFound
	*/
	case nil:
		return nilCode
	default:
		return http.StatusInternalServerError
	}
}
