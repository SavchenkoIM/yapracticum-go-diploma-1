package storage

import (
	"context"
	"go.uber.org/zap"
	"time"
)

type CtxCancellWaiter struct {
	skipSeconds int
	ctx         context.Context
	objectName  string
	logger      *zap.Logger
	currCycle   int
}

func NewCtxCancellWaiter(ctx context.Context, objectName string, logger *zap.Logger, skipSeconds int) *CtxCancellWaiter {
	return &CtxCancellWaiter{ctx: ctx, skipSeconds: skipSeconds, objectName: objectName, logger: logger, currCycle: 0}
}

func (ccw *CtxCancellWaiter) Scan() (scan bool, ctxStatus error) {
	time.Sleep(1 * time.Second)

	if err := ccw.ctx.Err(); err != nil {
		ccw.logger.Sugar().Infof("%s routine exited: context cancelled.", ccw.objectName)
		return false, err
	}
	if ccw.currCycle >= ccw.skipSeconds {
		ccw.currCycle = 0
		return true, nil
	}
	ccw.currCycle++
	return false, nil
}

func (ccw *CtxCancellWaiter) SetSkipSeconds(duration int) {
	ccw.currCycle = 0
	ccw.skipSeconds = duration
}
