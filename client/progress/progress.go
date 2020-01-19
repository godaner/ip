package progress

import (
	"encoding/binary"
	"github.com/godaner/ip/client/config"
	"github.com/godaner/ip/ipp"
	"github.com/godaner/ip/ipp/ippnew"
	"io"
	"log"
	"math"
	"math/rand"
	"net"
	"sync"
	"time"
)

const (
	restart_interval = 5
)

type Progress struct {
	ProxyConn      net.Conn
	Config         *config.Config
	RestartSignal  chan int
	ForwardConnRID sync.Map // map[uint16]net.Conn
}

func (p *Progress) Listen() (err error) {
	c := new(config.Config)
	err = c.Load()
	if err != nil {
		return err
	}
	//p.CID = p.newSerialNo()
	p.Config = c
	p.RestartSignal = make(chan int)

	// log
	log.SetFlags(log.Lmicroseconds)
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
func (p *Progress) setRestartSignal() {
	p.RestartSignal <- 1
}
func (p *Progress) listenProxy() {
	// when restart , we should close all forward conn
	p.ForwardConnRID.Range(func(key, value interface{}) bool {
		p.ForwardConnRID.Delete(key)
		forwardConn, _ := value.(net.Conn)
		addr := ""
		if forwardConn != nil {
			addr = forwardConn.RemoteAddr().String()
			forwardConn.Close()
		}
		log.Printf("Progress#listenProxy : re listen proxy , we need close the forward conn , cID is : %v , currt forward addr is : %v ! ", key, addr)
		return true
	})
	p.ForwardConnRID = sync.Map{} //map[uint16]net.Conn{}
	addr := p.Config.ProxyAddr
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Printf("Progress#Listen : dial proxy addr err , err is : %v !", err)
		p.setRestartSignal()
		return
	}
	log.Printf("Progress#Listen : dial proxy addr is : %v !", addr)

	p.ProxyConn = conn
	// listen proxy return msg
	go func() {
		p.fromProxyHandler()
	}()
	// say hello to proxy
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForHelloReq([]byte(p.Config.ClientWannaProxyPort), 0, p.newSerialNo())
	// marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err = p.ProxyConn.Write(b)
	if err != nil {
		p.setRestartSignal()
		log.Printf("Progress#Listen : say hello to proxy err , err : %v !", err)
	}
}

// fromProxyHandler
//  监听proxy返回的消息
func (p *Progress) fromProxyHandler() {
	for {
		// parse protocol
		sID := p.newSerialNo()
		length := make([]byte, 4, 4)
		n, err := p.ProxyConn.Read(length)
		if err != nil {
			log.Printf("Progress#fromProxyHandler : receive proxy ipp len err , err is : %v !", err)
			p.setRestartSignal()
			return
		}
		ippLength := binary.BigEndian.Uint32(length)
		log.Printf("Progress#fromProxyHandler : receive info from proxy ippLength is : %v !", ippLength)
		bs := make([]byte, ippLength, ippLength)
		n, err = io.ReadFull(p.ProxyConn, bs)
		if err != nil {
			log.Printf("Progress#fromProxyHandler : receive proxy err , err is : %v !", err)
			p.setRestartSignal()
			return
		}
		log.Printf("Progress#fromProxyHandler : receive proxy msg , ipp len is : %v !", n)
		//if n <= 0 {
		//	continue
		//}
		m := ippnew.NewMessage(p.Config.IPPVersion)
		m.UnMarshall(bs)
		cID := m.CID()
		switch m.Type() {
		case ipp.MSG_TYPE_HELLO:
			log.Printf("Progress#fromProxyHandler : receive proxy hello , cID is : %v , sID is : %v !", cID, sID)
		case ipp.MSG_TYPE_CONN_CREATE:
			log.Printf("Progress#fromProxyHandler : receive proxy conn create , cID is : %v , sID is : %v !", cID, sID)
			go p.proxyCreateBrowserConnHandler(cID, sID)
		case ipp.MSG_TYPE_CONN_CLOSE:
			log.Printf("Progress#fromProxyHandler : receive proxy conn close , cID is : %v , sID is : %v !", cID, sID)
			p.proxyCloseBrowserConnHandler(cID, sID)
		case ipp.MSG_TYPE_REQ:
			log.Printf("Progress#fromProxyHandler : receive proxy req , cID is : %v , sID is : %v !", cID, sID)
			// receive proxy req info , we should dispatch the info
			p.fromProxyReqHandler(m)

		default:
			log.Println("Progress#fromProxyHandler : receive proxy msg , but can't find type !")
		}
	}

}

