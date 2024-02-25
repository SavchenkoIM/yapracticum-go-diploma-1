package utils

import (
	"context"
	"go.uber.org/zap"
	"sync"
	"time"
)

type SafeTime struct {
	m    sync.RWMutex
	time time.Time
}

func (st *SafeTime) Get() time.Time {
	st.m.RLock()
	defer st.m.RUnlock()
	return st.time
}

func (st *SafeTime) Set(val time.Time) {
	st.m.Lock()
	defer st.m.Unlock()
	st.time = val
}

type CtxCancellWaiter struct {
	waitUntil  SafeTime
	interval   time.Duration
	Ctx        context.Context
	objectName string
	logger     *zap.Logger
}

func NewCtxCancellWaiter(ctx context.Context, interval time.Duration) *CtxCancellWaiter {
	return &CtxCancellWaiter{Ctx: ctx,
		interval:  interval,
		waitUntil: SafeTime{time: time.Now()}}
}

func (ccw *CtxCancellWaiter) Scan() error {
	var tUntil time.Time
	for {
		time.Sleep(50 * time.Millisecond)

		if err := ccw.Ctx.Err(); err != nil {
			return err
		}

		tUntil = ccw.waitUntil.Get()
		if tUntil.Before(time.Now()) {
			if ccw.interval > 0 {
				ccw.waitUntil.Set(tUntil.Add(ccw.interval))
			}
			return nil
		}
	}
}

func (ccw *CtxCancellWaiter) SetTimeUntil(time time.Time) {
	ccw.waitUntil.Set(time)
}
