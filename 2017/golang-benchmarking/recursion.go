package main

func recursive1(count int64) int64 {
	if count <= 0 {
		return 0
	}
	return recursive1(count-1) + 1
}

func recursive2(result *int64, count int32) {
	if count <= 0 {
		return
	}
	*result++
	recursive2(result, count-1)
}
