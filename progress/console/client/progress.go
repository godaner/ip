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
	cliID := uint16(0)
	c.ClientProxyMappingParser.Range(func(clientWannaProxyPort, clientForwardAddr string) {
		cli := &client.Client{
			ProxyAddr:            c.ProxyAddr,
			IPPVersion:           c.IPPVersion,
			ClientForwardAddr:    clientForwardAddr,
			ClientWannaProxyPort: clientWannaProxyPort,
			TempCliID:            cliID,
			V2Secret:             c.V2Secret,
		}
		cliID++
		go func() {
			err := cli.Restart()
			if err != nil {
				log.Printf("Progress#Launch : restart client err , client id is : %v , err is : %v !", cliID, err.Error())
			}
		}()
	})
	return nil
}
