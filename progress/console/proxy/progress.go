package proxy

import (
	"github.com/godaner/ip/endpoint/proxy"
	"log"
)

type Progress struct {
}

func (p *Progress) Launch() (err error) {
	// log
	log.SetFlags(log.Lmicroseconds)
	// config
	c := new(Config)
	err = c.Load()
	if err != nil {
		return err
	}

	pry := &proxy.Proxy{
		LocalPort:  c.LocalPort,
		IPPVersion: c.IPPVersion,
		V2Secret:   c.V2Secret,
	}
	go func() {
		err := pry.Start()
		if err != nil {
			log.Printf("Progress#Start : start client err , err is : %v !", err.Error())
		}
	}()

	return nil
}
