package ippnew

import (
	"github.com/godaner/ip/ipp"
	v1 "github.com/godaner/ip/ipp/v1"
	v2 "github.com/godaner/ip/ipp/v2"
)

func NewMessage(v int, opts ...Option) (m ipp.Message) {
	options := Options{}
	for _, o := range opts {
		o(&options)
	}
	if v == ipp.VERSION_V1 {
		return new(v1.Message)
	}
	if v == ipp.VERSION_V2 {
		m := new(v2.Message)
		m.Salt = options.V2Secret
		return m
	}
	return nil
}
