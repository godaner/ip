package ipp
type Attr interface {
	T() byte
	L() byte
	V() []byte
}