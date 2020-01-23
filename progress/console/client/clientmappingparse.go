package client

import (
	"log"
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
	addrs               []string
}

// init
func (c *ClientProxyMappingParser) init() {
	c.Do(func() {
		coms := []*ClientProxyMapping{}
		ports := []string{}
		addrs := []string{}
		mappings := strings.Split(c.ClientProxyMappingSource, mapping_split)
		for _, m := range mappings {
			p := strings.Split(m, mapping_port_addr_split)
			if len(p) != 2 {
				log.Printf("ClientProxyMappingParser#init : ClientProxyMappingSource is not right , ClientProxyMappingSource is : %v !", c.ClientProxyMappingSource)
				return
			}
			port := p[0]
			addr := p[1]

			coms = append(coms, &ClientProxyMapping{
				clientWannaProxyPort: port,
				clientForwardAddr:    addr,
			})
			ports = append(ports, port)
			addrs = append(addrs, addr)
		}
		c.clientProxyMappings = coms
		c.ports = ports
		c.addrs = addrs

	})
}

type rangeFunc func(clientWannaProxyPort, clientForwardAddr string)

// Range
func (c *ClientProxyMappingParser) Range(rf rangeFunc) {
	c.init()
	for _, c := range c.clientProxyMappings {
		rf(c.clientWannaProxyPort, c.clientForwardAddr)
	}
}

// GetClientProxyMappingPorts
//  from 12345:192.168.6.207:443,1234:192.168.6.207:22
//  to 12345,1234
func (c *ClientProxyMappingParser) GetClientProxyMappingPorts() (ports []string) {
	c.init()
	return c.ports
}

// GetClientProxyMappingAddrs
func (c *ClientProxyMappingParser) GetClientProxyMappingAddrs() (addrs []string) {
	c.init()
	return c.addrs
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
// Diff
func (c *ClientProxyMappingParser) Diff(target *ClientProxyMappingParser) (diff bool) {
	c.init()
	if target==nil{
		return true
	}
	targetPorts := target.GetClientProxyMappingPorts()
	sourcePorts := c.GetClientProxyMappingPorts()
	if len(targetPorts) != len(sourcePorts) {
		return true
	}
	targetAddrs := target.GetClientProxyMappingAddrs()
	sourceAdds := c.GetClientProxyMappingAddrs()
	if len(targetAddrs) != len(sourceAdds) {
		return true
	}
	for _, c := range c.clientProxyMappings {
		sourcePort := c.clientWannaProxyPort
		sourceAddr := c.clientForwardAddr
		if !instrslic(sourcePort, targetPorts) {
			return true
		}
		if !instrslic(sourceAddr, targetAddrs) {
			return true
		}
	}
	return false
}

//字符串数组包含
func instrslic(target string, source []string) bool {
	for _, s := range source {
		if s == target {
			return true
		}
	}
	return false
}
