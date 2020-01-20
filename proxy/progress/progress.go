package progress

import (
	"encoding/binary"
	"github.com/godaner/ip/conn"
	"github.com/godaner/ip/ipp"
	"github.com/godaner/ip/ipp/ippnew"
	"github.com/godaner/ip/proxy/config"
	"io"
	"log"
	"math"
	"net"
	"strings"
	"sync"
	"sync/atomic"
)

type Progress struct {
	Config         *config.Config
	BrowserConnRID sync.Map // map[uint16]net.Conn
	seq            int32
}

func (p *Progress) Listen() (err error) {
	// log
	log.SetFlags(log.Lmicroseconds)
	// config
	c := new(config.Config)
	err = c.Load()
	if err != nil {
		return err
	}
	p.Config = c
	p.BrowserConnRID = sync.Map{} // map[uint16]net.Conn{}
	// listen client conn
	go func() {
		addr := ":" + c.LocalPort
		l, err := net.Listen("tcp", addr)
		if err != nil {
			panic(err)
		}
		log.Printf("Progress#Listen : local addr is : %v !", addr)
		for {
			c, err := l.Accept()
			if err != nil {
				log.Printf("Progress#receiveClientMsg : read ipp len info from client err , err is : %v !", err.Error())
				continue
			}
			go p.receiveClientMsg(c)
		}
	}()

	return nil
}

// receiveClientMsg
//  接受client消息
func (p *Progress) receiveClientMsg(c net.Conn) {
	clientConn := conn.NewIPConn(c)
	for {
		select {
		case <-clientConn.IsClose():
			log.Println("Progress#receiveClientMsg : get client conn close signal , we will stop read client conn !")
			return
		default:
			// parse protocol
			length := make([]byte, 4, 4)
			n, err := clientConn.Read(length)
			if err != nil {
				log.Printf("Progress#receiveClientMsg : read ipp len info from client err , err is : %v !", err.Error())
				return
			}
			ippLength := binary.BigEndian.Uint32(length)
			log.Printf("Progress#receiveClientMsg : read info from client ippLength is : %v !", ippLength)
			bs := make([]byte, ippLength, ippLength)
			n, err = io.ReadFull(clientConn, bs)
			if err != nil {
				log.Printf("Progress#receiveClientMsg : read info from client err , err is : %v !", err.Error())
				return
			}
			log.Printf("Progress#receiveClientMsg : read info from client ipp len is : %v !", n)
			m := ippnew.NewMessage(p.Config.IPPVersion)
			m.UnMarshall(bs)
			cID := m.CID()
			sID := m.SerialId()
			// choose handler
			switch m.Type() {
			case ipp.MSG_TYPE_HELLO:
				log.Printf("Progress#receiveClientMsg : receive client hello , cID is : %v , sID is : %v !", cID, sID)
				// receive client hello , we should listen the client_wanna_proxy_port , and dispatch browser data to this client.
				clientWannaProxyPort := string(m.AttributeByType(ipp.ATTR_TYPE_PORT))
				go p.clientHelloHandler(clientConn, clientWannaProxyPort, sID)
			case ipp.MSG_TYPE_CONN_CREATE_DONE:
				log.Printf("Progress#receiveClientMsg : receive client conn create done , cID is : %v , sID is : %v !", cID, sID)
				go p.clientConnCreateDoneHandler(clientConn, cID, sID)
			case ipp.MSG_TYPE_CONN_CLOSE:
				log.Printf("Progress#receiveClientMsg : receive client conn close , cID is : %v , sID is : %v !", cID, sID)
				p.clientConnCloseHandler(cID, sID)
			case ipp.MSG_TYPE_REQ:
				log.Printf("Progress#receiveClientMsg : receive client req , cID is : %v , sID is : %v !", cID, sID)
				// receive client req , we should judge the client port , and dispatch the data to all browser who connect to this port.
				p.clientReqHandler(clientConn, m, cID, sID)
			}
		}
	}

}
func (p *Progress) sendBrowserConnCreateEvent(clientConn, browserConn net.Conn, clientWannaProxyPort string, cID, sID uint16) (success bool) {
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForConnCreate([]byte(clientWannaProxyPort), cID, sID)
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
func (p *Progress) sendBrowserConnCloseEvent(clientConn net.Conn, cID, sID uint16) {
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
		return
	}
	return
}

// clientHelloHandler
//  处理client发送过来的hello
func (p *Progress) clientHelloHandler(clientConn *conn.IPConn, clientWannaProxyPort string, sID uint16) {
	clientWannaProxyPorts := strings.Split(clientWannaProxyPort, ",")
	for _, port := range clientWannaProxyPorts {
		// 监听client想监听的端口
		err := p.listenBrowser(clientConn, port, sID)
		if err != nil {
			// say hello to client , if listen fail , return "" port to client
			p.sayHello(clientConn, "", sID)
			return
		}

	}
	// say hello to client , if listen fail , return "" port to client
	p.sayHello(clientConn, clientWannaProxyPort, sID)

}

