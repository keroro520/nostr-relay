package main

import (
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nbd-wtf/go-nostr"
	"log"
)

type PGListener struct {
	*pgxpool.Conn
	Subscribers map[string]*Subscriber
}

func NewPGListener(conn *pgxpool.Conn) *PGListener {
	return &PGListener{
		Conn:        conn,
		Subscribers: make(map[string]*Subscriber, 0),
	}
}

func (p *PGListener) AddSubscriber(subscriptionID string, filters nostr.Filters) chan nostr.Event {
	sub := &Subscriber{
		SubscriptionID: subscriptionID,
		Filters:        filters,
		Chan:           make(chan nostr.Event, 100),
	}
	p.Subscribers[subscriptionID] = sub
	return sub.Chan
}

func (p *PGListener) RemoveSubscriber(subscriptionId string) {
	delete(p.Subscribers, subscriptionId)
}

func (p *PGListener) Start(ctx context.Context) {
	if _, err := p.Exec(ctx, "LISTEN nostr_events"); err != nil {
		log.Fatalf("Failed to listen nostr_events, error: %s", err)
	}
	for {
		notification, err := p.Conn.Conn().WaitForNotification(ctx)
		if err != nil {
			log.Printf("[ERROR] Failed to receive notification: %s", err)
			continue
		}

		var event nostr.Event
		err = event.UnmarshalJSON([]byte(notification.Payload))
		if err != nil {
			log.Printf("[ERROR] Failed to unmarshal notification event: %s", err)
			continue
		}

		for _, sub := range p.Subscribers {
			sub.NotifyIfMatch(&event)
		}
	}
}

type Subscriber struct {
	SubscriptionID string
	Filters        nostr.Filters
	Chan           chan nostr.Event
}

func (p *Subscriber) NotifyIfMatch(event *nostr.Event) {
	if p.Filters.Match(event) {
		select {
		case p.Chan <- *event:
		default:
			log.Printf("[WARN] Subscriber channel full, dropping event, subscription: %s", p.SubscriptionID)
		}
	}
}
