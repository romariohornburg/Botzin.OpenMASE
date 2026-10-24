package main

import (
	"bufio"
	"errors"
	"io"
	"net"
	"time"

	"github.com/iagoMAO/Botzin.OpenMASE/protocol"
	"github.com/rs/zerolog/log"
)

// Frames that fail to decrypt in a row before we give up on the connection
const maxMalformedFrames = 5

// readMessages reads length prefixed frames until the connection closes, the handler
// returns false or the stream gets out of sync.
func readMessages(conn net.Conn, service string, handle func(protocol.NetworkMessage) bool) {
	if tcp, ok := conn.(*net.TCPConn); ok {
		// Detects clients that vanished without closing the socket (crash, network drop)
		tcp.SetKeepAlive(true)
		tcp.SetKeepAlivePeriod(30 * time.Second)
	}

	reader := bufio.NewReader(conn)
	malformed := 0

	for {
		frame, err := protocol.ReadFrame(reader)

		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
				log.Debug().Msgf("%s - closing %s: %s", service, conn.RemoteAddr(), err)
			}
			return
		}

		message, err := protocol.DecryptPacket(frame)

		if err != nil {
			malformed++
			log.Warn().Msgf("%s - invalid packet from %s: %s", service, conn.RemoteAddr(), err)

			if malformed >= maxMalformedFrames {
				return
			}
			continue
		}

		malformed = 0

		if !handle(message) {
			return
		}
	}
}
