package progress

import (
	"github.com/godaner/ip/client/config"
	"log"
)

// Progress
type Progress struct {
	stopSignal chan bool
}

func (p *Progress) Stop() (err error) {
	close(p.stopSignal)
	return nil
}

func (p *Progress) Start() (err error) {
	p.stopSignal = make(chan bool)
	// log
	log.SetFlags(log.Lmicroseconds)

	c := new(config.Config)
	err = c.Load()
	if err != nil {
		return err
	}
	// parse client proxy mapping parser
	clientProxyMappingParser := ClientProxyMappingParser{
		ClientProxyMappingSource: c.ClientProxyMapping,
	}
	cliID := uint16(0)
	clientProxyMappingParser.Range(func(clientWannaProxyPort, clientForwardAddr string) {
		client := &Client{
			ProxyAddr:            c.ProxyAddr,
			IPPVersion:           c.IPPVersion,
			ClientForwardAddr:    clientForwardAddr,
			ClientWannaProxyPort: clientWannaProxyPort,
			V2Secret:             c.V2Secret,
			TempCliID:            cliID,
		}
		go func() {
			err := client.Start()
			if err != nil {
				log.Printf("Progress#Start : start client err , client id is : %v , err is : %v !", cliID, err.Error())
			}
			select {
			case <-p.stopSignal:
				err := client.Stop()
				if err != nil {
					log.Printf("Progress#Start : stop client err , client id is : %v , err is : %v !", cliID, err.Error())
				}
			}
		}()
		cliID++
	})

	return nil
}
