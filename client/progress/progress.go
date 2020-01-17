package progress

import (
	"github.com/godaner/ip/client/config"
	"github.com/godaner/ip/ipp"
	"github.com/godaner/ip/ipp/ippnew"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
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
	f, err := os.OpenFile("./ipclient.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Println("Progress#Listen : open file err :", err.Error())
		return
	}
	log.SetOutput(f)
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
		bs := make([]byte, 10240, 10240)
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
		cID := m.CID()
		switch m.Type() {
		case ipp.MSG_TYPE_HELLO:
			log.Printf("Progress#fromClientConnHandler : receive proxy hello , cID is : %v !", cID)
		case ipp.MSG_TYPE_CONN_CREATE:
			log.Printf("Progress#fromClientConnHandler : receive proxy conn create , cID is : %v !", cID)
			go p.proxyCreateBrowserConnHandler(cID)
		case ipp.MSG_TYPE_CONN_CLOSE:
			log.Printf("Progress#fromClientConnHandler : receive proxy conn close , cID is : %v !", cID)
			go func() {
				p.proxyCloseBrowserConnHandler(cID)
			}()
		case ipp.MSG_TYPE_REQ:
			log.Printf("Progress#fromProxyHandler : receive proxy req , cID is : %v !", cID)
			// receive proxy req info , we should dispatch the info
			go p.fromProxyReqHandler(m)

		default:
			log.Println("Progress#fromProxyHandler : receive proxy msg , but can't find type !")
		}
	}

}

// fromProxyReqHandler
func (p *Progress) fromProxyReqHandler(m ipp.Message) {
	cID := m.CID()
	b := m.AttributeByType(ipp.ATTR_TYPE_BODY)
	log.Printf("Progress#fromProxyHandler : receive proxy req , cID is : %v , body is : %v , len is : %v !", cID, string(b), len(b))
	if len(b) <= 0 {
		return
	}
	// some times the conn create , and proxy send req , but the forward conn is not ok.
	// we will wait the dial 5s
	var forwardConn net.Conn
	c := 0
	for {
		if c > 100 {
			break
		}
		if forwardConn != nil {
			break
		}
		v, ok := p.ForwardConnRID.Load(cID)
		if !ok {
			continue
		}
		forwardConn, _ = v.(net.Conn)
		if forwardConn == nil {
			continue
		}
		c++
		time.Sleep(50 * time.Millisecond)
	}
	if forwardConn == nil {
		log.Printf("Progress#fromProxyHandler : receive proxy req but no forward conn find , cID is : %v !", cID)
		return
	}
	n, err := forwardConn.Write(b)
	if err != nil {
		log.Printf("Progress#fromProxyHandler : receive proxy req , cID is : %v , forward err , err : %v !", cID, err)
	}
	log.Printf("Progress#fromProxyHandler : from proxy to forward , cID is : %v , msg is : %v , len is : %v !", cID, string(b), n)
}

// proxyCreateBrowserConnHandler
//  处理proxy回复的conn_create信息
func (p *Progress) proxyCreateBrowserConnHandler(cID uint16) {
	// proxy return browser conn create , we should dial forward addr
	addr := p.Config.ClientForwardAddr
	forwardConn, err := net.Dial("tcp", addr)
	if err != nil {
		// if dial fail , tell proxy to close browser conn
		log.Printf("Progress#proxyCreateBrowserConnHandler : after get proxy browser conn create , dial forward err , cID is : %v , err is : %v !", cID, err)
		p.sendForwardConnCloseEvent(cID)
		return
	}
	p.ForwardConnRID.Store(cID, forwardConn)
	log.Printf("Progress#proxyCreateBrowserConnHandler : dial forward addr success , cID is : %v , forward address is : %v !", cID, forwardConn.RemoteAddr())
	for {
		log.Printf("Progress#proxyCreateBrowserConnHandler : wait receive forward msg , cID is : %v !", cID)
		bs := make([]byte, 10240, 10240)
		n, err := forwardConn.Read(bs)
		if err != nil {
			log.Printf("Progress#proxyCreateBrowserConnHandler : read forward data err , cID is : %v , err is : %v !", cID, err)
			// lost connection , notify proxy
			_, ok := p.ForwardConnRID.Load(cID)
			if ok {
				p.ForwardConnRID.Delete(cID)
				p.sendForwardConnCloseEvent(cID)
			}
			return
		}
		log.Printf("Progress#proxyCreateBrowserConnHandler : receive forward msg , cID is : %v , msg is : %v , len is : %v !", cID, string(bs[0:n]), n)
		if n <= 0 {
			return
		}
		m := ippnew.NewMessage(p.Config.IPPVersion)
		m.ForReq(bs[0:n], cID)
		_, err = p.ProxyConn.Write(m.Marshall())
		if err != nil {
			log.Printf("Progress#proxyCreateBrowserConnHandler : write forward's data to proxy err , cID is : %v , err is : %v !", cID, err.Error())
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
		log.Printf("Progress#sendForwardConnCloseEvent : notify proxy conn close err , cID is : %v , err is : %v !", cID, err.Error())
		return false
	}
	return true
}

// proxyCloseBrowserConnHandler
func (p *Progress) proxyCloseBrowserConnHandler(cID uint16) {
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
		log.Printf("Progress#proxyCloseBrowserConnHandler : close forward conn err , cID is : %v , err : %v !", cID, err.Error())
	}
}

//产生随机序列号
func (p *Progress) newSerialNo() uint16 {
	rand.Seed(time.Now().UnixNano())
	r := rand.Intn(math.MaxUint16)
	return uint16(r)
}
