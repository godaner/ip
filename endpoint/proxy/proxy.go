package proxy

import (
	"encoding/binary"
	"encoding/json"
	"github.com/godaner/ip/ipp"
	"github.com/godaner/ip/ipp/ippnew"
	ipnet "github.com/godaner/ip/net"
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

const (
	restart_interval           = 5
	wait_server_hello_time_sec = 5
	hb_interval_sec            = 15
)

type Proxy struct {
	LocalPort      string
	IPPVersion     int
	V2Secret       string
	browserConnRID sync.Map // map[uint16]net.Conn
	seq            int32
	destroySignal  chan bool
	stopSignal     chan bool
	startSignal    chan bool
	isStart        bool
	sync.Once
}

func (p *Proxy) IsStart() bool {
	p.init()
	return p.isStart
}

func (p *Proxy) Destroy() error {
	close(p.destroySignal)
	return nil
}

func (p *Proxy) GetID() uint16 {
	return 0
}

func (p *Proxy) Restart() error {
	p.init()
	if p.IsStart() {
		err := p.Stop()
		if err != nil {
			return err
		}
	}
	err := p.Start()
	if err != nil {
		return err
	}
	return nil
}

func (p *Proxy) Stop() (err error) {
	p.init()
	if !p.IsStart() {
		return
	}
	if p.stopSignal == nil {
		return nil
	}
	stopSignal := p.stopSignal
	select {
	case <-stopSignal:
		// already stop , never happen
	default:
		// omit close signal
		p.isStart = false
		p.stopSignal = make(chan bool)
		close(stopSignal)

	}
	return nil
}

func (p *Proxy) Start() (err error) {
	p.init()
	if p.IsStart() {
		return
	}
	if p.startSignal == nil {
		return nil
	}
	startSignal := p.startSignal
	select {
	case <-startSignal:
		// already start , never happen
	default:
		// omit start signal
		p.isStart = true
		p.startSignal = make(chan bool)
		close(startSignal)
	}
	return nil
}

// init
func (p *Proxy) init() {
	p.Do(func() {
		//// init var ////
		p.destroySignal = make(chan bool)
		p.stopSignal = make(chan bool)
		p.startSignal = make(chan bool)

		//// log ////
		p.browserConnRID = sync.Map{} // map[uint16]net.Conn{}
		go func() {
			for {
				select {
				case <-p.destroySignal:
					log.Printf("Proxy#init : get destroy the proxy signal , we will destroy the proxy !")
					return
				case <-p.stopSignal:
					log.Printf("Proxy#init : get stop the proxy signal , we will stop the proxy !")
					continue
				}
			}
		}()
		go func() {
			for {
				select {
				case <-p.destroySignal:
					log.Printf("Proxy#init : get destroy the proxy signal , we will destroy the proxy !")
					return
				case <-p.startSignal: // wanna start
					log.Printf("Proxy#init : get start the proxy signal , we will start the proxy in %vs !", restart_interval)
					time.Sleep(restart_interval * time.Second)
					select {
					case <-p.stopSignal:
						log.Printf("Proxy#init : when we wanna start proxy , but get stop signal , so stop the proxy !")
						continue
					default:
						go p.startListen()
						continue
					}
				}
			}
		}()

		// wait the select
		time.Sleep(500 * time.Millisecond)
	})

}
func (p *Proxy) startListen() {
	//// print info ////
	i, _ := json.Marshal(p)
	log.Printf("Proxy#startListen : print proxy info , info is : %v !", string(i))
	//// listen client conn ////
	go func() {
		// lis
		addr := ":" + p.LocalPort
		lis, err := net.Listen("tcp", addr)
		if err != nil {
			p.Restart()
			return
		}
		cl := ipnet.NewIPListener(lis)
		cl.AddCloseTrigger(func(listener net.Listener) {
			log.Printf("Proxy#startListen : client listener close by self !")
		}, &ipnet.ListenerCloseTrigger{
			Signal: p.stopSignal,
			Handler: func(listener net.Listener) {
				log.Printf("Proxy#startListen : client listener close by stopSignal !")
				listener.Close()
			},
		}, &ipnet.ListenerCloseTrigger{
			Signal: p.destroySignal,
			Handler: func(listener net.Listener) {
				log.Printf("Proxy#startListen : client listener close by destroySignal !")
				listener.Close()
			},
		})
		log.Printf("Proxy#startListen : local addr is : %v !", addr)

		// accept conn
		p.acceptClientConn(cl)

		log.Println("Progress#startListen : stop the client success !")
	}()
}

// acceptClientConn
func (p *Proxy) acceptClientConn(cl *ipnet.IPListener) {
	for {
		select {
		case <-cl.CloseSignal():
			log.Println("Proxy#acceptClientConn : stop client accept !")
			return
		default:
			c, err := cl.Accept()
			if err != nil {
				log.Printf("Proxy#acceptClientConn : accept client conn err , err is : %v !", err.Error())
				continue
			}

			// client conn
			cc := c.(*ipnet.IPConn)
			// set hb interval and check client heart beat
			cc.SetHeartBeatInterval(time.Duration(hb_interval_sec) * time.Second)
			go p.checkClientHB(cc)
			// add trigger
			cc.AddCloseTrigger(func(conn net.Conn) {
				log.Printf("Proxy#acceptClientConn : client conn close by self !")
			}, &ipnet.ConnCloseTrigger{
				Signal: cl.CloseSignal(),
				Handler: func(conn net.Conn) {
					log.Printf("Proxy#acceptClientConn : client conn close by client listener closeSignal !")
					conn.Close()
				},
			}, &ipnet.ConnCloseTrigger{
				Signal: p.stopSignal,
				Handler: func(conn net.Conn) {
					log.Printf("Proxy#acceptClientConn : client conn close by client stopSignal !")
					conn.Close()
				},
			}, &ipnet.ConnCloseTrigger{
				Signal: p.destroySignal,
				Handler: func(conn net.Conn) {
					log.Printf("Proxy#acceptClientConn : client conn close by client destroySignal !")
					conn.Close()
				},
			})
			go p.receiveClientMsg(cc)
		}
	}
}

// receiveClientMsg
//  接受client消息
func (p *Proxy) receiveClientMsg(clientConn *ipnet.IPConn) {
	for {
		select {
		case <-clientConn.CloseSignal():
			log.Println("Proxy#receiveClientMsg : get client conn close signal , we will stop read client conn !")
			return
		default:
			// parse protocol
			length := make([]byte, 4, 4)
			_, err := clientConn.Read(length)
			if err != nil {
				log.Printf("Proxy#receiveClientMsg : read ipp len info from client err , err is : %v !", err.Error())
				continue
			}
			ippLength := binary.BigEndian.Uint32(length)
			bs := make([]byte, ippLength, ippLength)
			_, err = io.ReadFull(clientConn, bs)
			if err != nil {
				log.Printf("Proxy#receiveClientMsg : read info from client err , err is : %v !", err.Error())
				continue
			}
			m := ippnew.NewMessage(p.IPPVersion, ippnew.SetV2Secret(p.V2Secret))
			err = m.UnMarshall(bs)
			if err != nil {
				log.Printf("Proxy#receiveClientMsg : UnMarshall proxy err , some reasons as follow : 1. maybe client's ipp version is diff from proxy , 2. maybe client's ippv2 secret is diff from proxy , 3. maybe the data sent to proxy is not right , err is : %v !", err)
				continue
			}
			cID := m.CID()
			sID := m.SerialId()
			cliID := m.CliID()
			// choose handler
			switch m.Type() {
			case ipp.MSG_TYPE_CLIENT_HELLO:
				log.Printf("Proxy#receiveClientMsg : receive client hello , cliID is : %v , cID is : %v , sID is : %v !", cliID, cID, sID)
				// receive client hello , we should listen the client_wanna_proxy_port , and dispatch browser data to this client.
				clientWannaProxyPort := string(m.AttributeByType(ipp.ATTR_TYPE_PORT))
				p.clientHelloHandler(clientConn, clientWannaProxyPort, cliID, sID)
			case ipp.MSG_TYPE_CONN_CREATE_DONE:
				log.Printf("Proxy#receiveClientMsg : receive client conn create done , cliID is : %v , cID is : %v , sID is : %v !", cliID, cID, sID)
				p.clientConnCreateDoneHandler(clientConn, cliID, cID, sID)
			case ipp.MSG_TYPE_CONN_CLOSE:
				log.Printf("Proxy#receiveClientMsg : receive client conn close , cliID is : %v , cID is : %v , sID is : %v !", cliID, cID, sID)
				p.clientConnCloseHandler(cliID, cID, sID)
			case ipp.MSG_TYPE_REQ:
				log.Printf("Proxy#receiveClientMsg : receive client req , cliID is : %v , cID is : %v , sID is : %v !", cliID, cID, sID)
				// receive client req , we should judge the client port , and dispatch the data to all browser who connect to this port.
				p.clientReqHandler(clientConn, m)
			case ipp.MSG_TYPE_CONN_HB:
				log.Printf("Proxy#receiveClientMsg : receive client heart beat , cliID is : %v , cID is : %v , sID is : %v !", cliID, cID, sID)
				p.clientHBHandler(clientConn, m)
			}
		}
	}

}
func (p *Proxy) sendBrowserConnCreateEvent(clientConn, browserConn net.Conn, clientWannaProxyPort string, cliID, cID, sID uint16) (success bool) {
	m := ippnew.NewMessage(p.IPPVersion, ippnew.SetV2Secret(p.V2Secret))
	m.ForConnCreate([]byte(clientWannaProxyPort), cliID, cID, sID)
	//marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err := clientConn.Write(b)
	if err != nil {
		log.Printf("Proxy#sendBrowserConnCreateEvent : notify client conn create err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", cliID, cID, sID, err.Error())
		err = browserConn.Close()
		if err != nil {
			log.Printf("Proxy#sendBrowserConnCreateEvent : after notify client conn create , close conn err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", cliID, cID, sID, err.Error())
		}
		return false
	}
	return true
}
func (p *Proxy) sendBrowserConnCloseEvent(clientConn net.Conn, cliID, cID, sID uint16) {
	m := ippnew.NewMessage(p.IPPVersion, ippnew.SetV2Secret(p.V2Secret))
	m.ForConnClose([]byte{}, cliID, cID, sID)
	//marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err := clientConn.Write(b)
	if err != nil {
		log.Printf("Proxy#sendBrowserConnCloseEvent : notify client conn close err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", cliID, cID, sID, err.Error())
		return
	}
	return
}

// clientHelloHandler
//  处理client发送过来的hello
func (p *Proxy) clientHelloHandler(clientConn *ipnet.IPConn, clientWannaProxyPort string, cliID, sID uint16) {
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
func (p *Proxy) listenBrowser(clientConn *ipnet.IPConn, clientWannaProxyPort string, cliID, sID uint16) (err error) {

	// listen clientWannaProxyPort. data from browser , to client
	lis, err := net.Listen("tcp", ":"+clientWannaProxyPort)
	if err != nil {
		log.Printf("Proxy#listenBrowser : listen clientWannaProxyPort err , err is : %v !", err)
		return err
	}
	bl := ipnet.NewIPListener(lis)
	bl.AddCloseTrigger(func(listener net.Listener) {
		log.Printf("Proxy#listenBrowser : browser listener close by self !")
	}, &ipnet.ListenerCloseTrigger{
		Signal: p.stopSignal,
		Handler: func(listener net.Listener) {
			log.Printf("Proxy#listenBrowser : browser listener close by stopSignal !")
			bl.Close()
		},
	}, &ipnet.ListenerCloseTrigger{
		Signal: clientConn.CloseSignal(),
		Handler: func(listener net.Listener) {
			log.Printf("Proxy#listenBrowser : browser listener close by client conn closeSignal !")
			bl.Close()
		},
	}, &ipnet.ListenerCloseTrigger{
		Signal: p.destroySignal,
		Handler: func(listener net.Listener) {
			log.Printf("Proxy#listenBrowser : browser listener close by destroySignal !")
			bl.Close()
		},
	})
	log.Printf("Proxy#listenBrowser : listen browser port is : %v !", clientWannaProxyPort)
	go func() {
		for {
			select {
			case <-bl.CloseSignal():
				log.Println("Proxy#listenBrowser : get browser listener close signal , we will stop accept browser conn !")
				return
			default:
				// when listener stop , we stop accept
				c, err := bl.Accept()
				if err != nil {
					log.Printf("Proxy#listenBrowser : accept browser conn err , err is : %v !", err.Error())
					break
				}
				// cID sID
				cID := p.newSerialNo()
				sID := p.newSerialNo()
				// trans to ip net
				bc := ipnet.NewIPConn(c)
				bc.AddCloseTrigger(func(conn net.Conn) {
					log.Println("Proxy#listenBrowser : browser conn close by self !")
					p.browserConnRID.Delete(cID)
					p.sendBrowserConnCloseEvent(clientConn, cliID, cID, sID)
					return
				}, &ipnet.ConnCloseTrigger{
					Signal: p.stopSignal,
					Handler: func(conn net.Conn) {
						log.Println("Proxy#listenBrowser : browser conn close by stopSignal !")
						p.browserConnRID.Delete(cID)
						p.sendBrowserConnCloseEvent(clientConn, cliID, cID, sID)
						return
					},
				}, &ipnet.ConnCloseTrigger{
					Signal: p.destroySignal,
					Handler: func(conn net.Conn) {
						log.Println("Proxy#listenBrowser : browser conn close by destroySignal !")
						p.browserConnRID.Delete(cID)
						p.sendBrowserConnCloseEvent(clientConn, cliID, cID, sID)
						return
					},
				}, &ipnet.ConnCloseTrigger{
					Signal: bl.CloseSignal(),
					Handler: func(conn net.Conn) {
						log.Println("Proxy#listenBrowser : browser conn close by browser listener closeSignal !")
						p.browserConnRID.Delete(cID)
						p.sendBrowserConnCloseEvent(clientConn, cliID, cID, sID)
						return
					},
				}, &ipnet.ConnCloseTrigger{
					Signal: clientConn.CloseSignal(),
					Handler: func(conn net.Conn) {
						log.Println("Proxy#listenBrowser : browser conn close by client conn closeSignal !")
						p.browserConnRID.Delete(cID)
						p.sendBrowserConnCloseEvent(clientConn, cliID, cID, sID)
						return
					},
				})
				// rem browser conn and notify client
				p.browserConnRID.Store(cID, bc)
				p.sendBrowserConnCreateEvent(clientConn, bc, clientWannaProxyPort, cliID, cID, sID)
				log.Printf("Proxy#listenBrowser : accept a browser conn success , cliID is : %v , cID is : %v , sID is : %v , clientWannaProxyPort is : %v , browser addr is : %v !", cliID, cID, sID, clientWannaProxyPort, bc.RemoteAddr())
			}
		}
	}()
	return nil
}

// clientConnCreateDoneHandler
//  开始监听用户发送的消息
func (p *Proxy) clientConnCreateDoneHandler(clientConn *ipnet.IPConn, cliID, cID, sID uint16) {
	v, ok := p.browserConnRID.Load(cID)
	if !ok {
		return
	}
	browserConn, _ := v.(*ipnet.IPConn)
	if browserConn == nil {
		return
	}
	// read browser request
	bs := make([]byte, 4096, 4096)
	go func() {
		for {
			select {
			case <-browserConn.CloseSignal():
				log.Printf("Proxy#clientConnCreateDoneHandler : get browser conn close signal , will stop read browser conn , cliID is : %v , cID is : %v , sID is : %v !", cliID, cID, sID)
				return
			default:
				log.Printf("Proxy#proxyCreateBrowserConnHandler : wait receive browser msg , cliID is : %v , cID is : %v , sID is : %v !", cliID, cID, sID)
				// build protocol to client
				sID := p.newSerialNo()
				n, err := browserConn.Read(bs)
				s := bs[0:n]
				if err != nil {
					log.Printf("Proxy#clientConnCreateDoneHandler : read browser data err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", cliID, cID, sID, err.Error())
					continue
				}
				//if n <= 0 {
				//	continue
				//}
				log.Printf("Proxy#clientConnCreateDoneHandler : accept browser req , cliID is : %v , cID is : %v , sID is : %v , len is : %v !", cliID, cID, sID, n)
				m := ippnew.NewMessage(p.IPPVersion, ippnew.SetV2Secret(p.V2Secret))
				m.ForReq(s, cliID, cID, sID)
				//marshal
				b := m.Marshall()
				ippLen := make([]byte, 4, 4)
				binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
				b = append(ippLen, b...)
				n, err = clientConn.Write(b)
				if err != nil {
					log.Printf("Proxy#clientConnCreateDoneHandler : send browser data to client err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", cliID, cID, sID, err.Error())
					continue
				}
				log.Printf("Proxy#clientConnCreateDoneHandler : from proxy to client , cliID is : %v , cID is : %v , sID is : %v , len is : %v !", cliID, cID, sID, n)
			}
		}
	}()
}

func (p *Proxy) clientConnCloseHandler(cliID, cID, sID uint16) {
	v, ok := p.browserConnRID.Load(cID)
	if !ok {
		return
	}
	browserConn, _ := v.(net.Conn)
	if browserConn == nil {
		return
	}
	p.browserConnRID.Delete(cID)
	err := browserConn.Close()
	if err != nil {
		log.Printf("Proxy#clientConnCloseHandler : after receive client conn close , close browser conn err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", cliID, cID, sID, err.Error())
	}
}

func (p *Proxy) clientReqHandler(clientConn net.Conn, m ipp.Message) {
	cID := m.CID()
	sID := m.SerialId()
	cliID := m.CliID()
	v, ok := p.browserConnRID.Load(cID)
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
		log.Printf("Proxy#clientReqHandler : from client to browser err , cliID is : %v , cID is : %v , sID is : %v , err is : %v !", cliID, cID, sID, err.Error())
		return
	}
	log.Printf("Proxy#clientReqHandler : from client to browser success , cliID is : %v , cID is : %v , sID is : %v , data len is : %v !", cliID, cID, sID, n)
}

// sayHello
//  如果port为空，那么代表监听browser失败，port被占用？
func (p *Proxy) sayHello(clientConn net.Conn, port string, sID uint16) {
	// return client hello
	cliID := p.newCID()
	errCode := byte(0)
	if port == "" {
		errCode = ipp.ERROR_CODE_BROWSER_PORT_OCUP
	}
	m := ippnew.NewMessage(p.IPPVersion, ippnew.SetV2Secret(p.V2Secret))
	m.ForServerHelloReq([]byte(strconv.FormatInt(int64(cliID), 10)), []byte(port), sID, errCode)
	//marshal
	b := m.Marshall()
	ippLen := make([]byte, 4, 4)
	binary.BigEndian.PutUint32(ippLen, uint32(len(b)))
	b = append(ippLen, b...)
	_, err := clientConn.Write(b)
	if err != nil {
		log.Printf("Proxy#sayHello : return client hello err , cliID is : %v , err is : %v !", cliID, err.Error())
		return
	}
	log.Printf("Proxy#sayHello : say hello to client success , cliID is : %v , sID is : %v !", cliID, sID)
}

//产生随机序列号
func (p *Proxy) newSerialNo() uint16 {
	atomic.CompareAndSwapInt32(&p.seq, math.MaxUint16, 0)
	atomic.AddInt32(&p.seq, 1)
	return uint16(p.seq)
}

//产生随机序列号
func (p *Proxy) newCID() uint16 {
	rand.Seed(time.Now().UnixNano())
	r := rand.Intn(math.MaxUint16)
	return uint16(r)
}

// clientHBHandler
func (p *Proxy) clientHBHandler(conn *ipnet.IPConn, message ipp.Message) {
	conn.ResetHeartBeatTimer()
}

// checkClientHB
func (p *Proxy) checkClientHB(clientConn *ipnet.IPConn) {
	<-clientConn.GetHeartBeatTimer().C
	clientConn.Close()
}
