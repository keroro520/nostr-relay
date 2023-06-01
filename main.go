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
	config "nostr_relay/config"
	"nostr_relay/flags"
	"nostr_relay/store"
	"os"
	"os/signal"
	"syscall"
)

var cfg *config.Config
var pgpool *pgxpool.Pool

func main() {
	app := cli.NewApp()
	app.Flags = flags.Flags
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
	cfg = config.NewConfig(ctx)
	jsonConfig, err := json.Marshal(cfg)
	if err != nil {
		log.Fatalf("Failed to marshal config, error: %s", err)
	} else {
		log.Printf("Starting NOSTR Relay with config: %s", jsonConfig)
	}

	appCtx, appCtxCancel := context.WithCancel(context.Background())

	// Database
	database, err := store.InitStore(appCtx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to initialize store, error: %s", err)
	}

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
			defer func() { _ = conn.Close() }()

			for {
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
					if err = database.AddEvent(appCtx, &event.Event); err != nil {
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

					events, err := database.FilterEvents(appCtx, &req.Filters)
					if err != nil {
						log.Printf("[ERROR] Failed to filter events, error: %s", err)
						return
					}
					log.Printf("[DEBUG] Filtered events: %s", events)

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
