package client

import (
	"flag"
	conf "github.com/Unknwon/goconfig"
	"log"
	"strconv"
	"sync"
	"time"
)

const (
	check_config_time = 5
)

type Config struct {
	config                   string
	ProxyAddr                string
	IPPVersion               int
	V2Secret                 string
	ClientProxyMappingParser *ClientProxyMappingParser
	sync.Once
}
type UpdateEventHandler func(c *Config)

// SetUpdateEventHandler
func (c *Config) SetUpdateEventHandler(e UpdateEventHandler) {
	go func() {
		for {
			diff, err := c.load()
			if err != nil {
				log.Println("Config#SetUpdateEventHandler : load config file error :", err)
			} else {
				if diff {
					log.Println("Config#SetUpdateEventHandler : load new config file !")
					go e(c)
				}
			}
			time.Sleep(time.Second * check_config_time)
		}
	}()
}

func (c *Config) load() (diff bool, err error) {
	c.Once.Do(func() {
		flag.StringVar(&c.config, "c", "./ipclient.ini", "set configuration `file`")
		flag.Parse()
	})
	confo, err := conf.LoadConfigFile(c.config)
	if err != nil {
		log.Println("Config#Load : load config file error :", err)
		return diff, err
	}

	globalConfig, err := confo.GetSection("global")
	if err != nil {
		log.Println("Config#Load : get global error :", err)
		return diff, err
	}
	// proxy_addr
	nProxyAddr := globalConfig["proxy_addr"]
	if c.ProxyAddr != nProxyAddr {
		diff = true
	}
	c.ProxyAddr = nProxyAddr

	// v2_secret
	nV2Secret := globalConfig["v2_secret"]
	if c.V2Secret != nV2Secret {
		diff = true
	}
	c.V2Secret = nV2Secret

	// ipp_version
	iv, _ := strconv.ParseInt(globalConfig["ipp_version"], 10, 64)
	nIPPVersion := int(iv)
	if c.IPPVersion != nIPPVersion {
		diff = true
	}
	c.IPPVersion = nIPPVersion

	// client_proxy_mapping
	nClientProxyMappingParser := &ClientProxyMappingParser{
		ClientProxyMappingSource: globalConfig["client_proxy_mapping"],
	}
	if nClientProxyMappingParser.Diff(c.ClientProxyMappingParser) {
		diff = true
	}
	c.ClientProxyMappingParser = nClientProxyMappingParser

	return diff, nil
}
