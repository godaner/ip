package progress

import (
	"github.com/godaner/ip/ipp"
	"github.com/godaner/ip/ipp/ippnew"
	"github.com/godaner/ip/proxy/config"
	"log"
	"net"
)

type Progress struct {
	Config                *config.Config
	ClientCIDPort         map[uint16]string
	BrowserPortConns      map[string][]net.Conn
	ClientsListener       net.Listener
	ClientWannaProxyPorts map[string]net.Listener
}

func (p *Progress) Listen() (err error) {
	c := new(config.Config)
	err = c.Load()
	if err != nil {
		return err
	}
	p.Config = c
	p.BrowserPortConns = map[string][]net.Conn{}
	p.ClientCIDPort = map[uint16]string{}
	p.ClientWannaProxyPorts = map[string]net.Listener{}
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
				bs := make([]byte, 1024, 1024)
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
					go p.clientHelloHandler(m.ReqId(), conn, clientWannaProxyPort)
				case ipp.MSG_TYPE_REQ:
					// receive client req , we should judge the client port , and dispatch the data to all browser who connect to this port.
					port := p.ClientCIDPort[m.ReqId()]
					browserConns := p.BrowserPortConns[port]
					data := m.AttributeByType(ipp.ATTR_TYPE_BODY)
					log.Printf("Progress#fromClientConnHandler : receive client req , data is : %v", string(data))
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
func (p *Progress) clientHelloHandler(cID uint16, clientConn net.Conn, clientWannaProxyPort string) {
	// remember cID port relation
	p.ClientCIDPort[cID] = clientWannaProxyPort
	// return server hello
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForHelloReq([]byte{}, cID)
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
	log.Printf("Progress#clientHelloHandler : proxy port is : %v !", clientWannaProxyPort)
	for {
		conn, err := l.Accept()
		if err != nil {
			log.Printf("Progress#clientHelloHandler : accept conn err , err is : %v !", err.Error())
			break
		}
		log.Printf("Progress#clientHelloHandler : accept a browser conn success , clientWannaProxyPort is : %v , browser addr is : %v !", clientWannaProxyPort, conn.RemoteAddr())
		// remember browser port conns relation
		conns, ok := p.BrowserPortConns[clientWannaProxyPort]
		if !ok {
			conns = []net.Conn{}
		}
		conns = p.addConn(conn, conns)
		p.BrowserPortConns[clientWannaProxyPort] = conns
		go func() {
			// one browser
			for {
				// build protocol to client
				bs := make([]byte, 1024, 1024)
				n, err := conn.Read(bs)
				log.Println("Progress#clientHelloHandler : accept browser req !")
				if err != nil {
					log.Printf("Progress#clientHelloHandler : read browser data err , err is : %v !", err.Error())
					break
				}
				m := ippnew.NewMessage(p.Config.IPPVersion)
				s := bs[0:n]
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
