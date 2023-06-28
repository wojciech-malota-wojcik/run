package run

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/outofforest/ioc/v2"
	"github.com/outofforest/logger"
	"github.com/outofforest/parallel"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
)

// Run runs the application.
func Run(appName string, containerBuilder func(c *ioc.Container), appFunc interface{}) {
	log := logger.New(logger.ConfigureWithCLI(logger.DefaultConfig))
	if appName != "" && appName != "." {
		log = log.Named(appName)
	}
	ctx := logger.WithLogger(context.Background(), log)

	var exitBySignal bool
	err := parallel.Run(ctx, func(ctx context.Context, spawn parallel.SpawnFn) error {
		spawn("", parallel.Exit, func(ctx context.Context) error {
			defer func() {
				_ = log.Sync()
			}()

			c := ioc.New()
			c.Singleton(func() context.Context {
				return ctx
			})
			if containerBuilder != nil {
				containerBuilder(c)
			}

			var err error
			c.Call(appFunc, &err)
			return err
		})
		spawn("signals", parallel.Exit, func(ctx context.Context) error {
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

			select {
			case <-ctx.Done():
				return nil
			case sig := <-sigs:
				exitBySignal = true
				log.Info("Signal received, terminating...", zap.Stringer("signal", sig))
			}
			return nil
		})
		return nil
	})

	switch {
	case err == nil:
	case errors.Is(err, context.Canceled) && exitBySignal:
	case errors.Is(err, pflag.ErrHelp):
		os.Exit(2)
	default:
		log.Error("Application returned error", zap.Error(err))
		os.Exit(1)
	}

	// This is done intentionally to be able to wrap one app inside another without side effects,
	// e.g. when a tool starts a service. By calling os.Exit here, when started service quits,
	// control is not passed back to the tool.
	os.Exit(0)
}
