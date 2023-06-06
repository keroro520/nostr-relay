package nostr_relay

import (
	"github.com/urfave/cli"
)

type Config struct {
	//WSPort      uint   `json:"ws_port"`      // Websocket port
	//WSAddr      string `json:"ws_addr"`      // Websocket address
	DatabaseURL string `json:"database_url"` // Database URL
}

func NewConfig(ctx *cli.Context) *Config {
	return &Config{
		//WSPort:      ctx.GlobalUint(flags.WSPort.Name),
		//WSAddr:      ctx.GlobalString(flags.WSAddr.Name),
		DatabaseURL: ctx.GlobalString(DatabaseURL.Name),
	}
}
