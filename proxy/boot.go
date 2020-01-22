package main

import (
	"github.com/godaner/ip/proxy/progress"
	"log"
)

func main() {
	p := new(progress.Progress)
	err := p.Start()
	if err != nil {
		log.Printf("mian : progress Start err , err is : %v !", err.Error())
		return
	}
	f := make(chan int, 1)
	<-f
}
