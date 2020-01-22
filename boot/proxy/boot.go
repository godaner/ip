package main

import (
	"github.com/godaner/ip/progress/console/proxy"
	"log"
)

func main() {
	p := new(proxy.Progress)
	err := p.Launch()
	if err != nil {
		log.Printf("mian : progress Start err , err is : %v !", err.Error())
		return
	}
	f := make(chan int, 1)
	<-f
}
