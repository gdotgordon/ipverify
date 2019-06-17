// Package api is the endpoint implementation for the produce service.
// The HTTP endpoint implmentations are here.  This package deals with
// unmarshaling and marshaling payloads, dispatching to the service (which is
// itself contains an instance of the store), processing those errors,
// and implementing proper REST semantics.
package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gdotgordon/ipverify/types"
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
	log *zap.SugaredLogger
}

// Init sets up the endpoint processing.  There is nothing returned, other
// than potntial errors, because the endpoint handling is configured in
// the passed-in muxer.
func Init(ctx context.Context, r *mux.Router, log *zap.SugaredLogger) error {
	ap := apiImpl{log: log}
	r.HandleFunc(statusURL, ap.getStatus).Methods(http.MethodGet)
	//r.HandleFunc(verifyURL, ap.verify).Methods(http.MethodPost)
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
		a.notifyInternalServerError(w, "json marshal failed", err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	w.Write(b)
}

func (a apiImpl) notifyInternalServerError(w http.ResponseWriter, msg string,
	err error) {
	a.log.Errorw(msg, "error", err)
	w.WriteHeader(http.StatusInternalServerError)
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

// For HTTP bad request repsonses, serialize a JSON status message with
// the cause.
func writeBadRequestResponse(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusBadRequest)
	b, _ := json.MarshalIndent(types.StatusResponse{Status: err.Error()}, "", "  ")
	w.Write(b)
}
