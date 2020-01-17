package progress

import (
	"github.com/godaner/ip/ipp"
	"github.com/godaner/ip/ipp/ippnew"
	"github.com/godaner/ip/proxy/config"
	"log"
	"math"
	"math/rand"
	"net"
	"os"
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
	f, err := os.OpenFile("./ipproxy.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Println("Progress#Listen : open file err :", err.Error())
		return
	}
	log.SetOutput(f)
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
				bs := make([]byte, 10240, 10240)
				n, err := clientConn.Read(bs)
				if err != nil {
					log.Printf("Progress#fromClientConnHandler : read info from client err , err is : %v !", err.Error())
					// close client connection , wait client reconnect
					clientConn.Close()
					break
				}
				m := ippnew.NewMessage(p.Config.IPPVersion)
				m.UnMarshall(bs[0:n])
				cID := m.CID()
				switch m.Type() {
				case ipp.MSG_TYPE_HELLO:
					log.Printf("Progress#fromClientConnHandler : receive client hello , cID is : %v !", cID)
					// receive client hello , we should listen the client_wanna_proxy_port , and dispatch browser data to this client.
					clientWannaProxyPort := string(m.AttributeByType(ipp.ATTR_TYPE_PORT))
					go p.clientHelloHandler(clientConn, clientWannaProxyPort)
				case ipp.MSG_TYPE_CONN_CLOSE:
					log.Printf("Progress#fromClientConnHandler : receive client conn close , cID is : %v !", cID)
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
						log.Printf("Progress#fromClientConnHandler : after receive client conn close , close browser conn err , cID is : %v , err is : %v !", cID, err.Error())
					}
				case ipp.MSG_TYPE_REQ:
					log.Printf("Progress#fromClientConnHandler : receive client req , cID is : %v !", cID)
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
					if len(data) <= 0 {
						return
					}
					_, err := browserConn.Write(data)
					if err != nil {
						log.Printf("Progress#fromClientConnHandler : from client to browser err , cID is : %v , err is : %v !", cID, err.Error())
						_, ok := p.BrowserConnRID.Load(cID)
						if ok {
							p.sendBrowserConnCloseEvent(clientConn, cID)
							p.BrowserConnRID.Delete(cID)
						}

					}
					log.Printf("Progress#fromClientConnHandler : from client to browser success , cID is : %v , data is : %v , data len is : %v !", cID, string(data), len(data))
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
func (p *Progress) sendBrowserConnCreateEvent(clientConn, browserConn net.Conn, cID uint16) (success bool) {
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForConnCreate([]byte{}, cID)
	_, err := clientConn.Write(m.Marshall())
	if err != nil {
		log.Printf("Progress#sendBrowserConnCreateEvent : notify client conn create err , cID is : %v , err is : %v !", cID, err.Error())
		err = browserConn.Close()
		if err != nil {
			log.Printf("Progress#sendBrowserConnCreateEvent : after notify client conn create , close conn err , cID is : %v , err is : %v !", cID, err.Error())
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
		log.Printf("Progress#sendBrowserConnCloseEvent : notify client conn close err , cID is : %v , err is : %v !", cID, err.Error())
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
		p.BrowserConnRID.Store(cID, browserConn)
		success := p.sendBrowserConnCreateEvent(clientConn, browserConn, cID)
		if !success {
			log.Printf("Progress#clientHelloHandler : sendBrowserConnCreateEvent fail , cID is : %v  , clientWannaProxyPort is : %v , browser addr is : %v!", cID, clientWannaProxyPort, browserConn.RemoteAddr())
			continue
		}
		log.Printf("Progress#clientHelloHandler : accept a browser conn success , cID is : %v , clientWannaProxyPort is : %v , browser addr is : %v !", cID, clientWannaProxyPort, browserConn.RemoteAddr())
		go func() {
			// read browser request
			for {
				// build protocol to client
				bs := make([]byte, 10240, 10240)
				n, err := browserConn.Read(bs)
				s := bs[0:n]
				log.Printf("Progress#clientHelloHandler : accept browser req , cID is : %v , msg is : %v , len is : %v !", cID, string(s), len(s))
				if n <= 0 {
					continue
				}
				if err != nil {
					log.Printf("Progress#clientHelloHandler : read browser data err , cID is : %v , err is : %v !", cID, err.Error())
					_, ok := p.BrowserConnRID.Load(cID)
					if ok {
						p.sendBrowserConnCloseEvent(clientConn, cID)
						p.BrowserConnRID.Delete(cID)
					}
					break
				}
				m := ippnew.NewMessage(p.Config.IPPVersion)
				m.ForReq(s, cID)
				n, err = clientConn.Write(m.Marshall())
				if err != nil {
					log.Printf("Progress#clientHelloHandler : send browser data to client err , cID is : %v , err is : %v !", cID, err.Error())
				}
				log.Printf("Progress#clientHelloHandler : from proxy to client , cID is : %v , msg is : %v , len is : %v !", cID, string(s), n)
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
	time.Sleep(10 * time.Millisecond)
	return uint16(r)
}
