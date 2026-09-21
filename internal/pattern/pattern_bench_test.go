// Part of go-uptime, derived from Gatus by TwiN (Apache-2.0); files that existed in Gatus were modified. See NOTICE.

package pattern

import "testing"

func BenchmarkMatch(b *testing.B) {
	for n := 0; n < b.N; n++ {
		if !Match("*ing*", "livingroom") {
			b.Error("should've matched")
		}
	}
	b.ReportAllocs()
}

func BenchmarkMatchWithBackslash(b *testing.B) {
	for n := 0; n < b.N; n++ {
		if !Match("*ing*", "living\\room") {
			b.Error("should've matched")
		}
	}
	b.ReportAllocs()
}
