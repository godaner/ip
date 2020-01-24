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
	sync.Once
	closeHandlers []ListenerCloseHandler
}

func NewIPListener(lis net.Listener) *IPListener {
	return &IPListener{
		Listener: lis,
	}
}
func (l *IPListener) Accept() (conn net.Conn, err error) {
	conn, err = l.Listener.Accept()
	if err != nil {
		l.omitCloseSignal()
	}
	return NewIPConn(conn), err
}
func (l *IPListener) init() {
	l.Do(func() {
		l.isClose = make(chan bool)
	})
}

// Accept
//  return IPConn
func (l *IPListener) Close() (err error) {
	err = l.Listener.Close()
	if err != nil {
		l.omitCloseSignal()
	}
	return err
}

func (l *IPListener) CloseSignal() (c chan bool) {
	l.init()
	return l.isClose
}

type ListenerCloseTrigger struct {
	Signal  chan bool
	Handler ListenerCloseHandler
}

func (l *IPListener) AddCloseTrigger(selfCloseHandler ListenerCloseHandler, triggers ...*ListenerCloseTrigger) {
	l.init()
	over := make(chan bool)
	go func() {
		select {
		case <-over:
			return
		case <-l.CloseSignal():
			select {
			case <-over:
			default:
				close(over)
			}
			if selfCloseHandler != nil {
				selfCloseHandler(l)
			}
		}
	}()
	for _, t := range triggers {
		tri := t
		go func() {
			select {
			case <-over:
				return
			case <-tri.Signal:
				select {
				case <-over:
				default:
					close(over)
				}
				if tri.Handler != nil {
					tri.Handler(l)
				}
				err := l.Close()
				if err != nil {
					log.Printf("IPListener#SetCloseTrigger : close conn err , err is : %v !", err.Error())
				}
				return
			}
		}()
	}
}
func (l *IPListener) omitCloseSignal() {
	l.init()
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
