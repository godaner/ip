package ippnew

type Options struct {
	V2Secret string
}

type Option func(o *Options)

func SetV2Secret(v2Secret string) Option {
	return func(o *Options) {
		o.V2Secret = v2Secret
	}
}
