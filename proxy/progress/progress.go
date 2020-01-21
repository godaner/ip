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
	"math/rand"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
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
			_, err := clientConn.Read(length)
			if err != nil {
				log.Printf("Progress#receiveClientMsg : read ipp len info from client err , err is : %v !", err.Error())
				continue
			}
			ippLength := binary.BigEndian.Uint32(length)
			bs := make([]byte, ippLength, ippLength)
			_, err = io.ReadFull(clientConn, bs)
			if err != nil {
				log.Printf("Progress#receiveClientMsg : read info from client err , err is : %v !", err.Error())
				continue
			}
			m := ippnew.NewMessage(p.Config.IPPVersion)
			m.UnMarshall(bs)
			cID := m.CID()
			sID := m.SerialId()
			cliID := m.CliID()
			// choose handler
			switch m.Type() {
			case ipp.MSG_TYPE_CLIENT_HELLO:
				log.Printf("Progress#receiveClientMsg : receive client hello , cliID is : %v , cID is : %v , sID is : %v !", cliID, cID, sID)
				// receive client hello , we should listen the client_wanna_proxy_port , and dispatch browser data to this client.
				clientWannaProxyPort := string(m.AttributeByType(ipp.ATTR_TYPE_PORT))
				p.clientHelloHandler(clientConn, clientWannaProxyPort, cliID, sID)
			case ipp.MSG_TYPE_CONN_CREATE_DONE:
				log.Printf("Progress#receiveClientMsg : receive client conn create done , cliID is : %v , cID is : %v , sID is : %v !", cliID, cID, sID)
				p.clientConnCreateDoneHandler(clientConn, cliID, cID, sID)
			case ipp.MSG_TYPE_CONN_CLOSE:
				log.Printf("Progress#receiveClientMsg : receive client conn close , cliID is : %v , cID is : %v , sID is : %v !", cliID, cID, sID)
				p.clientConnCloseHandler(cliID, cID, sID)
			case ipp.MSG_TYPE_REQ:
				log.Printf("Progress#receiveClientMsg : receive client req , cliID is : %v , cID is : %v , sID is : %v !", cliID, cID, sID)
				// receive client req , we should judge the client port , and dispatch the data to all browser who connect to this port.
				p.clientReqHandler(clientConn, m)
			}
		}
	}

}
func (p *Progress) sendBrowserConnCreateEvent(clientConn, browserConn net.Conn, clientWannaProxyPort string, cliID, cID, sID uint16) (success bool) {
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForConnCreate([]byte(clientWannaProxyPort), cliID, cID, sID)
	//marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err := clientConn.Write(b)
	if err != nil {
		log.Printf("Progress#sendBrowserConnCreateEvent : notify client conn create err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", cliID, cID, sID, err.Error())
		err = browserConn.Close()
		if err != nil {
			log.Printf("Progress#sendBrowserConnCreateEvent : after notify client conn create , close conn err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", cliID, cID, sID, err.Error())
		}
		return false
	}
	return true
}
func (p *Progress) sendBrowserConnCloseEvent(clientConn net.Conn, cliID, cID, sID uint16) {
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForConnClose([]byte{}, cliID, cID, sID)
	//marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err := clientConn.Write(b)
	if err != nil {
		log.Printf("Progress#sendBrowserConnCloseEvent : notify client conn close err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", cliID, cID, sID, err.Error())
		return
	}
	return
}

// clientHelloHandler
//  处理client发送过来的hello
func (p *Progress) clientHelloHandler(clientConn *conn.IPConn, clientWannaProxyPort string, cliID, sID uint16) {
	clientWannaProxyPorts := strings.Split(clientWannaProxyPort, ",")
	for _, port := range clientWannaProxyPorts {
		// 监听client想监听的端口
		err := p.listenBrowser(clientConn, port, cliID, sID)
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
func (p *Progress) listenBrowser(clientConn *conn.IPConn, clientWannaProxyPort string, cliID, sID uint16) (err error) {

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
				p.sendBrowserConnCloseEvent(clientConn, cliID, cID, sID)
			}()
			// rem browser conn and notify client
			p.BrowserConnRID.Store(cID, browserConn)
			p.sendBrowserConnCreateEvent(clientConn, browserConn, clientWannaProxyPort, cliID, cID, sID)
			log.Printf("Progress#listenBrowser : accept a browser conn success , cliID is : %v , cID is : %v , sID is : %v , clientWannaProxyPort is : %v , browser addr is : %v !", cliID, cID, sID, clientWannaProxyPort, browserConn.RemoteAddr())
		}
	}()
	return nil
}

