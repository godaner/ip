package proxy

import (
	"errors"
	"flag"
	"fmt"
	conf "github.com/Unknwon/goconfig"
	"log"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	check_config_time = 5
)

var (
	h      bool
	config string
)

type Config struct {
	LocalPort    string
	IPPVersion   int
	V2Secret     string
	updateSignal chan bool
	once         sync.Once
}

func (c *Config) ISUpdate() (s chan bool) {
	if c.updateSignal == nil {
		c.updateSignal = make(chan bool)
		go func() {
			for {
				time.Sleep(time.Second * check_config_time)
				err := c.loadConfFile()
				if err != nil {
					log.Println("Config#ISUpdate : load config file error :", err)
					continue
				}
				close(c.updateSignal)
				c.updateSignal = make(chan bool)
			}
		}()
	}
	return c.updateSignal
}
func (c *Config) loadConfFile() (err error) {
	confo, err := conf.LoadConfigFile(config)
	if err != nil {
		log.Println("Config#loadConfFile : load config file error :", err)
		return err
	}

	globalConfig, err := confo.GetSection("global")
	if err != nil {
		log.Println("Config#loadConfFile : get global error :", err)
		return err
	}
	c.LocalPort = globalConfig["loc_port"]
	c.V2Secret = globalConfig["v2_secret"]
	iv, _ := strconv.ParseInt(globalConfig["ipp_version"], 10, 64)
	c.IPPVersion = int(iv)
	return nil
}
func (c *Config) Load() (err error) {
	flag.BoolVar(&h, "h", false, "this help")
	flag.StringVar(&config, "c", "./ipproxy.ini", "set configuration `file`")
	flag.Usage = usage
	flag.Parse()
	if h {
		flag.Usage()
		return errors.New("help you")
	}
	err = c.loadConfFile()
	if err != nil {
		return err
	}
	return nil
}

func usage() {
	fmt.Fprintf(os.Stderr, `Usage: ipproxy [-h] [-c filename]

Options:
`)
	flag.PrintDefaults()
}
