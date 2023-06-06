package nostr_relay

import (
	"context"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	"net"
	"net/http"
	"sync"
)

type Server struct {
	appCtx   context.Context
	wg       sync.WaitGroup
	listener net.Listener
	pgpool   *pgxpool.Pool
}

func NewServer(appCtx context.Context, listener net.Listener, pgpool *pgxpool.Pool) *Server {
	return &Server{
		appCtx:   appCtx,
		wg:       sync.WaitGroup{},
		listener: listener,
		pgpool:   pgpool,
	}
}

func (s *Server) Start() {
	http.ListenAndServe(":8080", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			// handle error
		}
		go func() {
			defer conn.Close()

			for {
				msg, op, err := wsutil.ReadClientData(conn)
				if err != nil {
					// handle error
				}
				err = wsutil.WriteServerMessage(conn, op, msg)
				if err != nil {
					// handle error
				}
			}
		}()
	}))

	//log.Println("Server start ...")
	//for {
	//	if s.appCtx.Err() != nil {
	//		return
	//	}
	//
	//	conn, err := s.listener.Accept()
	//	if err != nil {
	//		if errors.Is(err, net.ErrClosed) {
	//			log.Println("closing all connections...")
	//			continue
	//		} else {
	//			log.Printf("[ERROR] Unknown error when accepting connection, error: %s", err)
	//			return
	//		}
	//	}
	//
	//	s.wg.Add(1)
	//	go func(conn net.Conn) {
	//		defer s.wg.Done()
	//		s.handle(conn)
	//	}(conn)
	//}
}

func (s *Server) Stop() {
	log.Println("Server stop")
	s.wg.Wait()

	_ = s.listener.Close()
}

func (s *Server) handle(conn net.Conn) {
}
