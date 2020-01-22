package progress

import (
	"github.com/godaner/ip/proxy/config"
	"log"
)

type Progress struct {
	stopSignal chan bool
}

func (p *Progress) Stop() (err error) {
	close(p.stopSignal)
	return nil
}

func (p *Progress) Start() (err error) {
	// log
	log.SetFlags(log.Lmicroseconds)
	// config
	c := new(config.Config)
	err = c.Load()
	if err != nil {
		return err
	}

	proxy := &Proxy{
		LocalPort:  c.LocalPort,
		IPPVersion: c.IPPVersion,
		V2Secret:   c.V2Secret,
	}
	go func() {
		err := proxy.Start()
		if err != nil {
			log.Printf("Progress#Start : start client err , err is : %v !", err.Error())
		}
		select {
		case <-p.stopSignal:
			err := proxy.Stop()
			if err != nil {
				log.Printf("Progress#Start : stop client err , err is : %v !", err.Error())
			}
		}
	}()

	return nil
}
