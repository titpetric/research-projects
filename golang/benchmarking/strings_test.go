package main

import "testing"
import "strings"

func BenchmarkStringJoin1(b *testing.B) {
	b.ReportAllocs()
	input := []string{"Hello", "World"}
	for i := 0; i < b.N; i++ {
		result := strings.Join(input, " ")
		if result != "Hello World" {
			b.Error("Unexpected result: " + result)
		}
	}
}

func BenchmarkStringJoin1B(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		input := []string{"Hello", "World"}
		result := strings.Join(input, " ")
		if result != "Hello World" {
			b.Error("Unexpected result: " + result)
		}
	}
}

func BenchmarkStringJoin2(b *testing.B) {
	b.ReportAllocs()
	input := []string{"Hello", "World"}
	join := func(strs []string, delim string) string {
		if len(strs) == 2 {
			return strs[0] + delim + strs[1];
		}
		return "";
	};
	for i := 0; i < b.N; i++ {
		result := join(input, " ")
		if result != "Hello World" {
			b.Error("Unexpected result: " + result)
		}
	}
}

func BenchmarkStringJoin2B(b *testing.B) {
	b.ReportAllocs()
	join := func(strs []string, delim string) string {
		if len(strs) == 2 {
			return strs[0] + delim + strs[1];
		}
		return "";
	};
	for i := 0; i < b.N; i++ {
		input := []string{"Hello", "World"}
		result := join(input, " ")
		if result != "Hello World" {
			b.Error("Unexpected result: " + result)
		}
	}
}

func BenchmarkStringJoin3(b *testing.B) {
	b.ReportAllocs()
	input := []string{"Hello", "World"}
	for i := 0; i < b.N; i++ {
		result := input[0] + " " + input[1];
		if result != "Hello World" {
			b.Error("Unexpected result: " + result)
		}
	}
}

func BenchmarkStringJoin3B(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		input := []string{"Hello", "World"}
		result := input[0] + " " + input[1];
		if result != "Hello World" {
			b.Error("Unexpected result: " + result)
		}
	}
}
