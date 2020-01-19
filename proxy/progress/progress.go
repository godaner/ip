package progress

import (
	"encoding/binary"
	"github.com/godaner/ip/ipp"
	"github.com/godaner/ip/ipp/ippnew"
	"github.com/godaner/ip/proxy/config"
	"log"
	"math"
	"math/rand"
	"net"
	"sync"
	"time"
)

type Progress struct {
	Config                *config.Config
	BrowserConnRID        sync.Map // map[uint16]net.Conn
	ClientWannaProxyPorts sync.Map // map[string]net.Listener
}

func (p *Progress) Listen() (err error) {
	c := new(config.Config)
	err = c.Load()
	if err != nil {
		return err
	}
	p.Config = c
	p.ClientWannaProxyPorts = sync.Map{} // map[string]net.Listener{}
	p.BrowserConnRID = sync.Map{}        // map[uint16]net.Conn{}
	// log
	log.SetFlags(log.Lmicroseconds)
	// from client conn
	go func() {
		addr := ":" + c.LocalPort
		l, err := net.Listen("tcp", addr)
		if err != nil {
			panic(err)
		}
		log.Printf("Progress#Listen : local addr is : %v !", addr)
		go p.fromClientConnHandler(l)
	}()

	return nil
}
func (p *Progress) fromClientConnHandler(l net.Listener) {
	for {
		clientConn, err := l.Accept()
		if err != nil {
			panic(err)
		}
		go func() {
			for {
				// parse protocol
				length := make([]byte, 4, 4)
				n, err := clientConn.Read(length)
				if err != nil {
					log.Printf("Progress#fromClientConnHandler : read ipp len info from client err , err is : %v !", err.Error())
					// close client connection , wait client reconnect
					//clientConn.Close()
					break
				}
				ippLength := binary.BigEndian.Uint32(length)
				log.Printf("Progress#fromClientConnHandler : read info from client ippLength is : %v !", ippLength)
				bs := make([]byte, ippLength, ippLength)
				n, err = clientConn.Read(bs)
				if err != nil {
					log.Printf("Progress#fromClientConnHandler : read info from client err , err is : %v !", err.Error())
					// close client connection , wait client reconnect
					//clientConn.Close()
					break
				}
				m := ippnew.NewMessage(p.Config.IPPVersion)
				m.UnMarshall(bs[0:n])
				cID := m.CID()
				sID := m.SerialId()
				switch m.Type() {
				case ipp.MSG_TYPE_HELLO:
					log.Printf("Progress#fromClientConnHandler : receive client hello , cID is : %v , sID is : %v !", cID, sID)
					// receive client hello , we should listen the client_wanna_proxy_port , and dispatch browser data to this client.
					clientWannaProxyPort := string(m.AttributeByType(ipp.ATTR_TYPE_PORT))
					go p.clientHelloHandler(clientConn, clientWannaProxyPort, sID)
				case ipp.MSG_TYPE_CONN_CREATE_DONE:
					log.Printf("Progress#fromClientConnHandler : receive client conn create done , cID is : %v , sID is : %v !", cID, sID)
					go p.clientConnCreateDoneHandler(clientConn, cID, sID)
				case ipp.MSG_TYPE_CONN_CLOSE:
					log.Printf("Progress#fromClientConnHandler : receive client conn close , cID is : %v , sID is : %v !", cID, sID)
					v, ok := p.BrowserConnRID.Load(cID)
					if !ok {
						continue
					}
					browserConn, _ := v.(net.Conn)
					if browserConn == nil {
						continue
					}
					p.BrowserConnRID.Delete(cID)
					err := browserConn.Close()
					if err != nil {
						log.Printf("Progress#fromClientConnHandler : after receive client conn close , close browser conn err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err.Error())
					}
				case ipp.MSG_TYPE_REQ:
					log.Printf("Progress#fromClientConnHandler : receive client req , cID is : %v , sID is : %v !", cID, sID)
					// receive client req , we should judge the client port , and dispatch the data to all browser who connect to this port.
					v, ok := p.BrowserConnRID.Load(cID)
					if !ok {
						continue
					}
					browserConn, _ := v.(net.Conn)
					if browserConn == nil {
						continue
					}
					data := m.AttributeByType(ipp.ATTR_TYPE_BODY)
					//if len(data) <= 0 {
					//	return
					//}
					_, err := browserConn.Write(data)
					if err != nil {
						log.Printf("Progress#fromClientConnHandler : from client to browser err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err.Error())
						_, ok := p.BrowserConnRID.Load(cID)
						if ok {
							p.sendBrowserConnCloseEvent(clientConn, cID, sID)
							p.BrowserConnRID.Delete(cID)
						}

					}
					log.Printf("Progress#fromClientConnHandler : from client to browser success , cID is : %v , sID is : %v , data is : %v , data len is : %v !", cID, sID, string(data), len(data))
				}
			}
		}()
	}

}
func (p *Progress) ListenClientWannaProxyPort(clientWannaProxyPort string) (l net.Listener, err error) {
	v, ok := p.ClientWannaProxyPorts.Load(clientWannaProxyPort)
	l, _ = v.(net.Listener)
	if ok && l != nil {
		err := l.Close()
		if err != nil {
			log.Printf("Progress#ListenClientWannaProxyPort : close port listener err , err is : %v !", err.Error())
		}
		p.ClientWannaProxyPorts.Delete(clientWannaProxyPort)
	}
	addr := ":" + clientWannaProxyPort
	l, err = net.Listen("tcp", addr)
	if err != nil {
		log.Printf("Progress#ListenClientWannaProxyPort : listen clientWannaProxyPort err , err is : %v !", err)
	} else {
		p.ClientWannaProxyPorts.Store(clientWannaProxyPort, l)
	}
	return l, err
}
func (p *Progress) sendBrowserConnCreateEvent(clientConn, browserConn net.Conn, cID, sID uint16) (success bool) {
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForConnCreate([]byte{}, cID, sID)
	//marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err := clientConn.Write(b)
	if err != nil {
		log.Printf("Progress#sendBrowserConnCreateEvent : notify client conn create err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err.Error())
		err = browserConn.Close()
		if err != nil {
			log.Printf("Progress#sendBrowserConnCreateEvent : after notify client conn create , close conn err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err.Error())
		}
		return false
	}
	return true
}
func (p *Progress) sendBrowserConnCloseEvent(clientConn net.Conn, cID, sID uint16) (success bool) {
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForConnClose([]byte{}, cID, sID)
	//marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err := clientConn.Write(b)
	if err != nil {
		log.Printf("Progress#sendBrowserConnCloseEvent : notify client conn close err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err.Error())
		return false
	}
	return true
}

// clientHelloHandler
//  处理client发送过来的hello
func (p *Progress) clientHelloHandler(clientConn net.Conn, clientWannaProxyPort string, sID uint16) {
	// return server hello
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForHelloReq([]byte{}, 0, sID)
	//marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err := clientConn.Write(b)
	if err != nil {
		log.Printf("Progress#clientHelloHandler : return server hello err , err is : %v !", err.Error())
	}
	// listen clientWannaProxyPort. data from browser , to client
	l, err := p.ListenClientWannaProxyPort(clientWannaProxyPort)
	if err != nil {
		log.Printf("Progress#clientHelloHandler : listen clientWannaProxyPort err , err is : %v !", err)
		return
	}
	log.Printf("Progress#clientHelloHandler : listen browser port is : %v !", clientWannaProxyPort)
	for {
		browserConn, err := l.Accept()
		if err != nil {
			log.Printf("Progress#clientHelloHandler : accept browser conn err , err is : %v !", err.Error())
			break
		}
		// rem browser conn and notify client
		cID := p.newSerialNo()
		sID := p.newSerialNo()
		p.BrowserConnRID.Store(cID, browserConn)
		success := p.sendBrowserConnCreateEvent(clientConn, browserConn, cID, sID)
		if !success {
			log.Printf("Progress#clientHelloHandler : sendBrowserConnCreateEvent fail , cID is : %v , sID is : %v  , clientWannaProxyPort is : %v , browser addr is : %v!", cID, sID, clientWannaProxyPort, browserConn.RemoteAddr())
			continue
		}
		log.Printf("Progress#clientHelloHandler : accept a browser conn success , cID is : %v , sID is : %v , clientWannaProxyPort is : %v , browser addr is : %v !", cID, sID, clientWannaProxyPort, browserConn.RemoteAddr())
	}
}
func (p *Progress) clientConnCreateDoneHandler(clientConn net.Conn, cID, sID uint16) {
	v, ok := p.BrowserConnRID.Load(cID)
	if !ok {
		p.closeBrowserConn(clientConn, sID, cID)
		return
	}
	browserConn, _ := v.(net.Conn)
	if browserConn == nil {
		p.closeBrowserConn(clientConn, sID, cID)
		return
	}
	// read browser request
	bs := make([]byte, 1024, 1024)
	for {
		// build protocol to client
		sID := p.newSerialNo()
		n, err := browserConn.Read(bs)
		s := bs[0:n]
		if err != nil {
			log.Printf("Progress#clientConnCreateDoneHandler : read browser data err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err.Error())
			p.closeBrowserConn(clientConn, cID, sID)
			return
		}
		//if n <= 0 {
		//	continue
		//}
		log.Printf("Progress#clientConnCreateDoneHandler : accept browser req , cID is : %v , sID is : %v , msg is : %v , len is : %v !", cID, sID, string(s), len(s))
		m := ippnew.NewMessage(p.Config.IPPVersion)
		m.ForReq(s, cID, sID)
		//marshal
		b := m.Marshall()
		ippLen := make([]byte, 4, 4)
		binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
		b = append(ippLen, b...)
		n, err = clientConn.Write(b)
		if err != nil {
			log.Printf("Progress#clientConnCreateDoneHandler : send browser data to client err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err.Error())
			// maybe client down , we stop the port listener
			p.closeBrowserConn(clientConn, cID, sID)
			return
		}
		log.Printf("Progress#clientConnCreateDoneHandler : from proxy to client , cID is : %v , sID is : %v , msg is : %v , len is : %v !", cID, sID, string(s), n)
	}
}

func (p *Progress) closeBrowserConn(clientConn net.Conn, cID, sID uint16) {
	_, ok := p.BrowserConnRID.Load(cID)
	if ok {
		p.sendBrowserConnCloseEvent(clientConn, cID, sID)
		p.BrowserConnRID.Delete(cID)
	}
}
//产生随机序列号
func (p *Progress) newSerialNo() uint16 {
	time.Sleep(100 * time.Millisecond)
	rand.Seed(time.Now().UnixNano())
	r := rand.Intn(math.MaxUint16)
	return uint16(r)
}
