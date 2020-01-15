package ippnew

import (
	"github.com/godaner/ip/ipp"
	v1 "github.com/godaner/ip/ipp/v1"
)

func NewMessage(v int) (m ipp.Message) {
	if v == ipp.VERSION_V1 {
		return new(v1.Message)
	}
	return nil
}
