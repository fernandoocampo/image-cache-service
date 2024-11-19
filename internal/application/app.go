package application

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/esadikov/interview-fm-backend/internal/resizers"
	lru "github.com/hashicorp/golang-lru"
)

const (
	hostport = "localhost:8080"
)

// Application
type Application struct {
}

func NewApplication() *Application {
	return &Application{}
}

// Run starts this application, loading settings and injecting dependencies.
func (a *Application) Run() error {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	handlerOptions := &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}

	loggerHandler := slog.NewJSONHandler(os.Stdout, handlerOptions)
	logger := slog.New(loggerHandler)

	logger.Info("log settings", slog.String("level", handlerOptions.Level.Level().String()))

	slog.SetDefault(logger)

	cache, err := lru.New(1024)
	if err != nil {
		slog.Error("creating cache", slog.String("error", err.Error()))

		return fmt.Errorf("unable to create cache: %w", err)
	}

	serviceSetup := resizers.ServiceSetup{
		Cache: cache,
	}
	svc := resizers.NewService(&serviceSetup)
	svc.StartWorkerHandler(ctx)

	handler := resizers.NewHandlerMaker(svc)

	mux := http.NewServeMux()
	mux.Handle("/v1/resize", handler.MakeResizeHandler())
	mux.Handle("/v1/image/", handler.MakeGetImageHandler())
	address := hostport

	slog.Info("Listening on ", slog.String("host+port", hostport))
	// When running on docker mac, can't listen only on localhost
	panic(http.ListenAndServe(address, mux))
}
