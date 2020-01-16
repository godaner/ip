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
	//ClientCIDPort         map[uint16]string
	//BrowserPortConns      map[string][]net.Conn
	ClientsListener       net.Listener
	//ClientWannaProxyPorts map[string]net.Listener
	BrowserAndClientConns map[uint16]*BrowserAndClientConn
}

func (p *Progress) Listen() (err error) {
	c := new(config.Config)
	err = c.Load()
	if err != nil {
		return err
	}
	p.Config = c
	//p.BrowserPortConns = map[string][]net.Conn{}
	//p.ClientCIDPort = map[uint16]string{}
	//p.ClientWannaProxyPorts = map[string]net.Listener{}
	p.BrowserAndClientConns = map[uint16]*BrowserAndClientConn{}
	// from client conn
	go func() {
		addr := ":" + c.LocalPort
		l, err := net.Listen("tcp", addr)
		if err != nil {
			panic(err)
		}
		log.Printf("Progress#Listen : local addr is : %v !", addr)
		p.ClientsListener = l
		go p.fromClientConnHandler()
	}()

	return nil
}
func (p *Progress) fromClientConnHandler() {
	for {
		conn, err := p.ClientsListener.Accept()
		if err != nil {
			panic(err)
		}
		go func() {
			for {
				// parse protocol
				bs := make([]byte, 4096, 4096)
				n, err := conn.Read(bs)
				if err != nil {
					log.Printf("Progress#fromClientConnHandler : read info from client err , err is : %v !", err.Error())
					break
				}
				m := ippnew.NewMessage(p.Config.IPPVersion)
				m.UnMarshall(bs[0:n])
				switch m.Type() {
				case ipp.MSG_TYPE_HELLO:
					log.Println("Progress#fromClientConnHandler : receive client hello !")
					// receive client hello , we should listen the client_wanna_proxy_port , and dispatch browser data to this client.
					clientWannaProxyPort := string(m.AttributeByType(ipp.ATTR_TYPE_PORT))
					go p.clientHelloHandler( conn, clientWannaProxyPort)
				case ipp.MSG_TYPE_REQ:
					// receive client req , we should judge the client port , and dispatch the data to all browser who connect to this port.
					port := p.ClientCIDPort[m.ReqId()]
					browserConns := p.BrowserPortConns[port]
					data := m.AttributeByType(ipp.ATTR_TYPE_BODY)
					log.Printf("Progress#fromClientConnHandler : receive client req , data is : %v , len is : %v !", string(data),len(data))
					tc := len(browserConns)
					sc := int64(0)
					for _, bc := range browserConns {
						//if sc >= 1 {
						//	break
						//}
						_, err := bc.Write(data)
						if err != nil {
							log.Printf("Progress#fromClientConnHandler : from client to browser err clientWannaProxyPort is : %v , err is : %v !", port, err.Error())
							delete(p.BrowserPortConns, port)
						} else {
							log.Printf("Progress#fromClientConnHandler : from client to a browser success , clientWannaProxyPort is : %v , browser addr is : %v !", port, bc.RemoteAddr())
							sc++
						}
					}
					log.Printf("Progress#fromClientConnHandler : from client to browser success , clientWannaProxyPort is : %v , browser total num is : %v , success num is : %v !", port, tc, sc)
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
type BrowserAndClientConn struct {
	id uint16
	clientConn net.Conn
	browserConn net.Conn
}
// clientHelloHandler
//  处理client发送过来的hello
func (p *Progress) clientHelloHandler(clientConn net.Conn, clientWannaProxyPort string) {
	// return server hello
	rID:=p.newSerialNo()
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForHelloReq([]byte{}, rID)
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
		conn, err := l.Accept()
		if err != nil {
			log.Printf("Progress#clientHelloHandler : accept browser conn err , err is : %v !", err.Error())
			break
		}
		// save client conn and browser conn relation
		p.BrowserAndClientConns[rID]=&BrowserAndClientConn{
			id:          rID,
			clientConn:  clientConn,
			browserConn: conn,
		}
		log.Printf("Progress#clientHelloHandler : accept a browser conn success , clientWannaProxyPort is : %v , browser addr is : %v !", clientWannaProxyPort, conn.RemoteAddr())
		go func() {
			// read browser request
			for {
				// build protocol to client
				bs := make([]byte, 4096, 4096)
				n, err := conn.Read(bs)
				s := bs[0:n]
				log.Printf("Progress#clientHelloHandler : accept browser req , msg is : %v , len is : %v !",string(s),len(s))
				if err != nil {
					log.Printf("Progress#clientHelloHandler : read browser data err , err is : %v !", err.Error())
					break
				}
				m := ippnew.NewMessage(p.Config.IPPVersion)
				m.ForReq(s, rID)
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
