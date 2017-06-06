package main

import "testing"

func benchmarkR2(b *testing.B, multiplier int) {
	b.ReportAllocs()
	var count int64
	for i := 0; i < b.N*multiplier; i++ {
		var loop int32
		loop = 1000
		recursive2(&count, loop)
	}
}

//*
func BenchmarkR2_1(b *testing.B) { benchmarkR2(b, 1) }
func BenchmarkR2_2(b *testing.B) { benchmarkR2(b, 2) }
func BenchmarkR2_5(b *testing.B) { benchmarkR2(b, 5) }
func BenchmarkR2_10(b *testing.B) { benchmarkR2(b, 10) }
//*/

func benchmarkR1(b *testing.B, multiplier int) {
	b.ReportAllocs()
	var count int64
	for i := 0; i < b.N*multiplier; i++ {
		count += recursive1(1000)
	}
}

//*
func BenchmarkR1_1(b *testing.B) { benchmarkR1(b, 1) }
func BenchmarkR1_2(b *testing.B) { benchmarkR1(b, 2) }
func BenchmarkR1_5(b *testing.B) { benchmarkR1(b, 5) }
func BenchmarkR1_10(b *testing.B) { benchmarkR1(b, 10) }
//*/
