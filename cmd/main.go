package main

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nbd-wtf/go-nostr"
	"github.com/urfave/cli"
	"io"
	"log"
	"net"
	"net/http"
	"nostr_relay"
	"os"
	"os/signal"
	"syscall"
)

var cfg *nostr_relay.Config
var pgpool *pgxpool.Pool

func main() {
	app := cli.NewApp()
	app.Flags = nostr_relay.Flags
	app.Version = "v0.0.1"
	app.Name = "nostr-relay"
	app.Usage = "NOSTR Relay"
	app.Description = "NOSTR Relay"
	app.Action = StartCommand

	if err := app.Run(os.Args); err != nil {
		log.Fatalf("Application failed, error: %s", err)
	}
}

func StartCommand(ctx *cli.Context) error {
	// Config
	cfg = nostr_relay.NewConfig(ctx)
	jsonConfig, err := json.Marshal(cfg)
	if err != nil {
		log.Fatalf("Failed to marshal config, error: %s", err)
	} else {
		log.Printf("Starting NOSTR Relay with config: %s", jsonConfig)
	}

	appCtx, appCtxCancel := context.WithCancel(context.Background())

	// Database
	database, err := nostr_relay.InitStore(appCtx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize store, error: %s", err)
	}
	defer database.Close()

	// PG Listener
	pgListenerConn, err := database.Acquire(appCtx)
	if err != nil {
		log.Fatalf("Failed to acquire database connection, error: %s", err)
	}
	pgListener := nostr_relay.NewPGListener(pgListenerConn)
	go func() {
		defer pgListenerConn.Release()
		pgListener.Start(appCtx)
	}()

	// HTTP Listener
	httpServer := &http.Server{}
	httpServer.Addr = ":8080"
	httpServer.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		writer := wsutil.NewWriter(conn, ws.StateServerSide, ws.OpText)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Println("Closing all connections...")
				return
			} else {
				log.Printf("[ERROR] Unknown error when accepting connection, error: %s", err)
				return
			}
		}

		select {
		case <-appCtx.Done():
			_ = conn.Close()
			return
		default:
		}

		go func() {
			connCtx, connCtxCancel := context.WithCancel(appCtx)
			defer connCtxCancel()
			defer func() { _ = conn.Close() }()

			for {
				select {
				case <-appCtx.Done():
					return
				default:
				}

				data, _, err := wsutil.ReadClientData(conn)
				if err == io.EOF {
					log.Printf("[DEBUG] client side closed")
					return
				} else if err != nil {
					log.Printf("[ERROR] wsutil.ReadClientData: error: %s", err)
					return
				}

				envelope := nostr.ParseMessage(data)
				switch envelope.Label() {
				case "EVENT":
					event, ok := envelope.(*nostr.EventEnvelope)
					if !ok {
						log.Printf("[ERROR] Failed to cast envelope to EventEnvelope")
						return
					}
					log.Printf("[DEBUG] EventEnvelope: %s", *event)
					if err = database.AddEvent(connCtx, &event.Event); err != nil {
						log.Printf("[ERROR] Failed to add event to database, error: %s", err)
						return
					}
				case "REQ":
					req, ok := envelope.(*nostr.ReqEnvelope)
					if !ok {
						log.Printf("[ERROR] Failed to cast envelope to ReqEnvelope")
						return
					}
					log.Printf("[DEBUG] ReqEnvelope: %s", *req)

					events, err := database.FilterEvents(connCtx, &req.Filters)
					if err != nil {
						log.Printf("[ERROR] Failed to filter events, error: %s", err)
						return
					}

					for _, event := range events {
						eventEnvelope := nostr.EventEnvelope{
							SubscriptionID: &req.SubscriptionID,
							Event:          *event,
						}
						marshaled, err := eventEnvelope.MarshalJSON()
						if err != nil {
							log.Printf("[ERROR] Failed to marshal event envelope, event: %s, error: %s", event, err)
							return
						}
						err = wsutil.WriteServerText(conn, marshaled)
						if err != nil {
							log.Printf("[ERROR] Failed to write event envelope, error: %s", err)
							return
						}
						err = writer.Flush()
						if err != nil {
							log.Printf("[ERROR] Failed to flush event, error: %s", err)
							return
						}
					}

					eoseEnvelope := nostr.EOSEEnvelope(req.SubscriptionID)
					marshaled, err := eoseEnvelope.MarshalJSON()
					if err != nil {
						log.Printf("[ERROR] Failed to marshal eose envelope, , error: %s", err)
						return
					}
					err = wsutil.WriteServerText(conn, marshaled)
					if err != nil {
						log.Printf("[ERROR] Failed to write eose envelope, error: %s", err)
						return
					}
					err = writer.Flush()
					if err != nil {
						log.Printf("[ERROR] Failed to flush eose, error: %s", err)
						return
					}

					eventChan := pgListener.AddSubscriber(
						req.SubscriptionID,
						req.Filters,
					)
					go func(subscriptionID string, eventChan chan nostr.Event) {
						for {
							select {
							case <-connCtx.Done():
								return
							case event := <-eventChan:
								eventEnvelope := nostr.EventEnvelope{SubscriptionID: &subscriptionID, Event: event}
								marshaledEvent, err := eventEnvelope.MarshalJSON()
								if err != nil {
									log.Printf("[ERROR] Failed to marshal event envelope, event: %s, error: %s", event, err)
								} else {
									err = wsutil.WriteServerText(conn, marshaledEvent)
									if err != nil {
										log.Printf("[ERROR] Failed to write event envelope, error: %s", err)
									}
								}
							}
						}
					}(req.SubscriptionID, eventChan)
				case "NOTICE":
					notice, ok := envelope.(*nostr.NoticeEnvelope)
					if !ok {
						log.Printf("[ERROR] Failed to cast envelope to NoticeEnvelope")
						return
					}
					log.Printf("[DEBUG] NoticeEnvelope: %s", *notice)
				case "EOSE":
					eose, ok := envelope.(*nostr.EOSEEnvelope)
					if !ok {
						log.Printf("[ERROR] Failed to cast envelope to EoseEnvelope")
						return
					}
					log.Printf("[DEBUG] EoseEnvelope: %s", *eose)
				case "CLOSE":
					close_, ok := envelope.(*nostr.CloseEnvelope)
					if !ok {
						log.Printf("[ERROR] Failed to cast envelope to CloseEnvelope")
						return
					}
					log.Printf("[DEBUG] CloseEnvelope: %s", *close_)
					return
				case "OK":
					ok_, ok := envelope.(*nostr.OKEnvelope)
					if !ok {
						log.Printf("[ERROR] Failed to cast envelope to OKEnvelope")
						return
					}
					log.Printf("[DEBUG] OKEnvelope: %+v", *ok_)
				case "AUTH":
					auth, ok := envelope.(*nostr.AuthEnvelope)
					if !ok {
						log.Printf("[ERROR] Failed to cast envelope to AuthEnvelope")
						return
					}
					log.Printf("[DEBUG] AuthEnvelope: %+v", *auth)
				default:
					log.Printf("[DEBUG] Unknown envelope label: %s", envelope.Label())
					return
				}
			}
		}()
	})

	// Graceful shutdown on SIGINT and SIGTERM
	exitSignal := make(chan os.Signal, 1)
	go func() {
		signal.Notify(exitSignal, syscall.SIGINT, syscall.SIGTERM)
		<-exitSignal
		appCtxCancel()
		_ = httpServer.Close()
		database.Close()
		_ = os.Stdin.Close()
	}()

	err = httpServer.ListenAndServe()
	if err != http.ErrServerClosed {
		log.Fatalf("[ERROR] HTTP listener exit, error: %s", err)
	}
	return nil
}
