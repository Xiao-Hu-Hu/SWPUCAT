package event

import (
	"SWPUCAT/internal/domain/shared"
	"log"
)

type NoOpPublisher struct{}

func NewNoOpPublisher() *NoOpPublisher {
	return &NoOpPublisher{}
}

func (p *NoOpPublisher) Publish(event shared.Event) error {
	log.Printf("Event published: %s (aggregate: %d)", event.Type(), event.AggregateID())
	return nil
}
