package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/dig"
	"google.golang.org/grpc"

	"github.com/WildanFrananda/kinetix-search-service/internal/config"
	searchv1 "github.com/WildanFrananda/kinetix-search-service/internal/contractgen/search/v1"
	"github.com/WildanFrananda/kinetix-search-service/internal/search"
	"github.com/WildanFrananda/kinetix-search-service/internal/wiring"
)

type searchd struct {
	dig.In
	Config config.Config
	Pool   *pgxpool.Pool
	HTTP   *http.Server
	GRPC   *grpc.Server
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(log); err != nil {
		log.Error("searchd stopped", "err", err, "kind", search.KindOf(err).String())
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	container, err := wiring.Searchd(log)
	if err != nil {
		return err
	}

	err = container.Invoke(func(d searchd) error {
		defer d.Pool.Close()

		return serve(ctx, log, d)
	})
	if err != nil {
		return dig.RootCause(err)
	}

	return nil
}

func serve(ctx context.Context, log *slog.Logger, d searchd) error {
	listener, err := net.Listen("tcp", d.Config.GRPCAddr)
	if err != nil {
		return search.Errf(
			search.KindDependencyUnavailable,
			"searchd.grpc",
			err,
			"binding %s",
			d.Config.GRPCAddr,
		)
	}

	log.Info("grpc registered", "service", searchv1.SearchService_ServiceDesc.ServiceName)

	serving := make(chan error, 2)
	go func() {
		log.Info("searchd http listening", "addr", d.Config.HTTPAddr)
		serving <- d.HTTP.ListenAndServe()
	}()

	go func() {
		log.Info("searchd grpc listening", "addr", listener.Addr().String())
		serving <- d.GRPC.Serve(listener)
	}()

	select {
	case err := <-serving:
		if errors.Is(err, http.ErrServerClosed) || errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}
		return search.Errf(search.KindDependencyUnavailable, "searchd.serve", err, "serving")
	case <-ctx.Done():
	}

	log.Info("searchd draining")
	d.GRPC.GracefulStop()

	shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	return d.HTTP.Shutdown(shutdown)
}
