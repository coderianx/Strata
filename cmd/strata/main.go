package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/coderianx/Strata/internal/config"
	"github.com/coderianx/Strata/internal/handlers"
	"github.com/coderianx/Strata/internal/store"
)

func main() {
	port := flag.Int("port", 9090, "port for run server")

	flag.Parse()

	err := config.Init()

	if err != nil {
		log.Fatal(err)
	}

	err = config.CheckEnvs()

	if err != nil {
		log.Fatal(err)
	}

	err = store.ConnectDatabase()

	if err != nil {
		log.Fatal(err)
	}

	defer store.DB.Close()

	err = store.CreateTables()

	if err != nil {
		log.Fatal(err)
	}

	apiServer := &http.Server{
		Addr: fmt.Sprintf(":%d", *port),

		Handler: handlers.APIRouter(),

		// Timeouts guard against slow or stalled clients holding
		// connections open.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// ListenAndServe blocks, so run it in the background and let the
	// main goroutine wait for a shutdown signal instead.
	go func() {
		log.Printf("strata: api listening on %s", apiServer.Addr)

		err := apiServer.ListenAndServe()

		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	// Ctrl+C or SIGTERM stops accepting new connections and lets
	// in-flight requests finish before the process exits.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)

	defer stop()

	<-ctx.Done()

	log.Println("strata: shutdown signal received, draining requests")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)

	defer cancel()

	err = apiServer.Shutdown(shutdownCtx)

	if err != nil {
		log.Printf("strata: graceful shutdown failed: %v", err)
	}
}
