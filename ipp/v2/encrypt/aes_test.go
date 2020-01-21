package encrypt

import (
	"fmt"
	"testing"
)

func TestDncrypt(t *testing.T) {
	k:="123qwe"
	txt:="I love you!"
	s,err:=Encrypt([]byte(txt),[]byte(k))
	fmt.Println(s,err)
	s1,err1:=Dncrypt(s,[]byte(k))
	fmt.Println(s1,err1)
}