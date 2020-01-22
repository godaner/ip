package progress

import (
	"encoding/binary"
	"encoding/json"
	"github.com/godaner/ip/ipp"
	"github.com/godaner/ip/ipp/ippnew"
	ipnet "github.com/godaner/ip/net"
	"io"
	"log"
	"math"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const (
	restart_interval = 5
)

type Client struct {
	ProxyAddr            string
	IPPVersion           int
	ClientForwardAddr    string
	ClientWannaProxyPort string
	TempCliID            uint16
	V2Secret             string
	proxyConn            *ipnet.IPConn
	forwardConnRID       sync.Map // map[uint16]net.Conn
	seq                  int32
	cliID                uint16
	proxyHelloSignal     chan bool
	restartSignal        chan bool
	stopSignal           chan bool
	initSignalLock       sync.RWMutex
}

// initSignal
//  close old , new signal
func (p *Client) initSignal(signal chan bool) (newSignal chan bool) {
	p.initSignalLock.Lock()
	defer p.initSignalLock.Unlock()
	if signal != nil {
		select {
		case <-signal:
		default:
			close(signal)
		}
	}
	return make(chan bool)
}
func (p *Client) Start() (err error) {
	//// init var ////
	p.stopSignal = p.initSignal(p.stopSignal)
	p.restartSignal = p.initSignal(p.restartSignal)
	// temp client id
	p.cliID = p.TempCliID
	// proxy net
	go func() {
		for {
			select {
			case <-p.restartSignal:
				log.Printf("Client#Start : we will start the client , cliID is : %v !", p.cliID)
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
func (p *Client) Stop() (err error) {
	<-p.stopSignal
	return nil
}

// setRestartSignal
//  restart the client
func (p *Client) setRestartSignal() {
	select {
	case <-p.restartSignal:
	default:
		close(p.restartSignal)
	}
}
func (p *Client) listenProxy() {
	//// print info ////
	i, _ := json.Marshal(p)
	log.Printf("Client#listenProxy : print client info , cliID is : %v , info is : %v !", p.cliID, string(i))

	//// init var ////
	// reset restart signal
	p.forwardConnRID = sync.Map{}
	p.restartSignal = p.initSignal(p.restartSignal)

	//// dial proxy conn ////
	addr := p.ProxyAddr
	c, err := net.Dial("tcp", addr)
	if err != nil {
		log.Printf("Client#listenProxy : dial proxy addr err , cliID is : %v , err is : %v !", p.cliID, err)
		p.setRestartSignal()
		return
	}
	p.proxyConn = ipnet.NewIPConn(c)
	p.proxyConn.SetCloseTrigger(p.restartSignal, p.stopSignal)
	p.proxyConn.SetCloseHandler(func(conn net.Conn) {
		p.setRestartSignal()
		log.Printf("Client#listenProxy : proxy conn is close , we will restart , cliID is : %v  !", p.cliID)
	})
	log.Printf("Client#listenProxy : dial proxy success , cliID is : %v , proxy addr is : %v !", p.cliID, addr)

	//// receive proxy msg ////
	go func() {
		p.receiveProxyMsg()
	}()

	//// say hello to proxy ////
	// wait some time , then check the proxy hello response
	go func() {
		p.proxyHelloSignal = p.initSignal(p.proxyHelloSignal)
		// check
		select {
		case <-time.After(restart_interval * time.Second):
			p.setRestartSignal()
			log.Printf("Client#listenProxy : can't receive proxy hello in %vs , some reasons as follow : 1. maybe client's ipp version is diff from proxy , 2. maybe client's ippv2 secret is diff from proxy , 3. maybe the data sent to proxy is not right , cliID is : %v !", restart_interval, p.cliID)
			return
		case <-p.restartSignal:
			return
		case <-p.stopSignal:
			return
		case <-p.proxyConn.IsClose():
			return
		case <-p.proxyHelloSignal:
			return
		}
	}()
	// send hello ipp
	cID := uint16(0)
	sID := p.newSerialNo()
	m := ippnew.NewMessage(p.IPPVersion, ippnew.SetV2Secret(p.V2Secret))
	clientWannaProxyPorts := p.ClientWannaProxyPort
	m.ForClientHelloReq([]byte(clientWannaProxyPorts), sID)
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err = p.proxyConn.Write(b)
	if err != nil {
		log.Printf("Client#listenProxy : say hello to proxy err , cliID is : %v , cID is : %v , sID is : %v , err : %v !", p.cliID, cID, sID, err)
		return
	}
	log.Printf("Client#listenProxy : say hello to proxy success , cliID is : %v , cID is : %v , sID is : %v , proxy addr is : %v !", p.cliID, cID, sID, addr)

}

// receiveProxyMsg
//  监听proxy返回的消息
func (p *Client) receiveProxyMsg() {
	for {
		select {
		case <-p.proxyConn.IsClose():
			log.Printf("Client#receiveProxyMsg : get proxy conn close signal , will stop read proxy conn , cliID is : %v !", p.cliID)
			return
		default:
			// parse protocol
			sID := p.newSerialNo()
			length := make([]byte, 4, 4)
			_, err := p.proxyConn.Read(length)
			if err != nil {
				log.Printf("Client#receiveProxyMsg : receive proxy ipp len err , cliID is : %v , err is : %v !", p.cliID, err)
				continue
			}
			ippLength := binary.BigEndian.Uint32(length)
			bs := make([]byte, ippLength, ippLength)
			_, err = io.ReadFull(p.proxyConn, bs)
			if err != nil {
				log.Printf("Client#receiveProxyMsg : receive proxy err , cliID is : %v , err is : %v !", p.cliID, err)
				continue
			}
			m := ippnew.NewMessage(p.IPPVersion, ippnew.SetV2Secret(p.V2Secret))
			err = m.UnMarshall(bs)
			if err != nil {
				log.Printf("Client#receiveProxyMsg : UnMarshall proxy err , maybe version not right or data is not right , cliID is : %v , err is : %v !", p.cliID, err)
				continue
			}
			cID := m.CID()
			// choose handler
			switch m.Type() {
			case ipp.MSG_TYPE_PROXY_HELLO:
				log.Printf("Client#receiveProxyMsg : receive proxy hello , cliID is : %v , cID is : %v , sID is : %v !", p.cliID, cID, sID)
				p.proxyHelloHandler(m, cID, sID)
			case ipp.MSG_TYPE_CONN_CREATE:
				log.Printf("Client#receiveProxyMsg : receive proxy conn create , cliID is : %v , cID is : %v , sID is : %v !", p.cliID, cID, sID)
				p.proxyCreateBrowserConnHandler(m, cID, sID)
			case ipp.MSG_TYPE_CONN_CLOSE:
				log.Printf("Client#receiveProxyMsg : receive proxy conn close , cliID is : %v , cID is : %v , sID is : %v !", p.cliID, cID, sID)
				p.proxyCloseBrowserConnHandler(cID, sID)
			case ipp.MSG_TYPE_REQ:
				log.Printf("Client#receiveProxyMsg : receive proxy req , cliID is : %v , cID is : %v , sID is : %v !", p.cliID, cID, sID)
				// receive proxy req info , we should dispatch the info
				p.proxyReqHandler(m)
			default:
				log.Printf("Client#receiveProxyMsg : receive proxy msg , but can't find type , cliID is : %v !", p.cliID)
			}
		}
	}

}

// proxyReqHandler
func (p *Client) proxyReqHandler(m ipp.Message) {
	cID := m.CID()
	sID := m.SerialId()
	b := m.AttributeByType(ipp.ATTR_TYPE_BODY)
	log.Printf("Client#proxyReqHandler : receive proxy req , cliID is : %v , cID is : %v , sID is : %v , len is : %v !", p.cliID, cID, sID, len(b))
	v, ok := p.forwardConnRID.Load(cID)
	if !ok {
		log.Printf("Client#proxyReqHandler : receive proxy req but no forward conn find , not ok , cliID is : %v , cID is : %v , sID is : %v !", p.cliID, cID, sID)
		return
	}
	forwardConn, _ := v.(net.Conn)
	if forwardConn == nil {
		log.Printf("Client#proxyReqHandler : receive proxy req but no forward conn find , forwardConnis nil , cliID is : %v , cID is : %v , sID is : %v !", p.cliID, cID, sID)
		return
	}
	n, err := forwardConn.Write(b)
	if err != nil {
		log.Printf("Client#proxyReqHandler : receive proxy req , cliID is : %v , cID is : %v , sID is : %v , forward err , err : %v !", p.cliID, cID, sID, err)
	}
	log.Printf("Client#proxyReqHandler : from proxy to forward , cliID is : %v , cID is : %v , sID is : %v , len is : %v !", p.cliID, cID, sID, n)
}

// proxyCreateBrowserConnHandler
//  处理proxy回复的conn_create信息
func (p *Client) proxyCreateBrowserConnHandler(m ipp.Message, cID, sID uint16) {
	//// proxy return browser conn create , we should dial forward addr ////
	port := string(m.AttributeByType(ipp.ATTR_TYPE_PORT))
	log.Printf("Client#proxyCreateBrowserConnHandler : accept proxy create browser conn , cliID is : %v , cID is : %v , sID is : %v , port is : %v !", p.cliID, cID, sID, port)
	forwardAddr := p.ClientForwardAddr
	c, err := net.Dial("tcp", forwardAddr)
	if err != nil {
		// if dial fail , tell proxy to close browser net
		log.Printf("Client#proxyCreateBrowserConnHandler : after get proxy browser conn create , dial forward err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", p.cliID, cID, sID, err)
		p.sendForwardConnCloseEvent(cID, sID)
		return
	}
	forwardConn := ipnet.NewIPConn(c)
	p.forwardConnRID.Store(cID, forwardConn)

	forwardConn.SetCloseTrigger(p.stopSignal, p.restartSignal, p.proxyConn.IsClose())
	forwardConn.SetCloseHandler(func(conn net.Conn) {
		log.Printf("Client#proxyCreateBrowserConnHandler : forward conn is close , cliID is : %v , cID is : %v , sID is : %v !", p.cliID, cID, sID)
		_, ok := p.forwardConnRID.Load(cID)
		if !ok {
			return
		}
		p.forwardConnRID.Delete(cID)
		p.sendForwardConnCloseEvent(cID, sID)
	})
	log.Printf("Client#proxyCreateBrowserConnHandler : dial forward addr success , cliID is : %v , cID is : %v , sID is : %v , forward local address is : %v , forward remote address is : %v !", p.cliID, cID, sID, forwardConn.LocalAddr(), forwardConn.RemoteAddr())

	//// read forward data ////
	bs := make([]byte, 4096, 4096)
	go func() {
		for {
			select {
			case <-forwardConn.IsClose():
				log.Printf("Client#proxyCreateBrowserConnHandler : get forward conn close signal , will stop read forward conn , cliID is : %v , cID is : %v , sID is : %v !", p.cliID, cID, sID)
				return
			default:
				sID = p.newSerialNo()
				log.Printf("Client#proxyCreateBrowserConnHandler : wait receive forward msg , cliID is : %v , cID is : %v , sID is : %v !", p.cliID, cID, sID)
				n, err := forwardConn.Read(bs)
				if err != nil {
					log.Printf("Client#proxyCreateBrowserConnHandler : read forward data err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", p.cliID, cID, sID, err)
					continue
				}
				log.Printf("Client#proxyCreateBrowserConnHandler : receive forward msg , cliID is : %v , cID is : %v , sID is : %v , len is : %v !", p.cliID, cID, sID, n)
				//if n <= 0 {
				//	return
				//}

				m := ippnew.NewMessage(p.IPPVersion, ippnew.SetV2Secret(p.V2Secret))
				m.ForReq(bs[0:n], p.cliID, cID, sID)
				//marshal
				b := m.Marshall()
				ippLen := make([]byte, 4, 4)
				binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
				b = append(ippLen, b...)
				n, err = p.proxyConn.Write(b)
				if err != nil {
					log.Printf("Client#proxyCreateBrowserConnHandler : write forward's data to proxy err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", p.cliID, cID, sID, err.Error())
					continue
				}
				log.Printf("Client#proxyCreateBrowserConnHandler : from client to proxy msg , cliID is : %v , cID is : %v , sID is : %v , len is : %v !", p.cliID, cID, sID, n)

			}

		}
	}()

	//// notify proxy ////
	p.sendCreateConnDoneEvent(cID, sID)
}
func (p *Client) sendCreateConnDoneEvent(cID, sID uint16) {

	m := ippnew.NewMessage(p.IPPVersion, ippnew.SetV2Secret(p.V2Secret))
	m.ForConnCreateDone([]byte{}, p.cliID, cID, sID)
	//marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err := p.proxyConn.Write(b)
	if err != nil {
		log.Printf("Client#sendForwardConnCloseEvent : notify proxy conn close err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", p.cliID, cID, sID, err.Error())
		return
	}
	return
}
func (p *Client) sendForwardConnCloseEvent(cID, sID uint16) {

	m := ippnew.NewMessage(p.IPPVersion, ippnew.SetV2Secret(p.V2Secret))
	m.ForConnClose([]byte{}, p.cliID, cID, sID)
	//marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err := p.proxyConn.Write(b)
	if err != nil {
		log.Printf("Client#sendForwardConnCloseEvent : notify proxy conn close err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", p.cliID, cID, sID, err.Error())
		return
	}
	return
}

// proxyCloseBrowserConnHandler
func (p *Client) proxyCloseBrowserConnHandler(cID, sID uint16) {
	v, ok := p.forwardConnRID.Load(cID)
	if !ok {
		return
	}
	c, _ := v.(net.Conn)
	if c == nil {
		return
	}
	p.forwardConnRID.Delete(cID)
	err := c.Close()
	if err != nil {
		log.Printf("Client#proxyCloseBrowserConnHandler : close forward conn err , cliID is : %v , cID is : %v , sID is : %v , err : %v !", p.cliID, cID, sID, err.Error())
	}
}

// proxyHelloHandler
//  处理proxy返回的hello信息
func (p *Client) proxyHelloHandler(m ipp.Message, cID uint16, sID uint16) {
	close(p.proxyHelloSignal)
	// check err code
	if m.ErrorCode() == ipp.ERROR_CODE_BROWSER_PORT_OCUP {
		log.Printf("Client#proxyHelloHandler : receive browser port be occupied err code , we will restart the client , cliID is : %v , cID is : %v , sID is : %v , errCode is : %v !", p.cliID, cID, sID, m.ErrorCode())
		p.setRestartSignal()
		return
	}
	// check err code
	if m.ErrorCode() == ipp.ERROR_CODE_VERSION_NOT_MATCH {
		log.Printf("Client#proxyHelloHandler : receive version not match err code , cliID is : %v , cID is : %v , sID is : %v , errCode is : %v !", p.cliID, cID, sID, m.ErrorCode())
		p.setRestartSignal()
		return
	}
	// get client id from proxy response
	cliID, err := strconv.ParseInt(string(m.AttributeByType(ipp.ATTR_TYPE_CLI_ID)), 10, 32)
	if err != nil {
		log.Printf("Client#proxyHelloHandler : accept proxy hello , parse cliID err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", p.cliID, cID, sID, err.Error())
		p.setRestartSignal()
		return
	}
	p.cliID = uint16(cliID)

	// check version
	if m.Version() != byte(p.IPPVersion) {
		log.Printf("Client#proxyHelloHandler : accept proxy hello , but ipp version is not right , proxy version is : %v , client version is :%v , cliID is : %v , cID is : %v , sID is : %v !", m.Version(), p.IPPVersion, p.cliID, cID, sID)
		p.setRestartSignal()
		return
	}

	// check port
	port := string(m.AttributeByType(ipp.ATTR_TYPE_PORT))
	if port != p.ClientWannaProxyPort {
		log.Printf("Client#proxyHelloHandler : maybe listen port in porxy side be occupied , cliID is : %v , cID is : %v , sID is : %v !", p.cliID, cID, sID)
		p.setRestartSignal()
		return
	}
	log.Printf("Client#proxyHelloHandler : accept proxy hello , and listen port success in porxy side , cliID is : %v , port is : %v , cID is : %v , sID is : %v !", p.cliID, port, cID, sID)
	return
}

//产生随机序列号
func (p *Client) newSerialNo() uint16 {
	atomic.CompareAndSwapInt32(&p.seq, math.MaxUint16, 0)
	atomic.AddInt32(&p.seq, 1)
	return uint16(p.seq)
}
