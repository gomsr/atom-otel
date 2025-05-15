package mon

import (
	"context"
	"go.mongodb.org/mongo-driver/mongo"
	mopt "go.mongodb.org/mongo-driver/mongo/options"
)

type decoratedCollection struct {
	*mongo.Collection
	name string
}

func (c *decoratedCollection) Aggregate(ctx context.Context, pipeline any,
	opts ...*mopt.AggregateOptions) (cur *mongo.Cursor, err error) {
	ctx, span := startSpan(ctx, "Aggregate")
	defer func() {
		endSpan(span, err)
	}()

	return c.Collection.Aggregate(ctx, pipeline, opts...)
}
