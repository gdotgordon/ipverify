// Package main runs the IP verify microservice.  It spins up an HTTP
// server to handle requests, which are handled by the api package.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gdotgordon/ipverify/api"
	"github.com/gdotgordon/ipverify/service"
	"github.com/gdotgordon/ipverify/store"
	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

type cleanupTask func()

var (
	portNum         int    // listen port
	logLevel        string // zap log level
	timeout         int    // server timeout in seconds
	maxMindFilepath string // location of Maxmind db file
	dbFilePath      string // location of SQLite3 db
)

func init() {
	flag.IntVar(&portNum, "port", 8080, "HTTP port number")
	flag.StringVar(&logLevel, "log", "production",
		"log level: 'production', 'development'")
	flag.IntVar(&timeout, "timeout", 30, "server timeout (seconds)")
	flag.StringVar(&maxMindFilepath, "mmdb", "mmdb/GeoLite2-City.mmdb",
		"location of MaxMind DB file")
	flag.StringVar(&dbFilePath, "db", "./db/requests.db",
		"location of SQLite DB file")
}

func main() {
	flag.Parse()

	// We'll propagate the context with cancel thorughout the program,
	// to be used by various entities, such as http clients, server
	// methods we implement, and other loops using channels.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up logging.
	log, err := initLogging()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating logger: %v", err)
		os.Exit(1)
	}

	// Create the server to handle the IP verify service.  The API module will
	// set up the routes, as we don't need to know the details in the
	// main program.
	muxer := mux.NewRouter()

	// Initialize the store.
	store, err := store.NewSQLiteStore(dbFilePath, log)
	if err != nil {
		log.Errorw("Error initializing service", "error", err)
		os.Exit(1)
	}

	// Build the service, passing it the maxmind path and the store.
	service, err := service.New(maxMindFilepath, store, log)
	if err != nil {
		log.Errorw("Error initializing service", "error", err)
		os.Exit(1)
	}

	// Initialize the API layer.
	if err := api.Init(ctx, muxer, service, log); err != nil {
		log.Errorf("Error initializing API layer", "error", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Handler:      muxer,
		Addr:         fmt.Sprintf(":%d", portNum),
		ReadTimeout:  time.Duration(timeout) * time.Second,
		WriteTimeout: time.Duration(timeout) * time.Second,
	}

	// Start server
	go func() {
		log.Infow("Listening for connections", "port", portNum)
		if err := srv.ListenAndServe(); err != nil {
			log.Infow("Server completed", "err", err)
		}
	}()

	// Block until we shutdown.
	waitForShutdown(ctx, srv, log, service.Shutdown)
}

// Set up the logger, condsidering any env vars.
func initLogging() (*zap.SugaredLogger, error) {
	var lg *zap.Logger
	var err error

	pdl := strings.ToLower(os.Getenv("IPVERIFY_LOG_LEVEL"))
	if strings.HasPrefix(pdl, "d") {
		logLevel = "development"
	}

	var cfg zap.Config
	if logLevel == "development" {
		cfg = zap.NewDevelopmentConfig()
	} else {
		cfg = zap.NewProductionConfig()
	}
	cfg.DisableStacktrace = true
	lg, err = cfg.Build()
	if err != nil {
		return nil, err
	}
	return lg.Sugar(), nil
}

// Setup for clean shutdown with signal handlers/cancel.
func waitForShutdown(ctx context.Context, srv *http.Server,
	log *zap.SugaredLogger, tasks ...cleanupTask) {
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive our signal.
	sig := <-interruptChan
	log.Debugw("Termination signal received", "signal", sig)
	for _, t := range tasks {
		t()
	}

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	srv.Shutdown(ctx)

	log.Infof("Shutting down")
}
