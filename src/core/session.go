package core

import (
	"crypto/rand"
	"math/big"
	"net"
	"sync"
	"time"

	"github.com/iagoMAO/Botzin.OpenMASE/protocol"
	"github.com/rs/zerolog/log"
)

type ConnType int

const (
	ConnTypeMASE ConnType = iota
	ConnTypeBUDDY
)

// Session for a connected Player
type Session struct {
	UserId    int
	Token     int
	Status    protocol.BuddyStatus
	Conn      net.Conn
	BuddyConn net.Conn

	LastScoreAt time.Time

	writeMutex sync.Mutex
}

var (
	sessionsByUser = make(map[int]*Session)
	sessionsByConn = make(map[net.Conn]*Session)
	sessionMutex   sync.RWMutex
)

func (s *Session) SetStatus(status protocol.BuddyStatus) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()
	s.Status = status
}

func (s *Session) GetStatus() protocol.BuddyStatus {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()
	return s.Status
}

func (s *Session) connection(connType ConnType) net.Conn {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	if connType == ConnTypeBUDDY {
		return s.BuddyConn
	}
	return s.Conn
}

func (s *Session) write(connType ConnType, packets ...[]byte) {
	conn := s.connection(connType)

	if conn == nil {
		return
	}

	// Packets written from different goroutines must not interleave
	s.writeMutex.Lock()
	defer s.writeMutex.Unlock()

	for _, packet := range packets {
		if _, err := conn.Write(packet); err != nil {
			log.Debug().Msgf("Failed writing to user %d: %s", s.UserId, err)
			return
		}
	}
}

// WriteMASE sends packets through the MASE connection (no-op if not connected).
func (s *Session) WriteMASE(packets ...[]byte) { s.write(ConnTypeMASE, packets...) }

// WriteBuddy sends packets through the Buddy connection (no-op if not connected).
func (s *Session) WriteBuddy(packets ...[]byte) { s.write(ConnTypeBUDDY, packets...) }

// GetToken returns the token handed out on the MASE login.
func (s *Session) GetToken() int {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()
	return s.Token
}

// MASEAddr returns the remote address of the MASE connection (nil if not connected).
func (s *Session) MASEAddr() net.Addr {
	if conn := s.connection(ConnTypeMASE); conn != nil {
		return conn.RemoteAddr()
	}
	return nil
}

// HasBuddy returns whether the Buddy connection is up.
func (s *Session) HasBuddy() bool { return s.connection(ConnTypeBUDDY) != nil }

// Close drops both connections of this session.
func (s *Session) Close() {
	for _, conn := range []net.Conn{s.connection(ConnTypeMASE), s.connection(ConnTypeBUDDY)} {
		if conn != nil {
			conn.Close()
		}
	}
}

// NewToken generates a random session token, used as the login "magic number".
func NewToken() int {
	n, err := rand.Int(rand.Reader, big.NewInt(899999999))
	if err != nil {
		return int(time.Now().UnixNano()%899999999) + 100000000
	}
	return int(n.Int64()) + 100000000
}

// IsLoggedIn returns whether the user has a live MASE connection.
func IsLoggedIn(userId int) bool {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	session, exists := sessionsByUser[userId]
	return exists && session.Conn != nil
}

// RegisterConnection binds a connection to the user's session (creating it if needed)
// and returns the session.
func RegisterConnection(conn net.Conn, userId int, connType ConnType) *Session {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	session, exists := sessionsByUser[userId]
	if !exists {
		session = &Session{UserId: userId, Status: protocol.BUDDY_STATUS_ONLINE}
		sessionsByUser[userId] = session
	}

	var previous net.Conn

	switch connType {
	case ConnTypeMASE:
		previous = session.Conn
		session.Conn = conn
	case ConnTypeBUDDY:
		previous = session.BuddyConn
		session.BuddyConn = conn
	}

	// A reconnect replaces the old socket
	if previous != nil && previous != conn {
		delete(sessionsByConn, previous)
		go previous.Close()
	}

	sessionsByConn[conn] = session

	return session
}

// SetToken stores the login token of the user's session.
func SetToken(userId int, token int) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	if session, exists := sessionsByUser[userId]; exists {
		session.Token = token
	}
}

func GetSessionByUserId(userId int) *Session {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()
	return sessionsByUser[userId]
}

// GetSessionByToken finds the session that was handed out the given token.
func GetSessionByToken(token int) *Session {
	if token == 0 {
		return nil
	}

	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	for _, session := range sessionsByUser {
		if session.Token == token && session.Conn != nil {
			return session
		}
	}
	return nil
}

func GetAllSessions() []*Session {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()

	sessions := make([]*Session, 0, len(sessionsByUser))
	for _, v := range sessionsByUser {
		sessions = append(sessions, v)
	}
	return sessions
}

func GetSession(conn net.Conn) *Session {
	sessionMutex.RLock()
	defer sessionMutex.RUnlock()
	return sessionsByConn[conn]
}

// RemoveSession unbinds the connection and returns the session it belonged to
// plus whether the user is now fully offline.
func RemoveSession(conn net.Conn) (*Session, bool) {
	sessionMutex.Lock()
	defer sessionMutex.Unlock()

	session, exists := sessionsByConn[conn]
	if !exists {
		return nil, false
	}

	delete(sessionsByConn, conn)

	if session.Conn == conn {
		session.Conn = nil
		session.Token = 0
	} else if session.BuddyConn == conn {
		session.BuddyConn = nil
	}

	if session.Conn == nil && session.BuddyConn == nil {
		delete(sessionsByUser, session.UserId)
		return session, true
	}

	return session, false
}
