package main

import (
	"strings"
	"testing"
)

func BenchmarkStringReverseBad(b *testing.B) {
	b.ReportAllocs()

	input := "A pessimist sees the difficulty in every opportunity; an optimist sees the opportunity in every difficulty."

	for i := 0; i < b.N; i++ {
		words := strings.Split(input, " ")
		wordsReverse := make([]string, 0)
		for {
			word := words[len(words)-1:][0]
			wordsReverse = append(wordsReverse, word)
			words = words[:len(words)-1]
			if len(words) == 0 {
				break
			}
		}
		output := strings.Join(wordsReverse, " ")
		if output != "difficulty. every in opportunity the sees optimist an opportunity; every in difficulty the sees pessimist A" {
			b.Error("Unexpected result: " + output)
		}
	}
}

func BenchmarkStringReverseBetter(b *testing.B) {
	b.ReportAllocs()

	input := "A pessimist sees the difficulty in every opportunity; an optimist sees the opportunity in every difficulty."

	for i := 0; i < b.N; i++ {
		words := strings.Split(input, " ")
		for i := 0; i < len(words)/2; i++ {
			words[len(words)-1-i], words[i] = words[i], words[len(words)-1-i]
		}
		output := strings.Join(words, " ")
		if output != "difficulty. every in opportunity the sees optimist an opportunity; every in difficulty the sees pessimist A" {
			b.Error("Unexpected result: " + output)
		}
	}
}
