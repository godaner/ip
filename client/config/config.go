package config

import (
	"errors"
	"flag"
	"fmt"
	conf "github.com/Unknwon/goconfig"
	"log"
	"os"
	"strconv"
)

var (
	h      bool
	config string
)

type Config struct {
	ProxyAddr string
	ClientProxyMapping string
	IPPVersion         int
	V2Secret           string
}

func (c *Config) Load() (err error) {
	flag.BoolVar(&h, "h", false, "this help")
	flag.StringVar(&config, "c", "./ipclient.ini", "set configuration `file`")
	flag.Usage = usage
	flag.Parse()
	if h {
		flag.Usage()
		return errors.New("help you")
	}
	confo, err := conf.LoadConfigFile(config)
	if err != nil {
		log.Println("Config#Get : load config file error :", err)
		return err
	}

	globalConfig, err := confo.GetSection("global")
	if err != nil {
		log.Println("Config#Get : get global error :", err)
		return err
	}
	c.ProxyAddr = globalConfig["proxy_addr"]
	c.V2Secret = globalConfig["v2_secret"]
	//c.ClientForwardAddr = globalConfig["client_forward_addr"]
	//c.ClientWannaProxyPort = globalConfig["client_wanna_proxy_port"]
	c.ClientProxyMapping = globalConfig["client_proxy_mapping"]
	iv, _ := strconv.ParseInt(globalConfig["ipp_version"], 10, 64)
	c.IPPVersion = int(iv)
	return nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: ipclient [-h] [-c filename]

Options:
`)
	flag.PrintDefaults()
}
