package main

import "testing"

func benchmarkDefer2(b *testing.B, multiplier int) {
	var d Defer
	b.ReportAllocs()
	for i := 0; i < b.N*multiplier; i++ {
		d.defer2()
	}
}

func BenchmarkDefer2_1(b *testing.B)  { benchmarkDefer2(b, 1) }
func BenchmarkDefer2_2(b *testing.B)  { benchmarkDefer2(b, 2) }
func BenchmarkDefer2_5(b *testing.B)  { benchmarkDefer2(b, 5) }
func BenchmarkDefer2_10(b *testing.B) { benchmarkDefer2(b, 10) }

func benchmarkDefer1(b *testing.B, multiplier int) {
	var d Defer
	b.ReportAllocs()
	for i := 0; i < b.N*multiplier; i++ {
		d.defer1()
	}
}

func BenchmarkDefer1_1(b *testing.B)  { benchmarkDefer1(b, 1) }
func BenchmarkDefer1_2(b *testing.B)  { benchmarkDefer1(b, 2) }
func BenchmarkDefer1_5(b *testing.B)  { benchmarkDefer1(b, 5) }
func BenchmarkDefer1_10(b *testing.B) { benchmarkDefer1(b, 10) }
