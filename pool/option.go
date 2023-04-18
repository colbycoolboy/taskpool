package pool

import "time"

type Option func(o *Options)

func loadOptions(options ...Option) *Options {
	o := new(Options)
	for _, option := range options {
		option(o)
	}
	return o
}

type Options struct {
	Size int32
	// 清理间隔 等于0时不清理
	ExpireWorkerCleanInterval time.Duration
	MaxWaitTaskNum            int32
	PanicHandler              func(interface{})
}

func ExpreWorkerCleanInterval(e time.Duration) Option {
	return func(opt *Options) {
		opt.ExpireWorkerCleanInterval = e
	}
}

func MaxWaitTaskNum(m int32) Option {
	return func(opt *Options) {
		opt.MaxWaitTaskNum = m
	}
}

func Size(s int32) Option {
	return func(opt *Options) {
		opt.Size = s
	}
}

func PanicHandler(h func(interface{})) Option {
	return func(opt *Options) {
		opt.PanicHandler = h
	}
}
