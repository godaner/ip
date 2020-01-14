package v1

import (
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestVersion_NewReq(t *testing.T) {

	Convey("New Req and UnMarshall !", t, func(cc C) {
		testBody := []byte("my love")
		ri := uint16(123)
		m1 := new(Message)
		m1.NewReq(testBody, ri)
		So(m1, ShouldNotBeNil)

		m2 := new(Message)
		m2.UnMarshall(m1.Marshall())
		So(m2, ShouldNotBeNil)
		So(m2.SerialId(), ShouldBeGreaterThan, 0)
		So(m2.ReqId(), ShouldEqual, ri)
		So(len(m2.Marshall()), ShouldBeGreaterThan, 0)
		t.Logf("bytes is : %v !", m2.Marshall())

	})
}
