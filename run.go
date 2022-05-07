package run

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/outofforest/ioc/v2"
	"github.com/outofforest/logger"
	"github.com/outofforest/parallel"
	"go.uber.org/zap"
)

// AppRunner is used to run application
type AppRunner func(appFunc parallel.Task)

var mu sync.Mutex

// Service runs service app
func Service(appName string, containerBuilder func(c *ioc.Container), appFunc interface{}) {
	c := ioc.New()
	if containerBuilder != nil {
		containerBuilder(c)
	}
	c.Call(run(context.Background(), filepath.Base(appName), appFunc, parallel.Fail))
}

// Tool runs tool app
func Tool(appName string, containerBuilder func(c *ioc.Container), appFunc interface{}) {
	c := ioc.New()
	if containerBuilder != nil {
		containerBuilder(c)
	}
	c.Call(run(context.Background(), filepath.Base(appName), appFunc, parallel.Exit))
}

func run(ctx context.Context, appName string, setupFunc interface{}, exit parallel.OnExit) func(c *ioc.Container) {
	return func(c *ioc.Container) {
		exitCode := 0
		log := logger.Get(newContext())
		if appName != "" && appName != "." {
			log = log.Named(appName)
		}
		ctx := logger.WithLogger(ctx, log)
		defer func() {
			if exitCode != 0 {
				os.Exit(exitCode)
			}
		}()

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

		if err != nil && !errors.Is(err, ctx.Err()) {
			log.Error("Application returned error", zap.Error(err))
			exitCode = 1
		}
	}
}
