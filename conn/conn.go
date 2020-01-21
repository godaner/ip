package conn

import (
	"net"
	"sync"
)

// IPConn
type IPConn struct {
	net.Conn
	isClose chan int
	sync.Mutex
}

func NewIPConn(conn net.Conn) *IPConn {
	return &IPConn{
		Conn: conn,
	}
}

func (i *IPConn) Read(b []byte) (n int, err error) {
	n, err = i.Conn.Read(b)
	if err != nil && i.isClose != nil {
		i.close()
	}
	return n, err
}
func (i *IPConn) Write(b []byte) (n int, err error) {
	n, err = i.Conn.Write(b)
	if err != nil {
		i.close()
	}
	return n, err
}
func (i *IPConn) Close() error {
	err := i.Conn.Close()
	i.close()
	return err
}

func (i *IPConn) IsClose() (c chan int) {
	if i.isClose == nil {
		i.isClose = make(chan int)
	}
	return i.isClose
}

func (i *IPConn) close() {
	if i.isClose == nil {
		return
	}
	i.Lock()
	select {
	case <-i.isClose:
	default:
		close(i.isClose)
	}
	i.Unlock()
}
