package net

import (
	"log"
	"net"
	"sync"
)

// ConnCloseHandler
type ConnCloseHandler func(conn net.Conn)

// IPConn
//  wrap net.Conn
type IPConn struct {
	net.Conn
	isClose chan bool
	sync.Mutex
	closeHandlers []ConnCloseHandler
}

func NewIPConn(conn net.Conn) *IPConn {
	return &IPConn{
		Conn: conn,
	}
}

func (i *IPConn) Read(b []byte) (n int, err error) {
	n, err = i.Conn.Read(b)
	if err != nil {
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

func (i *IPConn) IsClose() (c chan bool) {
	if i.isClose == nil {
		i.isClose = make(chan bool)
	}
	return i.isClose
}
func (i *IPConn) SetCloseHandler(closeHandler ConnCloseHandler) {
	if len(i.closeHandlers) <= 0 {
		i.closeHandlers = []ConnCloseHandler{}
	}
	i.closeHandlers = append(i.closeHandlers, closeHandler)
	go func() {
		select {
		case <-i.IsClose():
			closeHandler(i)
		}
	}()

}
func (i *IPConn) SetCloseTrigger(triggers ...chan bool) {
	for _, t := range triggers {
		tri := t
		go func() {
			select {
			case <-i.IsClose():
				return
			case <-tri:
				err := i.Close()
				if err != nil {
					log.Printf("IPConn#SetCloseTrigger : close conn err , err is : %v !", err.Error())
				}
				return
			}
		}()
	}
}
func (i *IPConn) close() {
	i.Lock()
	defer i.Unlock()
	if i.isClose == nil {
		return
	}
	select {
	case <-i.isClose:
	default:
		close(i.isClose)
	}
}
