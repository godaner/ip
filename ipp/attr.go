package ipp
type Attr interface {
	T() byte
	L() uint32
	V() []byte
}