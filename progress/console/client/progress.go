package client

import (
	"github.com/godaner/ip/endpoint/client"
	"log"
)

// Progress
type Progress struct {
}

func (p *Progress) Launch() (err error) {
	// log
	log.SetFlags(log.Lmicroseconds)

	c := new(Config)
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
		cli := &client.Client{
			ProxyAddr:            c.ProxyAddr,
			IPPVersion:           c.IPPVersion,
			ClientForwardAddr:    clientForwardAddr,
			ClientWannaProxyPort: clientWannaProxyPort,
			V2Secret:             c.V2Secret,
			TempCliID:            cliID,
		}
		go func() {
			err := cli.Start()
			if err != nil {
				log.Printf("Progress#Start : start client err , client id is : %v , err is : %v !", cliID, err.Error())
			}
		}()
		cliID++
	})

	return nil
}
