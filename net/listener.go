package net

import (
	"log"
	"net"
	"sync"
)

// ListenerCloseHandler
type ListenerCloseHandler func(listener net.Listener)

// IPListener
//  wrap net.Listener
type IPListener struct {
	net.Listener
	isClose chan bool
	sync.Mutex
	closeHandlers []ListenerCloseHandler
}

func NewIPListener(lis net.Listener) *IPListener {
	return &IPListener{
		Listener: lis,
	}
}

// Accept
//  return IPConn
func (l *IPListener) Accept() (conn net.Conn, err error) {
	conn, err = l.Listener.Accept()
	if err != nil {
		l.close()
	}
	return NewIPConn(conn), err
}
func (l *IPListener) Close() (err error) {
	err = l.Listener.Close()
	if err != nil {
		l.close()
	}
	return err
}
func (l *IPListener) IsClose() (c chan bool) {
	if l.isClose == nil {
		l.isClose = make(chan bool)
	}
	return l.isClose
}
func (l *IPListener) SetCloseHandler(closeHandler ListenerCloseHandler) {
	if len(l.closeHandlers) <= 0 {
		l.closeHandlers = []ListenerCloseHandler{}
	}
	l.closeHandlers = append(l.closeHandlers, closeHandler)
	go func() {
		select {
		case <-l.IsClose():
			closeHandler(l)
		}
	}()

}
func (l *IPListener) SetCloseTrigger(triggers ...chan bool) {
	for _, t := range triggers {
		tri := t
		go func() {
			select {
			case <-l.IsClose():
				return
			case <-tri:
				err := l.Close()
				if err != nil {
					log.Printf("IPListener#SetCloseTrigger : close listener err , err is : %v !", err.Error())
				}
				return
			}
		}()
	}
}
func (l *IPListener) close() {
	l.Lock()
	defer l.Unlock()
	if l.isClose == nil {
		return
	}
	select {
	case <-l.isClose:
	default:
		close(l.isClose)
	}
}
