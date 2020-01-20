package progress

import (
	"encoding/binary"
	"github.com/godaner/ip/client/config"
	"github.com/godaner/ip/conn"
	"github.com/godaner/ip/ipp"
	"github.com/godaner/ip/ipp/ippnew"
	"io"
	"log"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

const (
	restart_interval = 5
)

type Progress struct {
	ProxyConn      *conn.IPConn
	Config         *config.Config
	RestartSignal  chan int
	ForwardConnRID sync.Map // map[uint16]net.Conn
	seq            int32
}

func (p *Progress) Listen() (err error) {
	// log
	log.SetFlags(log.Lmicroseconds)

	c := new(config.Config)
	err = c.Load()
	if err != nil {
		return err
	}
	//p.CID = p.newSerialNo()
	p.Config = c
	p.RestartSignal = make(chan int)
	// proxy conn
	go func() {
		for {
			select {
			case <-p.RestartSignal:
				log.Println("Progress#Listen : we will start the client !")
				go func() {
					p.listenProxy()
				}()
			default:
			}
			time.Sleep(restart_interval * time.Second)
		}
	}()
	p.setRestartSignal()
	return nil
}

// setRestartSignal
//  restart the client
func (p *Progress) setRestartSignal() {
	select {
	case <-p.RestartSignal:
	default:
		close(p.RestartSignal)
	}
}
func (p *Progress) listenProxy() {
	//// init var ////
	// reset restart signal
	p.RestartSignal = make(chan int)
	p.ForwardConnRID = sync.Map{}

	//// dial proxy conn ////
	addr := p.Config.ProxyAddr
	c, err := net.Dial("tcp", addr)
	if err != nil {
		log.Printf("Progress#Listen : dial proxy addr err , err is : %v !", err)
		p.setRestartSignal()
		return
	}
	p.ProxyConn = conn.NewIPConn(c)
	// check proxy conn
	go func() {
		<-p.ProxyConn.IsClose()
		log.Println("Progress#Listen : proxy conn is close !")
		p.setRestartSignal()
	}()
	log.Printf("Progress#Listen : dial proxy addr is : %v !", addr)

	//// receive proxy msg ////
	go func() {
		p.receiveProxyMsg()
	}()

	//// say hello to proxy ////
	cID := uint16(0)
	sID := p.newSerialNo()
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForHelloReq([]byte(p.Config.ClientWannaProxyPort), cID, sID)
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err = p.ProxyConn.Write(b)
	if err != nil {
		log.Printf("Progress#Listen : say hello to proxy err , cID is : %v , sID is : %v , err : %v !", cID, sID, err)
		return
	}
	log.Printf("Progress#Listen : say hello to proxy success , cID is : %v , sID is : %v , proxy addr is : %v !", cID, sID, addr)
}

// receiveProxyMsg
//  监听proxy返回的消息
func (p *Progress) receiveProxyMsg() {
	for {
		select {
		case <-p.RestartSignal:
			log.Println("Progress#receiveProxyMsg : get client restart signal , will stop read proxy conn !")
			return
		case <-p.ProxyConn.IsClose():
			log.Println("Progress#receiveProxyMsg : get proxy conn close signal , will stop read proxy conn !")
			return
		default:
			// parse protocol
			sID := p.newSerialNo()
			length := make([]byte, 4, 4)
			n, err := p.ProxyConn.Read(length)
			if err != nil {
				log.Printf("Progress#fromProxyHandler : receive proxy ipp len err , err is : %v !", err)
				return
			}
			ippLength := binary.BigEndian.Uint32(length)
			log.Printf("Progress#fromProxyHandler : receive info from proxy ippLength is : %v !", ippLength)
			bs := make([]byte, ippLength, ippLength)
			n, err = io.ReadFull(p.ProxyConn, bs)
			if err != nil {
				log.Printf("Progress#fromProxyHandler : receive proxy err , err is : %v !", err)
				return
			}
			log.Printf("Progress#fromProxyHandler : receive proxy msg , ipp len is : %v !", n)
			m := ippnew.NewMessage(p.Config.IPPVersion)
			m.UnMarshall(bs)
			cID := m.CID()
			// choose handler
			switch m.Type() {
			case ipp.MSG_TYPE_HELLO:
				log.Printf("Progress#fromProxyHandler : receive proxy hello , cID is : %v , sID is : %v !", cID, sID)
				p.clientProxyHello(m, cID, sID)
			case ipp.MSG_TYPE_CONN_CREATE:
				log.Printf("Progress#fromProxyHandler : receive proxy conn create , cID is : %v , sID is : %v !", cID, sID)
				go p.proxyCreateBrowserConnHandler(cID, sID)
			case ipp.MSG_TYPE_CONN_CLOSE:
				log.Printf("Progress#fromProxyHandler : receive proxy conn close , cID is : %v , sID is : %v !", cID, sID)
				p.proxyCloseBrowserConnHandler(cID, sID)
			case ipp.MSG_TYPE_REQ:
				log.Printf("Progress#fromProxyHandler : receive proxy req , cID is : %v , sID is : %v !", cID, sID)
				// receive proxy req info , we should dispatch the info
				p.proxyReqHandler(m)
			default:
				log.Println("Progress#fromProxyHandler : receive proxy msg , but can't find type !")
			}
		}
	}

}

// proxyReqHandler
func (p *Progress) proxyReqHandler(m ipp.Message) {
	cID := m.CID()
	sID := m.SerialId()
	b := m.AttributeByType(ipp.ATTR_TYPE_BODY)
	log.Printf("Progress#proxyReqHandler : receive proxy req , cID is : %v , sID is : %v , len is : %v !", cID, sID, len(b))
	v, ok := p.ForwardConnRID.Load(cID)
	if !ok {
		log.Printf("Progress#proxyReqHandler : receive proxy req but no forward conn find , not ok , cID is : %v , sID is : %v !", cID, sID)
		return
	}
	forwardConn, _ := v.(net.Conn)
	if forwardConn == nil {
		log.Printf("Progress#proxyReqHandler : receive proxy req but no forward conn find , forwardConnis nil , cID is : %v , sID is : %v !", cID, sID)
		return
	}
	n, err := forwardConn.Write(b)
	if err != nil {
		log.Printf("Progress#proxyReqHandler : receive proxy req , cID is : %v , sID is : %v , forward err , err : %v !", cID, sID, err)
	}
	log.Printf("Progress#proxyReqHandler : from proxy to forward , cID is : %v , sID is : %v , len is : %v !", cID, sID, n)
}

// proxyCreateBrowserConnHandler
//  处理proxy回复的conn_create信息
func (p *Progress) proxyCreateBrowserConnHandler(cID, sID uint16) {
	//// proxy return browser conn create , we should dial forward addr ////
	addr := p.Config.ClientForwardAddr
	c, err := net.Dial("tcp", addr)
	if err != nil {
		// if dial fail , tell proxy to close browser conn
		log.Printf("Progress#proxyCreateBrowserConnHandler : after get proxy browser conn create , dial forward err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err)
		p.sendForwardConnCloseEvent(cID, sID)
		return
	}
	forwardConn := conn.NewIPConn(c)
	// check forward conn
	go func() {
		<-forwardConn.IsClose()
		log.Printf("Progress#proxyCreateBrowserConnHandler : forward conn is close , cID is : %v , sID is : %v !", cID, sID)
		_, ok := p.ForwardConnRID.Load(cID)
		if !ok {
			return
		}
		p.ForwardConnRID.Delete(cID)
		p.sendForwardConnCloseEvent(cID, sID)
	}()
	p.ForwardConnRID.Store(cID, forwardConn)
	log.Printf("Progress#proxyCreateBrowserConnHandler : dial forward addr success , cID is : %v , sID is : %v , forward local address is : %v , forward remote address is : %v !", cID, sID, forwardConn.LocalAddr(), forwardConn.RemoteAddr())

	//// notify proxy ////
	p.sendCreateConnDoneEvent(cID, sID)

	//// read forward data ////
	bs := make([]byte, 1024, 1024)
	for {
		select {
		case <-p.RestartSignal:
			log.Printf("Progress#proxyCreateBrowserConnHandler : get client restart signal , will stop read forward conn , cID is : %v , sID is : %v !", cID, sID)
			return
		case <-forwardConn.IsClose():
			log.Printf("Progress#proxyCreateBrowserConnHandler : get forward conn close signal , will stop read forward conn , cID is : %v , sID is : %v !", cID, sID)
			return
		default:
			log.Printf("Progress#proxyCreateBrowserConnHandler : wait receive forward msg , cID is : %v , sID is : %v !", cID, sID)
			sID = p.newSerialNo()
			n, err := forwardConn.Read(bs)
			if err != nil {
				log.Printf("Progress#proxyCreateBrowserConnHandler : read forward data err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err)
				return
			}
			log.Printf("Progress#proxyCreateBrowserConnHandler : receive forward msg , cID is : %v , sID is : %v , len is : %v !", cID, sID, n)
			//if n <= 0 {
			//	return
			//}
			m := ippnew.NewMessage(p.Config.IPPVersion)
			m.ForReq(bs[0:n], cID, sID)
			//marshal
			b := m.Marshall()
			ippLen := make([]byte, 4, 4)
			binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
			b = append(ippLen, b...)
			n, err = p.ProxyConn.Write(b)
			if err != nil {
				log.Printf("Progress#proxyCreateBrowserConnHandler : write forward's data to proxy err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err.Error())
				return
			}
			log.Printf("Progress#proxyCreateBrowserConnHandler : from client to proxy msg , cID is : %v , sID is : %v , len is : %v !", cID, sID, n)

		}

	}
}
func (p *Progress) sendCreateConnDoneEvent(cID, sID uint16) {
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForConnCreateDone([]byte{}, cID, sID)
	//marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err := p.ProxyConn.Write(b)
	if err != nil {
		log.Printf("Progress#sendForwardConnCloseEvent : notify proxy conn close err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err.Error())
		return
	}
	return
}
func (p *Progress) sendForwardConnCloseEvent(cID, sID uint16) {
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForConnClose([]byte{}, cID, sID)
	//marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err := p.ProxyConn.Write(b)
	if err != nil {
		log.Printf("Progress#sendForwardConnCloseEvent : notify proxy conn close err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err.Error())
		return
	}
	return
}

// proxyCloseBrowserConnHandler
func (p *Progress) proxyCloseBrowserConnHandler(cID, sID uint16) {
	v, ok := p.ForwardConnRID.Load(cID)
	if !ok {
		return
	}
	c, _ := v.(net.Conn)
	if c == nil {
		return
	}
	p.ForwardConnRID.Delete(cID)
	err := c.Close()
	if err != nil {
		log.Printf("Progress#proxyCloseBrowserConnHandler : close forward conn err , cID is : %v , sID is : %v , err : %v !", cID, sID, err.Error())
	}
}

func (p *Progress) clientProxyHello(m ipp.Message, cID uint16, sID uint16) {
	port := string(m.AttributeByType(ipp.ATTR_TYPE_PORT))
	if port == "" {
		log.Printf("Progress#clientProxyHello : maybe listen port in porxy side be occupied , cID is : %v , sID is : %v !", cID, sID)
		p.setRestartSignal()
		return
	}
	log.Printf("Progress#clientProxyHello : listen port success in porxy side , port is : %v , cID is : %v , sID is : %v !", port, cID, sID)
	return
}

//产生随机序列号
func (p *Progress) newSerialNo() uint16 {
	atomic.CompareAndSwapInt32(&p.seq, math.MaxUint16, 0)
	atomic.AddInt32(&p.seq, 1)
	return uint16(p.seq)
}
