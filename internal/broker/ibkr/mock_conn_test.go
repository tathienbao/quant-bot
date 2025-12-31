package ibkr

import (
	"bytes"
	"io"
	"net"
	"sync"
	"time"
)

// mockConn implements net.Conn for testing.
type mockConn struct {
	mu           sync.Mutex
	readBuf      *bytes.Buffer
	writeBuf     *bytes.Buffer
	closed       bool
	readDeadline time.Time
	readErr      error // Force read error
	writeErr     error // Force write error
}

func newMockConn() *mockConn {
	return &mockConn{
		readBuf:  new(bytes.Buffer),
		writeBuf: new(bytes.Buffer),
	}
}

// Read reads from the mock connection.
func (m *mockConn) Read(b []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return 0, io.EOF
	}

	if m.readErr != nil {
		return 0, m.readErr
	}

	// Simulate timeout if deadline is set and passed
	if !m.readDeadline.IsZero() && time.Now().After(m.readDeadline) {
		return 0, &mockTimeoutError{}
	}

	// If no data and deadline is set, simulate timeout
	if m.readBuf.Len() == 0 && !m.readDeadline.IsZero() {
		return 0, &mockTimeoutError{}
	}

	return m.readBuf.Read(b)
}

// Write writes to the mock connection.
func (m *mockConn) Write(b []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return 0, io.ErrClosedPipe
	}

	if m.writeErr != nil {
		return 0, m.writeErr
	}

	return m.writeBuf.Write(b)
}

// Close closes the mock connection.
func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

// LocalAddr returns a mock address.
func (m *mockConn) LocalAddr() net.Addr {
	return &mockAddr{network: "tcp", addr: "127.0.0.1:12345"}
}

// RemoteAddr returns a mock address.
func (m *mockConn) RemoteAddr() net.Addr {
	return &mockAddr{network: "tcp", addr: "127.0.0.1:7497"}
}

// SetDeadline sets read and write deadlines.
func (m *mockConn) SetDeadline(t time.Time) error {
	m.SetReadDeadline(t)
	return nil
}

// SetReadDeadline sets read deadline.
func (m *mockConn) SetReadDeadline(t time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readDeadline = t
	return nil
}

// SetWriteDeadline sets write deadline.
func (m *mockConn) SetWriteDeadline(t time.Time) error {
	return nil
}

// QueueResponse queues data to be read.
func (m *mockConn) QueueResponse(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readBuf.Write(data)
}

// GetWritten returns data written to the connection.
func (m *mockConn) GetWritten() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writeBuf.Bytes()
}

// SetReadError sets an error to return on read.
func (m *mockConn) SetReadError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.readErr = err
}

// SetWriteError sets an error to return on write.
func (m *mockConn) SetWriteError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.writeErr = err
}

// IsClosed returns true if connection is closed.
func (m *mockConn) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// mockAddr implements net.Addr.
type mockAddr struct {
	network string
	addr    string
}

func (a *mockAddr) Network() string { return a.network }
func (a *mockAddr) String() string  { return a.addr }

// mockTimeoutError implements net.Error for timeout.
type mockTimeoutError struct{}

func (e *mockTimeoutError) Error() string   { return "timeout" }
func (e *mockTimeoutError) Timeout() bool   { return true }
func (e *mockTimeoutError) Temporary() bool { return true }

// mockDialer is a dialer that returns mock connections.
type mockDialer struct {
	conn    *mockConn
	dialErr error
}

func newMockDialer(conn *mockConn) *mockDialer {
	return &mockDialer{conn: conn}
}

func (d *mockDialer) SetDialError(err error) {
	d.dialErr = err
}
