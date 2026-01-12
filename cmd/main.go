package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/nuntiiscore/envmerge/internal/config"
	"github.com/nuntiiscore/envmerge/internal/envmerge/service"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	os.Exit(run(context.Background()))
}

func run(ctx context.Context) int {
	cfg := initConfig()
	srv, err := service.New(cfg.Src, cfg.Dst, cfg.Force)
	if err != nil {
		slog.Default().ErrorContext(ctx, "service initialization failed", "error", err)
		return 1
	}

	if err = srv.Run(); err != nil {
		slog.Default().ErrorContext(ctx, "service run failed", "error", err)
		return 1
	}

	return 0
}

func initConfig() config.Config {
	force := flag.Bool("force", false, "append updates for differing keys")
	dst := flag.String("dst", ".env", "destination .env file path")
	src := flag.String("src", ".env.example", "source .env.example file path")
	flag.Parse()

	return config.Config{
		Force: *force,
		Dst:   *dst,
		Src:   *src,
	}
}
