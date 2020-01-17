package progress

import (
	"github.com/godaner/ip/ipp"
	"github.com/godaner/ip/ipp/ippnew"
	"github.com/godaner/ip/proxy/config"
	"log"
	"math"
	"math/rand"
	"net"
	"time"
)

type Progress struct {
	Config                *config.Config
	BrowserConnRID        map[uint16]net.Conn
	ClientWannaProxyPorts map[string]net.Listener
}

func (p *Progress) Listen() (err error) {
	c := new(config.Config)
	err = c.Load()
	if err != nil {
		return err
	}
	p.Config = c
	p.ClientWannaProxyPorts = map[string]net.Listener{}
	p.BrowserConnRID = map[uint16]net.Conn{}
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
				bs := make([]byte, 4096, 4096)
				n, err := clientConn.Read(bs)
				if err != nil {
					log.Printf("Progress#fromClientConnHandler : read info from client err , err is : %v !", err.Error())
					break
				}
				s := bs[0:n]
				log.Printf("Progress#fromClientConnHandler : receive client msg , msg is : %v , len is : %v !", string(s), n)
				m := ippnew.NewMessage(p.Config.IPPVersion)
				m.UnMarshall(bs[0:n])
				switch m.Type() {
				case ipp.MSG_TYPE_HELLO:
					log.Println("Progress#fromClientConnHandler : receive client hello !")
					// receive client hello , we should listen the client_wanna_proxy_port , and dispatch browser data to this client.
					clientWannaProxyPort := string(m.AttributeByType(ipp.ATTR_TYPE_PORT))
					go p.clientHelloHandler(clientConn, clientWannaProxyPort)
				case ipp.MSG_TYPE_CONN_CLOSE:
					log.Println("Progress#fromClientConnHandler : receive client conn close !")
					cID := m.CID()
					browserConn, ok := p.BrowserConnRID[cID]
					if ok {
						delete(p.BrowserConnRID, cID)
						err := browserConn.Close()
						if err != nil {
							log.Printf("Progress#fromClientConnHandler : after receive client conn close , close browser conn err , err is : %v !", err.Error())
						}
					}
				case ipp.MSG_TYPE_REQ:
					log.Println("Progress#fromClientConnHandler : receive client req !")
					// receive client req , we should judge the client port , and dispatch the data to all browser who connect to this port.
					cID := m.CID()
					browserConn := p.BrowserConnRID[cID]
					if browserConn == nil {
						continue
					}
					data := m.AttributeByType(ipp.ATTR_TYPE_BODY)
					_, err := browserConn.Write(data)
					if err != nil {
						log.Printf("Progress#fromClientConnHandler : from client to browser err , cid is : %v , err is : %v !", cID, err.Error())
						_, ok := p.BrowserConnRID[cID]
						if ok {
							p.sendBrowserConnCloseEvent(clientConn, cID)
							delete(p.BrowserConnRID, cID)
						}

					}
					log.Printf("Progress#fromClientConnHandler : from client to browser success , cid is : %v , data is : %v , data len is : %v !", cID, string(data), len(data))
				}
			}
		}()
	}

}
func (p *Progress) ListenClientWannaProxyPort(clientWannaProxyPort string) (l net.Listener, err error) {
	l, ok := p.ClientWannaProxyPorts[clientWannaProxyPort]
	if ok && l != nil {
		err := l.Close()
		if err != nil {
			log.Printf("Progress#ListenClientWannaProxyPort : close port listener err , err is : %v !", err.Error())
		}
		delete(p.ClientWannaProxyPorts, clientWannaProxyPort)
	}
	addr := ":" + clientWannaProxyPort
	l, err = net.Listen("tcp", addr)
	if err != nil {
		log.Printf("Progress#ListenClientWannaProxyPort : listen clientWannaProxyPort err , err is : %v !", err)
	} else {
		p.ClientWannaProxyPorts[clientWannaProxyPort] = l
	}
	return l, err
}
func (p *Progress) sendBrowserConnCreateEvent(clientConn, browserConn net.Conn, cID uint16) (success bool) {
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForConnCreate([]byte{}, cID)
	_, err := clientConn.Write(m.Marshall())
	if err != nil {
		log.Printf("Progress#sendBrowserConnCreateEvent : notify client conn create err , err is : %v !", err.Error())
		err = browserConn.Close()
		if err != nil {
			log.Printf("Progress#sendBrowserConnCreateEvent : after notify client conn create , close conn err , err is : %v !", err.Error())
		}
		return false
	}
	return true
}
func (p *Progress) sendBrowserConnCloseEvent(clientConn net.Conn, cID uint16) (success bool) {
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForConnClose([]byte{}, cID)
	_, err := clientConn.Write(m.Marshall())
	if err != nil {
		log.Printf("Progress#sendBrowserConnCloseEvent : notify client conn close err , err is : %v !", err.Error())
		return false
	}
	return true
}

// clientHelloHandler
//  处理client发送过来的hello
func (p *Progress) clientHelloHandler(clientConn net.Conn, clientWannaProxyPort string) {
	// return server hello
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForHelloReq([]byte{}, 0)
	_, err := clientConn.Write(m.Marshall())
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
		p.BrowserConnRID[cID] = browserConn
		success := p.sendBrowserConnCreateEvent(clientConn, browserConn, cID)
		if !success {
			log.Printf("Progress#clientHelloHandler : sendBrowserConnCreateEvent fail , clientWannaProxyPort is : %v , browser addr is : %v !", clientWannaProxyPort, browserConn.RemoteAddr())
			continue
		}
		log.Printf("Progress#clientHelloHandler : accept a browser conn success , clientWannaProxyPort is : %v , browser addr is : %v !", clientWannaProxyPort, browserConn.RemoteAddr())
		go func() {
			// read browser request
			for {
				// build protocol to client
				bs := make([]byte, 4096, 4096)
				n, err := browserConn.Read(bs)
				s := bs[0:n]
				log.Printf("Progress#clientHelloHandler : accept browser req , msg is : %v , len is : %v !", string(s), len(s))
				if err != nil {
					log.Printf("Progress#clientHelloHandler : read browser data err , err is : %v !", err.Error())
					_, ok := p.BrowserConnRID[cID]
					if ok {
						p.sendBrowserConnCloseEvent(clientConn, cID)
						delete(p.BrowserConnRID, cID)
					}
					break
				}
				m := ippnew.NewMessage(p.Config.IPPVersion)
				m.ForReq(s, cID)
				n, err = clientConn.Write(m.Marshall())
				if err != nil {
					log.Printf("Progress#clientHelloHandler : send browser data to client err , err is : %v !", err.Error())
				}
				log.Printf("Progress#clientHelloHandler : from proxy to client , msg is : %v , len is : %v !", string(s), n)
			}
		}()
	}
}

func (p *Progress) addConn(conn net.Conn, conns []net.Conn) (cs []net.Conn) {
	cs = []net.Conn{conn}
	for _, c := range conns {
		if conn.RemoteAddr() == c.RemoteAddr() {
			continue
		}
		cs = append(cs, c)
	}
	return cs
}

//产生随机序列号
func (p *Progress) newSerialNo() uint16 {
	rand.Seed(time.Now().UnixNano())
	r := rand.Intn(math.MaxUint16)
	return uint16(r)
}