// clientConnCreateDoneHandler
//  开始监听用户发送的消息
func (p *Progress) clientConnCreateDoneHandler(clientConn *conn.IPConn, cliID, cID, sID uint16) {
	v, ok := p.BrowserConnRID.Load(cID)
	if !ok {
		return
	}
	browserConn, _ := v.(*conn.IPConn)
	if browserConn == nil {
		return
	}
	// read browser request
	bs := make([]byte, 4096, 4096)
	go func() {
		for {
			select {
			case <-browserConn.IsClose():
				log.Printf("Progress#clientConnCreateDoneHandler : get browser conn close signal , will stop read browser conn , cliID is : %v , cID is : %v , sID is : %v !", cliID, cID, sID)
				return
			case <-clientConn.IsClose():
				log.Printf("Progress#clientConnCreateDoneHandler : get client conn close signal , will stop read browser conn , cliID is : %v , cID is : %v , sID is : %v !", cliID, cID, sID)
				err := browserConn.Close()
				if err != nil {
					log.Printf("Progress#clientConnCreateDoneHandler : close browser conn when client conn close err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", cliID, cID, sID, err.Error())
				}
				return
			default:
				log.Printf("Progress#proxyCreateBrowserConnHandler : wait receive browser msg , cliID is : %v , cID is : %v , sID is : %v !", cliID, cID, sID)
				// build protocol to client
				sID := p.newSerialNo()
				n, err := browserConn.Read(bs)
				s := bs[0:n]
				if err != nil {
					log.Printf("Progress#clientConnCreateDoneHandler : read browser data err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", cliID, cID, sID, err.Error())
					continue
				}
				//if n <= 0 {
				//	continue
				//}
				log.Printf("Progress#clientConnCreateDoneHandler : accept browser req , cliID is : %v , cID is : %v , sID is : %v , len is : %v !", cliID, cID, sID, n)
				m := ippnew.NewMessage(p.Config.IPPVersion)
				m.ForReq(s, cliID, cID, sID)
				//marshal
				b := m.Marshall()
				ippLen := make([]byte, 4, 4)
				binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
				b = append(ippLen, b...)
				n, err = clientConn.Write(b)
				if err != nil {
					log.Printf("Progress#clientConnCreateDoneHandler : send browser data to client err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", cliID, cID, sID, err.Error())
					continue
				}
				log.Printf("Progress#clientConnCreateDoneHandler : from proxy to client , cliID is : %v , cID is : %v , sID is : %v , len is : %v !", cliID, cID, sID, n)
			}
		}
	}()
}

func (p *Progress) clientConnCloseHandler(cliID, cID, sID uint16) {
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
		log.Printf("Progress#clientConnCloseHandler : after receive client conn close , close browser conn err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", cliID, cID, sID, err.Error())
	}
}

func (p *Progress) clientReqHandler(clientConn net.Conn, m ipp.Message) {
	cID := m.CID()
	sID := m.SerialId()
	cliID := m.CliID()
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
		log.Printf("Progress#clientReqHandler : from client to browser err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", cliID, cID, sID, err.Error())
		return
	}
	log.Printf("Progress#clientReqHandler : from client to browser success , cliID is : %v , cID is : %v , sID is : %v , data len is : %v !", cliID, cID, sID, n)
}

func (p *Progress) sayHello(clientConn net.Conn, port string, sID uint16) {
	// return client hello
	cliID := p.newCID()
	m := ippnew.NewMessage(p.Config.IPPVersion)
	m.ForServerHelloReq([]byte(strconv.FormatInt(int64(cliID), 10)), []byte(port), sID)
	//marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err := clientConn.Write(b)
	if err != nil {
		log.Printf("Progress#sayHello : return client hello err , cliID is : %v , err is : %v !", cliID, err.Error())
		return
	}
	log.Printf("Progress#sayHello : say hello to client success , cliID is : %v , sID is : %v !", cliID, sID)
}

//产生随机序列号
func (p *Progress) newSerialNo() uint16 {
	atomic.CompareAndSwapInt32(&p.seq, math.MaxUint16, 0)
	atomic.AddInt32(&p.seq, 1)
	return uint16(p.seq)
}

//产生随机序列号
func (p *Progress) newCID() uint16 {
	rand.Seed(time.Now().UnixNano())
	r := rand.Intn(math.MaxUint16)
	return uint16(r)
}
