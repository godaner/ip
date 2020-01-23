package client

import (
	"flag"
	conf "github.com/Unknwon/goconfig"
	"log"
	"strconv"
	"sync"
)

type Config struct {
	config                   string
	ProxyAddr                string
	IPPVersion               int
	V2Secret                 string
	ClientProxyMappingParser *ClientProxyMappingParser
	sync.Once
}

func (c *Config) Load() (err error) {
	c.Once.Do(func() {
		flag.StringVar(&c.config, "c", "./ipclient.ini", "set configuration `file`")
		flag.Parse()
	})
	confo, err := conf.LoadConfigFile(c.config)
	if err != nil {
		log.Println("Config#Load : load config file error :", err)
		return err
	}

	globalConfig, err := confo.GetSection("global")
	if err != nil {
		log.Println("Config#Load : get global error :", err)
		return err
	}
	// proxy_addr
	c.ProxyAddr = globalConfig["proxy_addr"]

	// v2_secret
	c.V2Secret = globalConfig["v2_secret"]

	// ipp_version
	iv, _ := strconv.ParseInt(globalConfig["ipp_version"], 10, 64)
	c.IPPVersion = int(iv)

	// client_proxy_mapping
	c.ClientProxyMappingParser = &ClientProxyMappingParser{
		ClientProxyMappingSource: globalConfig["client_proxy_mapping"],
	}

	return nil
}
