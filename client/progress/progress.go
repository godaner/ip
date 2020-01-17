package progress

import (
	"github.com/godaner/ip/client/config"
	"github.com/godaner/ip/ipp"
	"github.com/godaner/ip/ipp/ippnew"
	"log"
	"math"
	"math/rand"
	"net"
	"time"
)

const (
	restart_interval = 5
)

type Progress struct {
	//ClientForwardConn net.Conn
	ProxyConn net.Conn
	//CID               uint16
	Config         *config.Config
	RestartSignal  chan int
	ForwardConnRID map[uint16]net.Conn
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
	p.ForwardConnRID = map[uint16]net.Conn{}
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
	m.ForHelloReq([]byte(p.Config.ClientWannaProxyPort), 0)
	_, err = p.ProxyConn.Write(m.Marshall())
	if err != nil {
		log.Printf("Progress#Listen : say hello to proxy err , err : %v !", err)
	}
}

// fromProxyHandler
//  监听proxy返回的消息
func (p *Progress) fromProxyHandler() {
	for {
		// parse protocol
		bs := make([]byte, 4096, 4096)
		n, err := p.ProxyConn.Read(bs)
		if err != nil {
			log.Printf("Progress#fromClientConnHandler : receive proxy err , err is : %v !", err)
			p.setRestartSignal()
			return
		}
		s := bs[0:n]
		log.Printf("Progress#fromProxyHandler : receive proxy msg , msg is : %v , len is : %v !", string(s), n)
		if n <= 0 {
			continue
		}
		m := ippnew.NewMessage(p.Config.IPPVersion)
		m.UnMarshall(s)
		switch m.Type() {
		case ipp.MSG_TYPE_HELLO:
			log.Println("Progress#fromClientConnHandler : receive proxy hello !")
		case ipp.MSG_TYPE_CONN_CREATE:
			log.Println("Progress#fromClientConnHandler : receive proxy conn create !")
			go p.proxyCreateBrowserConnHandler(m.CID())
		case ipp.MSG_TYPE_CONN_CLOSE:
			log.Println("Progress#fromClientConnHandler : receive proxy conn close !")
			go func() {
				cID:=m.CID()
				p.proxyCloseBrowserConnHandler(cID)
				delete(p.ForwardConnRID, cID)
			}()
		case ipp.MSG_TYPE_REQ:
			log.Println("Progress#fromProxyHandler : receive proxy req !")
			// receive proxy req info , we should dispatch the info
			b := m.AttributeByType(ipp.ATTR_TYPE_BODY)
			log.Printf("Progress#fromProxyHandler : receive proxy req , body is : %v , len is : %v !", string(b), len(b))
			forwardConn := p.ForwardConnRID[m.CID()]
			if forwardConn != nil {
				n, err := forwardConn.Write(b)
				if err != nil {
					log.Printf("Progress#fromProxyHandler : receive proxy req , forward err , err : %v !", err)
				}
				log.Printf("Progress#fromProxyHandler : from proxy to forward , msg is : %v , len is : %v !", string(b), n)
			}
		default:
			log.Println("Progress#fromProxyHandler : receive proxy msg , but can't find type !")
		}
	}

}

// proxyCreateBrowserConnHandler
//  处理proxy回复的conn_create信息
func (p *Progress) proxyCreateBrowserConnHandler(cID uint16) {
	// proxy return browser conn create , we should dial forward addr
	addr := p.Config.ClientForwardAddr
	forwardConn, err := net.Dial("tcp", addr)
	if err != nil {
		// if dial fail , tell proxy to close browser conn
		log.Printf("Progress#proxyCreateBrowserConnHandler : after get proxy browser conn create , dial forward err , err is : %v !", err)
		p.sendForwardConnCloseEvent(cID)
		return
	}
	p.ForwardConnRID[cID] = forwardConn
	log.Printf("Progress#proxyCreateBrowserConnHandler : dial forward addr success , forward address is : %v !", forwardConn.RemoteAddr())
	for {
		log.Println("Progress#proxyCreateBrowserConnHandler : wait receive forward msg !")
		bs := make([]byte, 4096, 4096)
		n, err := forwardConn.Read(bs)
		if err != nil {
			log.Printf("Progress#proxyCreateBrowserConnHandler : read forward data err , err is : %v !", err)
			// lost connection , notify proxy
			_, ok := p.ForwardConnRID[cID]
			if ok {
				delete(p.ForwardConnRID, cID)
				p.sendForwardConnCloseEvent(cID)
			}
			return
		}
		log.Printf("Progress#proxyCreateBrowserConnHandler : receive forward msg , msg is : %v , len is : %v !", string(bs[0:n]), n)
		m := ippnew.NewMessage(p.Config.IPPVersion)
		m.ForReq(bs[0:n], cID)
		_, err = p.ProxyConn.Write(m.Marshall())
		if err != nil {
			log.Printf("Progress#proxyCreateBrowserConnHandler : write forward's data to proxy is : %v !", err.Error())
			p.setRestartSignal()
			return
		}
	}
}
func (p *Progress) sendForwardConnCloseEvent(cID uint16) (success bool) {
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForConnClose([]byte{}, cID)
	_, err := p.ProxyConn.Write(m.Marshall())
	if err != nil {
		log.Printf("Progress#sendForwardConnCloseEvent : notify proxy conn close err , err is : %v !", err.Error())
		return false
	}
	return true
}

// proxyCloseBrowserConnHandler
func (p *Progress) proxyCloseBrowserConnHandler(cID uint16) {
	c, _ := p.ForwardConnRID[cID]
	if c == nil {
		return
	}
	err := c.Close()
	if err != nil {
		log.Printf("Progress#proxyCloseBrowserConnHandler : close forward conn err , err : %v !", err.Error())
	}
}

//产生随机序列号
func (p *Progress) newSerialNo() uint16 {
	rand.Seed(time.Now().UnixNano())
	r := rand.Intn(math.MaxUint16)
	return uint16(r)
}
