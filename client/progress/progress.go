package progress

import (
	"github.com/godaner/ip/client/config"
	"github.com/godaner/ip/ipp"
	"github.com/godaner/ip/ipp/ippnew"
	"log"
	"math"
	"math/rand"
	"net"
	"time"
)

const (
	restart_interval = 5
)

type Progress struct {
	ClientForwardConn net.Conn
	ProxyConn         net.Conn
	CID               uint16
	Config            *config.Config
	RestartSignal     chan int
}

func (p *Progress) Listen() (err error) {
	c := new(config.Config)
	err = c.Load()
	if err != nil {
		return err
	}
	p.CID = p.newSerialNo()
	p.Config = c
	p.RestartSignal = make(chan int)
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
	m.ForHelloReq([]byte(p.Config.ClientWannaProxyPort), p.CID)
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
		bs := make([]byte, 1024, 1024)
		n, err := p.ProxyConn.Read(bs)
		s:=bs[0:n]
		log.Printf("Progress#fromProxyHandler : receive proxy msg , msg is : %v , len is : %v !",string(s),n)
		if err != nil {
			log.Printf("Progress#fromClientConnHandler : receive proxy err , err is : %v !", err)
			p.setRestartSignal()
			return
		}
		if n<=0{
			continue
		}
		m := ippnew.NewMessage(p.Config.IPPVersion)
		m.UnMarshall(s)
		switch m.Type() {
		case ipp.MSG_TYPE_HELLO:
			log.Println("Progress#fromClientConnHandler : receive proxy hello !")
			// listen forward's conn
			go p.fromForwardConnHandler()
		case ipp.MSG_TYPE_REQ:
			log.Println("Progress#fromProxyHandler : receive proxy req !")
			// receive proxy req info , we should dispatch the info
			b := m.AttributeByType(ipp.ATTR_TYPE_BODY)
			log.Printf("Progress#fromProxyHandler : receive proxy req , body is : %v", string(b))
			if p.ClientForwardConn == nil {
				log.Println("Progress#fromClientConnHandler : ClientForwardConn is nil !")
				p.setRestartSignal()
				return
			}
			n, err := p.ClientForwardConn.Write(b)
			if err != nil {
				log.Printf("Progress#fromProxyHandler : receive proxy req , forward err , err : %v !", err)
			}
			log.Printf("Progress#fromProxyHandler : from proxy to forward , msg len is : %v !", n)
		default:
			log.Println("Progress#fromProxyHandler : receive proxy msg , but can't find type !")
		}
	}

}

// fromForwardConnHandler
//  监听forward返回的消息
func (p *Progress) fromForwardConnHandler() {
	// proxy return hello , we should dial forward addr
	addr := p.Config.ClientForwardAddr
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		log.Printf("Progress#fromForwardConnHandler : after get proxy hello , dial forward err , err is : %v !", err)
		p.setRestartSignal()
		return
	}
	p.ClientForwardConn = conn
	log.Printf("Progress#fromForwardConnHandler : dial forward addr success , forward address is : %v !", addr)
	for {
		log.Println("Progress#fromForwardConnHandler : wait receive forward msg !")
		bs := make([]byte, 1024, 1024)
		n, err := p.ClientForwardConn.Read(bs)
		if err != nil {
			log.Printf("Progress#fromForwardConnHandler : read forward data err , err is : %v !", err)
			p.setRestartSignal()
			return
		}
		log.Printf("Progress#fromForwardConnHandler : receive forward msg , msg is : %v , len is : %v !", string(bs[0:n]), n)
		m := ippnew.NewMessage(p.Config.IPPVersion)
		m.ForReq(bs[0:n], p.CID)
		_, err = p.ProxyConn.Write(m.Marshall())
		if err != nil {
			log.Printf("Progress#fromForwardConnHandler : write forward's data to proxy is : %v !", err.Error())
			p.setRestartSignal()
			return
		}
	}
}

//产生随机序列号
func (p *Progress) newSerialNo() uint16 {
	rand.Seed(time.Now().UnixNano())
	r := rand.Intn(math.MaxUint16)
	return uint16(r)
}