// listenBrowser
//  监听browser信息
func (p *Progress) listenBrowser(clientConn *conn.IPConn, clientWannaProxyPort string, sID uint16) (err error) {

	// listen clientWannaProxyPort. data from browser , to client
	l, err := net.Listen("tcp", ":"+clientWannaProxyPort)
	if err != nil {
		log.Printf("Progress#listenBrowser : listen clientWannaProxyPort err , err is : %v !", err)
		return err
	}
	// check client conn
	go func() {
		<-clientConn.IsClose()
		log.Println("Progress#listenBrowser : get client conn close signal , we will close listener !")
		if l == nil {
			return
		}
		err := l.Close()
		if err != nil {
			log.Printf("Progress#listenBrowser : after get client conn close signal , we will close listener err , err is : %v !", err.Error())
		}
	}()
	log.Printf("Progress#listenBrowser : listen browser port is : %v !", clientWannaProxyPort)
	go func() {
		for {
			// when listener stop , we stop accept
			c, err := l.Accept()
			if err != nil {
				log.Printf("Progress#listenBrowser : accept browser conn err , err is : %v !", err.Error())
				break
			}
			// cID sID
			cID := p.newSerialNo()
			sID := p.newSerialNo()
			// check browser conn
			browserConn := conn.NewIPConn(c)
			go func() {
				<-browserConn.IsClose()
				log.Println("Progress#listenBrowser : browser conn is close !")
				p.BrowserConnRID.Delete(cID)
				p.sendBrowserConnCloseEvent(clientConn, cID, sID)
			}()
			// rem browser conn and notify client
			p.BrowserConnRID.Store(cID, browserConn)
			p.sendBrowserConnCreateEvent(clientConn, browserConn, clientWannaProxyPort, cID, sID)
			log.Printf("Progress#listenBrowser : accept a browser conn success , cID is : %v , sID is : %v , clientWannaProxyPort is : %v , browser addr is : %v !", cID, sID, clientWannaProxyPort, browserConn.RemoteAddr())
		}
	}()
	return nil
}

// clientConnCreateDoneHandler
//  开始监听用户发送的消息
func (p *Progress) clientConnCreateDoneHandler(clientConn net.Conn, cID, sID uint16) {
	v, ok := p.BrowserConnRID.Load(cID)
	if !ok {
		return
	}
	browserConn, _ := v.(*conn.IPConn)
	if browserConn == nil {
		return
	}
	// read browser request
	bs := make([]byte, 1024, 1024)
	for {
		select {
		case <-browserConn.IsClose():
			log.Printf("Progress#clientConnCreateDoneHandler : get browser conn close signal , will stop read browser conn , cID is : %v , sID is : %v !", cID, sID)
			return
		default:
			log.Printf("Progress#proxyCreateBrowserConnHandler : wait receive browser msg , cID is : %v , sID is : %v !", cID, sID)
			// build protocol to client
			sID := p.newSerialNo()
			n, err := browserConn.Read(bs)
			s := bs[0:n]
			if err != nil {
				log.Printf("Progress#clientConnCreateDoneHandler : read browser data err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err.Error())
				return
			}
			//if n <= 0 {
			//	continue
			//}
			log.Printf("Progress#clientConnCreateDoneHandler : accept browser req , cID is : %v , sID is : %v , len is : %v !", cID, sID, n)
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
				return
			}
			log.Printf("Progress#clientConnCreateDoneHandler : from proxy to client , cID is : %v , sID is : %v , len is : %v !", cID, sID, n)
		}
	}
}

func (p *Progress) clientConnCloseHandler(cID, sID uint16) {
	v, ok := p.BrowserConnRID.Load(cID)
	if !ok {
		return
	}
	browserConn, _ := v.(net.Conn)
	if browserConn == nil {
		return
	}
	p.BrowserConnRID.Delete(cID)
	err := browserConn.Close()
	if err != nil {
		log.Printf("Progress#clientConnCloseHandler : after receive client conn close , close browser conn err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err.Error())
	}
}

func (p *Progress) clientReqHandler(clientConn net.Conn, m ipp.Message, cID, sID uint16) {
	v, ok := p.BrowserConnRID.Load(cID)
	if !ok {
		return
	}
	browserConn, _ := v.(net.Conn)
	if browserConn == nil {
		return
	}
	data := m.AttributeByType(ipp.ATTR_TYPE_BODY)
	//if len(data) <= 0 {
	//	return
	//}
	n, err := browserConn.Write(data)
	if err != nil {
		log.Printf("Progress#clientReqHandler : from client to browser err , cID is : %v , sID is : %v , err is : %v !", cID, sID, err.Error())
		return
	}
	log.Printf("Progress#clientReqHandler : from client to browser success , cID is : %v , sID is : %v , data len is : %v !", cID, sID, n)
}

func (p *Progress) sayHello(clientConn net.Conn, port string, sID uint16) {
	// return server hello
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForHelloReq([]byte(port), 0, sID)
	//marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err := clientConn.Write(b)
	if err != nil {
		log.Printf("Progress#sayHello : return server hello err , err is : %v !", err.Error())
		return
	}
	log.Printf("Progress#sayHello : say hello to proxy success , sID is : %v !", sID)
}

//产生随机序列号
func (p *Progress) newSerialNo() uint16 {
	atomic.CompareAndSwapInt32(&p.seq, math.MaxUint16, 0)
	atomic.AddInt32(&p.seq, 1)
	return uint16(p.seq)
}
