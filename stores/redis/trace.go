package redis

import (
	"context"
	"errors"

	"github.com/gomsr/atom-otel/trace"
	red "github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const spanName = "redis"

func startSpan(ctx context.Context, cmds ...red.Cmder) (context.Context, func(err error)) {
	tracer := trace.TracerFromContext(ctx)

	ctx, span := tracer.Start(ctx,
		spanName,
		oteltrace.WithSpanKind(oteltrace.SpanKindClient),
	)

	cmdStrs := make([]string, 0, len(cmds))
	for _, cmd := range cmds {
		cmdStrs = append(cmdStrs, cmd.Name())
	}
	span.SetAttributes(attribute.Key("redis.cmds").StringSlice(cmdStrs))

	return ctx, func(err error) {
		defer span.End()

		if err == nil || errors.Is(err, red.Nil) {
			span.SetStatus(codes.Ok, "")
			return
		}

		span.SetStatus(codes.Error, err.Error())
		span.RecordError(err)
	}
}
