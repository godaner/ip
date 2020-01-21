package progress

import (
	"github.com/godaner/ip/client/config"
	"log"
)

// Progress
type Progress struct {
}

func (p *Progress) Listen() (err error) {
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
		go client.Start()
		cliID++
	})

	return nil
}
