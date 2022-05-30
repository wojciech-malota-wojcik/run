package run

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/outofforest/ioc/v2"
	"github.com/outofforest/logger"
	"github.com/outofforest/parallel"
	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
)

// AppRunner is used to run application
type AppRunner func(appFunc parallel.Task)

var mu sync.Mutex

// Service runs service app
func Service(appName string, containerBuilder func(c *ioc.Container), appFunc interface{}) {
	logger.ConfigureWithCLI(logger.ServiceDefaultConfig)
	c := ioc.New()
	if containerBuilder != nil {
		containerBuilder(c)
	}
	c.Call(run(filepath.Base(appName), logger.ServiceDefaultConfig, appFunc, parallel.Fail))
}

// Tool runs tool app
func Tool(appName string, containerBuilder func(c *ioc.Container), appFunc interface{}) {
	c := ioc.New()
	if containerBuilder != nil {
		containerBuilder(c)
	}
	c.Call(run(filepath.Base(appName), logger.ToolDefaultConfig, appFunc, parallel.Exit))
}

func run(appName string, loggerConfig logger.Config, setupFunc interface{}, exit parallel.OnExit) func(c *ioc.Container) {
	return func(c *ioc.Container) {
		log := logger.New(logger.ConfigureWithCLI(loggerConfig))
		if appName != "" && appName != "." {
			log = log.Named(appName)
		}
		ctx := logger.WithLogger(context.Background(), log)

		err := parallel.Run(ctx, func(ctx context.Context, spawn parallel.SpawnFn) error {
			spawn("", exit, func(ctx context.Context) error {
				defer func() {
					_ = log.Sync()
				}()

				c.Singleton(func() context.Context {
					return ctx
				})
				var err error
				c.Call(setupFunc, &err)
				return err
			})
			spawn("signals", parallel.Exit, func(ctx context.Context) error {
				sigs := make(chan os.Signal, 1)
				signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

				select {
				case <-ctx.Done():
					return ctx.Err()
				case sig := <-sigs:
					log.Info("Signal received, terminating...", zap.Stringer("signal", sig))
				}
				return nil
			})
			return nil
		})

		switch {
		case err == nil:
		case errors.Is(err, ctx.Err()):
		case errors.Is(err, pflag.ErrHelp):
			os.Exit(2)
		default:
			log.Error("Application returned error", zap.Error(err))
			os.Exit(1)
		}
	}
}
