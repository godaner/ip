package progress

import (
	"fmt"
	"github.com/godaner/ip/client/config"
	"github.com/godaner/ip/ipp"
	"github.com/godaner/ip/ipp/ippnew"
	"log"
	"math"
	"math/rand"
	"net"
	"time"
)

type Progress struct {
	ClientForwardConn net.Conn
	ProxyConn         net.Conn
	CID               uint16
	Config            *config.Config
}

func (p *Progress) Listen() (err error) {
	c := new(config.Config)
	err = c.Load()
	if err != nil {
		return err
	}
	p.CID = p.newSerialNo()
	p.Config = c
	// proxy conn
	go func() {
		addr := c.ProxyAddr
		conn, err := net.Dial("tcp", addr)
		if err != nil {
			panic(err)
		}
		log.Printf("Progress#Listen : dial proxy addr is : %v !", addr)

		p.ProxyConn = conn
		// listen proxy return msg
		go p.fromProxyHandler()
		// say hello to proxy
		m := ippnew.NewMessage(p.Config.IPPVersion)
		m.ForHelloReq([]byte(c.ClientWannaProxyPort), p.CID)
		_, err = p.ProxyConn.Write(m.Marshall())
		if err != nil {
			log.Printf("Progress#Listen : say hello to proxy err , err : %v !", err)
		}
	}()
	return nil
}

// fromProxyHandler
//  监听proxy返回的消息
func (p *Progress) fromProxyHandler() {
	for {
		// parse protocol
		bs := make([]byte, 1024, 1024)
		n, _ := p.ProxyConn.Read(bs)
		m := ippnew.NewMessage(p.Config.IPPVersion)
		m.UnMarshall(bs[0:n])
		switch m.Type() {
		case ipp.MSG_TYPE_HELLO:
			log.Println("Progress#fromClientConnHandler : receive proxy hello !")
			// proxy return hello , we should dial forward addr
			addr := p.Config.ClientForwardAddr
			conn, err := net.Dial("tcp", addr)
			if err != nil {
				panic(err)
			}
			p.ClientForwardConn = conn
			log.Printf("Progress#fromProxyHandler : receive proxy hello , dial forward addr is : %v !", addr)
			// listen forward's conn
			go p.fromForwardConnHandler()
		case ipp.MSG_TYPE_REQ:
			log.Println("Progress#fromProxyHandler : receive proxy req !")
			// receive proxy req info , we should dispatch the info
			b:=m.AttributeByType(ipp.ATTR_TYPE_BODY)
			fmt.Println(string(b))
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
	for {
		log.Println("Progress#fromForwardConnHandler : wait receive forward msg !")
		bs := make([]byte, 1024, 1024)
		n, _ := p.ClientForwardConn.Read(bs)
		log.Println("Progress#fromForwardConnHandler : receive forward msg !")
		m := ippnew.NewMessage(p.Config.IPPVersion)
		m.ForReq(bs[0:n], p.CID)
		_, err := p.ProxyConn.Write(m.Marshall())
		if err != nil {
			log.Printf("Progress#fromForwardConnHandler : write forward's data to proxy is : %v !", err.Error())
		}
	}
}

//产生随机序列号
func (p *Progress) newSerialNo() uint16 {
	rand.Seed(time.Now().UnixNano())
	r := rand.Intn(math.MaxUint16)
	return uint16(r)
}
