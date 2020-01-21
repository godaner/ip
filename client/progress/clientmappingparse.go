package progress

import (
	"strings"
	"sync"
)

const (
	mapping_split           = ","
	mapping_port_addr_split = "->"
)

// ClientProxyMappingParser
type ClientProxyMapping struct {
	clientWannaProxyPort string
	clientForwardAddr    string
}
type ClientProxyMappingParser struct {
	ClientProxyMappingSource string
	sync.Once
	clientProxyMappings []*ClientProxyMapping
	ports               []string
}

// init
func (c *ClientProxyMappingParser) init() {
	c.Do(func() {
		coms := []*ClientProxyMapping{}
		ports := []string{}
		mappings := strings.Split(c.ClientProxyMappingSource, mapping_split)
		for _, m := range mappings {
			p := strings.Split(m, mapping_port_addr_split)
			port := p[0]
			addr := p[1]

			coms = append(coms, &ClientProxyMapping{
				clientWannaProxyPort: port,
				clientForwardAddr:    addr,
			})
			ports = append(ports, port)
		}
		c.clientProxyMappings = coms
		c.ports = ports

	})
}
type rangeFunc func (clientWannaProxyPort , clientForwardAddr string)()
// Range
func (c *ClientProxyMappingParser) Range(rf rangeFunc) {
	c.init()
	for _,c:=range c.clientProxyMappings  {
		rf(c.clientWannaProxyPort,c.clientForwardAddr)
	}
}
// GetClientProxyMappingPorts
//  from 12345:192.168.6.207:443,1234:192.168.6.207:22
//  to 12345,1234
func (c *ClientProxyMappingParser) GetClientProxyMappingPorts() (ports []string) {
	c.init()
	return c.ports
}

// GetClientProxyForwardAddrByPort
//  eg. 12345:192.168.6.207:443,1234:192.168.6.207:22
func (c *ClientProxyMappingParser) GetClientProxyForwardAddrByPort(port string) (forwardAddr string) {
	c.init()
	for _, c := range c.clientProxyMappings {

		if port == c.clientWannaProxyPort {
			return c.clientForwardAddr
		}
	}
	return ""
}
