package net

import (
	"log"
	"net"
	"sync"
	"time"
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
	sync.Once
	hbTimer    *time.Timer
	hbInterval time.Duration
}

func (i *IPConn) init() {
	i.Do(func() {
		i.isClose = make(chan bool)
	})
}
func NewIPConn(conn net.Conn) *IPConn {
	return &IPConn{
		Conn: conn,
	}
}
func (i *IPConn) SetHeartBeatInterval(hbInterval time.Duration) {
	i.init()
	i.hbInterval = hbInterval
	i.hbTimer = time.NewTimer(i.hbInterval)
}
func (i *IPConn) GetHeartBeatTimer() (t *time.Timer) {
	i.init()
	return i.hbTimer
}
func (i *IPConn) ResetHeartBeatTimer() {
	i.init()
	i.hbTimer.Reset(i.hbInterval)
}
func (i *IPConn) Read(b []byte) (n int, err error) {
	i.init()
	n, err = i.Conn.Read(b)
	if err != nil {
		i.omitCloseSignal()
	}
	return n, err
}
func (i *IPConn) Write(b []byte) (n int, err error) {
	i.init()
	n, err = i.Conn.Write(b)
	if err != nil {
		i.omitCloseSignal()
	}
	return n, err
}
func (i *IPConn) Close() error {
	i.init()
	err := i.Conn.Close()
	i.omitCloseSignal()
	return err
}

func (i *IPConn) CloseSignal() (c chan bool) {
	i.init()
	return i.isClose
}

type ConnCloseTrigger struct {
	Signal  chan bool
	Handler ConnCloseHandler
}

func (i *IPConn) AddCloseTrigger(selfCloseHandler ConnCloseHandler, triggers ...*ConnCloseTrigger) {
	i.init()
	over := make(chan bool)
	go func() {
		select {
		case <-over:
			return
		case <-i.CloseSignal():
			select {
			case <-over:
			default:
				close(over)
			}
			if selfCloseHandler != nil {
				selfCloseHandler(i)
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
					tri.Handler(i)
				}
				err := i.Close()
				if err != nil {
					log.Printf("IPConn#SetCloseTrigger : close conn err , err is : %v !", err.Error())
				}
				return
			}
		}()
	}
}
func (i *IPConn) omitCloseSignal() {
	i.init()
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
