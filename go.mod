module github.com/outofforest/run

go 1.16

replace github.com/ridge/parallel => github.com/outofforest/parallel v0.1.2

require (
	github.com/outofforest/ioc/v2 v2.5.0
	github.com/outofforest/logger v0.2.0
	github.com/ridge/parallel v0.1.1
	go.uber.org/atomic v1.9.0 // indirect
	go.uber.org/multierr v1.7.0 // indirect
	go.uber.org/zap v1.19.1
)
