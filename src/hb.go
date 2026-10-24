package main

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/iagoMAO/Botzin.OpenMASE/utils"

	"github.com/rs/zerolog/log"
)

func StartHB() {
	defer utils.Recover("HB listener")

	// Create the listener
	listener, err := net.Listen("tcp", fmt.Sprintf(":%s", cfg.HB_PORT))

	if err != nil {
		log.Error().Msgf("Listening error: %s", err)
		return
	}

	log.Info().Msgf("HB - Successfully started listening on port %s.", cfg.HB_PORT)

	// Close the socket once we're done
	defer listener.Close()

	for {
		conn, err := listener.Accept()

		if err != nil {
			log.Error().Msgf("Error thrown whilst accepting connection: %s", err)
			continue
		}

		go handleHBConnection(conn)
	}
}

// handleHBConnection just keeps the connection open (discarding whatever comes in) until
// the client closes it, so sockets don't leak.
func handleHBConnection(conn net.Conn) {
	defer conn.Close()
	defer utils.Recover("HB connection")

	if tcp, ok := conn.(*net.TCPConn); ok {
		tcp.SetKeepAlive(true)
		tcp.SetKeepAlivePeriod(30 * time.Second)
	}

	io.Copy(io.Discard, conn)
}
