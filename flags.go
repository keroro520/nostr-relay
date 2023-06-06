package nostr_relay

import (
	"github.com/urfave/cli"
)

// Flags

const envVarPrefix = "NOSTR_RELAY"

func prefixEnvVar(name string) string {
	return envVarPrefix + "_" + name
}

var Flags = []cli.Flag{
	//WSPort,
	//WSAddr,
	DatabaseURL,
}

var (

	// WSPort = cli.UintFlag{
	// 	Name:   "ws.port",
	// 	Usage:  "Listening websocket port",
	// 	Value:  8080,
	// 	EnvVar: prefixEnvVar("WS_PORT"),
	// }
	// WSAddr = cli.StringFlag{
	// 	Name:   "ws.addr",
	// 	Usage:  "Listening websocket address",
	// 	Value:  "0.0.0.0",
	// 	EnvVar: prefixEnvVar("WS_ADDR"),
	// }

	DatabaseURL = cli.StringFlag{
		Name:   "database.url",
		Usage:  "Database URL",
		Value:  "postgres://localhost:5432/nostr_relay?sslmode=disable",
		EnvVar: prefixEnvVar("DATABASE_URL"),
	}
)
