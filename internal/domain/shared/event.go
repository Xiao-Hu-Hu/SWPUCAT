package shared

import "time"

type Event interface {
	Type() string
	OccurredAt() time.Time
	AggregateID() int64
}

type EventPublisher interface {
	Publish(event Event) error
}

type EventHandler interface {
	Handle(event Event) error
}
