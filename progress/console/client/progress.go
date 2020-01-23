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
	clis := []*client.Client{}
	cliID := uint16(0)
	c.SetUpdateEventHandler(func(c *Config) {
		// stop first
		for _, c := range clis {
			cli := c
			go cli.Destroy()
		}
		clis = []*client.Client{}
		// create new client
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

			clis = append(clis, cli)
			go func() {
				err := cli.Restart()
				if err != nil {
					log.Printf("Progress#Launch : restart client err , client id is : %v , err is : %v !", cliID, err.Error())
				}
			}()
		})
	})
	return nil
}
