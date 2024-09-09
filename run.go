package run

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/pkg/errors"
	"github.com/spf13/pflag"
	"go.uber.org/zap"

	"github.com/outofforest/ioc/v2"
	"github.com/outofforest/logger"
	"github.com/outofforest/parallel"
)

// FlavourFunc represents a flavour function.
type FlavourFunc func(ctx context.Context, appFunc parallel.Task) error

// WithFlavours runs task with flavours installed.
func WithFlavours(ctx context.Context, flavours []FlavourFunc, appFunc parallel.Task) error {
	for _, f := range flavours {
		currentAppFunc := appFunc
		appFunc = func(ctx context.Context) error {
			return f(ctx, currentAppFunc)
		}
	}

	return appFunc(ctx)
}

// Environment configures environment of an application to run.
type Environment interface {
	WithContainerBuilder(containerBuilder func(c *ioc.Container)) Environment
	WithFlavour(flavourFunc FlavourFunc) Environment
	Run(appName string, appFunc interface{})
}

// New creates new environment.
func New() Environment {
	return &environment{}
}

var _ Environment = &environment{}

type environment struct {
	containerBuilder func(c *ioc.Container)
	flavours         []FlavourFunc
}

// WithContainerBuilder sets IoC builder.
func (e *environment) WithContainerBuilder(containerBuilder func(c *ioc.Container)) Environment {
	e.containerBuilder = containerBuilder
	return e
}

// WithFlavour adds flavour to the environment.
func (e *environment) WithFlavour(flavourFunc FlavourFunc) Environment {
	e.flavours = append(e.flavours, flavourFunc)
	return e
}

// Run runs an application inside configured environment.
func (e *environment) Run(appName string, appFunc interface{}) {
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

			return WithFlavours(ctx, e.flavours, func(ctx context.Context) error {
				c := ioc.New()
				c.Singleton(func() context.Context {
					return ctx
				})
				if e.containerBuilder != nil {
					e.containerBuilder(c)
				}

				var err error
				c.Call(appFunc, &err)
				return err
			})
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
