package ipp

type Header interface {
	Version() byte
	Type() byte
	CID() uint16
	AttrNum() byte
}