// fromProxyReqHandler
func (p *Progress) fromProxyReqHandler(m ipp.Message) {
	cID := m.CID()
	sID := m.SerialId()
	b := m.AttributeByType(ipp.ATTR_TYPE_BODY)
	log.Printf("Progress#fromProxyHandler : receive proxy req , cID is : %v , sID is : %v , len is : %v !", cID, sID, len(b))
	//if len(b) <= 0 {
	//	return
	//}
	// some times the conn create , and proxy send req , but the forward conn is not ok.
	// we will wait the dial 5s
	//var forwardConn net.Conn
	//c := 0
	//for {
	//	if c > 100 {
	//		break
	//	}
	//	if forwardConn != nil {
	//		break
	//	}
	//	v, ok := p.ForwardConnRID.Load(cID)
	//	if !ok {
	//		continue
	//	}
	//	forwardConn, _ = v.(net.Conn)
	//	if forwardConn == nil {
	//		continue
	//	}
	//	c++
	//	time.Sleep(50 * time.Millisecond)
	//}
	v, ok := p.ForwardConnRID.Load(cID)
	if !ok {
		log.Printf("Progress#fromProxyHandler : receive proxy req but no forward conn find , not ok , cID is : %v , sID is : %v !", cID, sID)
		return
	}
	forwardConn, _ := v.(net.Conn)
	if forwardConn == nil {
		log.Printf("Progress#fromProxyHandler : receive proxy req but no forward conn find , forwardConnis nil , cID is : %v , sID is : %v !", cID, sID)
		return
	}
	n, err := forwardConn.Write(b)
	if err != nil {
		log.Printf("Progress#fromProxyHandler : receive proxy req , cID is : %v , sID is : %v , forward err , err : %v !", cID, sID, err)
	}
	log.Printf("Progress#fromProxyHandler : from proxy to forward , cID is : %v , sID is : %v , len is : %v !", cID, sID, n)
}
func (p *Progress) sendCreateConnDoneEvent(cID, sID uint16) (success bool) {
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForConnCreateDone([]byte{}, cID, sID)
	//marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err := p.ProxyConn.Write(b)
	if err != nil {
		p.setRestartSignal()
		log.Printf("Progress#sendForwardConnCloseEvent : notify proxy conn close err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err.Error())
		return false
	}
	return true
}

// proxyCreateBrowserConnHandler
//  处理proxy回复的conn_create信息
func (p *Progress) proxyCreateBrowserConnHandler(cID, sID uint16) {
	// proxy return browser conn create , we should dial forward addr
	addr := p.Config.ClientForwardAddr
	forwardConn, err := net.Dial("tcp", addr)
	if err != nil {
		// if dial fail , tell proxy to close browser conn
		log.Printf("Progress#proxyCreateBrowserConnHandler : after get proxy browser conn create , dial forward err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err)
		p.sendForwardConnCloseEvent(cID, sID)
		return
	}

	p.ForwardConnRID.Store(cID, forwardConn)
	log.Printf("Progress#proxyCreateBrowserConnHandler : dial forward addr success , cID is : %v , sID is : %v , forward local address is : %v , forward remote address is : %v !", cID, sID, forwardConn.LocalAddr(), forwardConn.RemoteAddr())
	p.sendCreateConnDoneEvent(cID, sID)
	bs := make([]byte, 1024, 1024)
	for {
		log.Printf("Progress#proxyCreateBrowserConnHandler : wait receive forward msg , cID is : %v , sID is : %v !", cID, sID)
		sID = p.newSerialNo()
		n, err := forwardConn.Read(bs)
		if err != nil {
			log.Printf("Progress#proxyCreateBrowserConnHandler : read forward data err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err)
			// lost connection , notify proxy
			_, ok := p.ForwardConnRID.Load(cID)
			if ok {
				p.ForwardConnRID.Delete(cID)
				p.sendForwardConnCloseEvent(cID, sID)
			}
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
			p.setRestartSignal()
			return
		}
		log.Printf("Progress#proxyCreateBrowserConnHandler : from client to proxy msg , cID is : %v , sID is : %v , len is : %v !", cID, sID, n)

	}
}
func (p *Progress) sendForwardConnCloseEvent(cID, sID uint16) (success bool) {
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForConnClose([]byte{}, cID, sID)
	//marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err := p.ProxyConn.Write(b)
	if err != nil {
		p.setRestartSignal()
		log.Printf("Progress#sendForwardConnCloseEvent : notify proxy conn close err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err.Error())
		return false
	}
	return true
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

//产生随机序列号
func (p *Progress) newSerialNo() uint16 {
	rand.Seed(time.Now().UnixNano())
	r := rand.Intn(math.MaxUint16)
	return uint16(r)
}
