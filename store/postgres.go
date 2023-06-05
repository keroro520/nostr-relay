package store

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nbd-wtf/go-nostr"
	"sort"
)

type Store struct {
	*pgxpool.Pool
}

func InitStore(ctx context.Context, postgresURL string) (*Store, error) {
	dbpool, err := pgxpool.New(ctx, postgresURL)
	if err != nil {
		return nil, err
	}

	_, err = dbpool.Exec(ctx, `
CREATE TABLE IF NOT EXISTS event (
	id 			text 			PRIMARY KEY,
	pubkey 		text 			NOT NULL,
	kind 		integer 		NOT NULL,
	created_at 	timestamp 		NOT NULL,
	sig 		text 			NOT NULL,
	content 	text 			NOT NULL,
	origin      bytea           NOT NULL,
	etags		text[],
	ptags		text[]
);

CREATE OR REPLACE FUNCTION notify_event() RETURNS trigger AS $$
    BEGIN
    	PERFORM pg_notify('nostr_event', row_to_json(NEW.origin)::text);
		RETURN NEW;
	END;
$$ LANGUAGE plpgsql;
    
    
    
DROP TRIGGER IF EXISTS nostr_event_trigger ON event;
CREATE TRIGGER nostr_event AFTER INSERT ON event FOR EACH ROW EXECUTE FUNCTION notify_event();
`)

	if err != nil {
		return nil, fmt.Errorf("init table 'event', error: %s", err)
	}

	return &Store{dbpool}, nil
}

func (s *Store) Close() {
	s.Pool.Close()
}

func (s *Store) AddEvent(ctx context.Context, event *nostr.Event) error {
	origin, err := event.MarshalJSON()
	if err != nil {
		return err
	}
	_, err = s.Exec(
		ctx,
		`INSERT INTO event (id, pubkey, kind, created_at, sig, content, origin, etags, ptags) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		event.ID,
		event.PubKey,
		event.Kind,
		event.CreatedAt.Time(),
		event.Sig,
		event.Content,
		origin,
		etags(event),
		ptags(event),
	)
	return err
}

func (s *Store) FilterEvents(ctx context.Context, filters *nostr.Filters) ([]*nostr.Event, error) {
	condition := filtersToSQLCond(filters)
	query := "SELECT origin FROM event WHERE " + condition
	rows, err := s.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*nostr.Event
	for rows.Next() {
		var event nostr.Event
		var origin []byte
		if err := rows.Scan(&origin); err != nil {
			return nil, err
		}

		err = event.UnmarshalJSON(origin)
		if err != nil {
			return nil, err
		}

		events = append(events, &event)
	}
	return events, nil
}

func etags(event *nostr.Event) []string {
	return tagsByLabel(event, "e")
}

func ptags(event *nostr.Event) []string {
	return tagsByLabel(event, "p")
}

func tagsByLabel(event *nostr.Event, label string) []string {
	matchedTags := event.Tags.GetAll([]string{label})

	values := make([]string, len(matchedTags))
	for i, tag := range matchedTags {
		values[i] = tag.Value()
	}

	sort.Strings(values)
	return values
}
