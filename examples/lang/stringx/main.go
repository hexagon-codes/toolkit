package main

import (
	"fmt"

	"github.com/hexagon-codes/toolkit/lang/stringx"
)

func main() {
	data := []byte("Hello, 世界")
	text := stringx.BytesToString(data)
	copyOfText := stringx.StringToBytes(text)
	items := stringx.StringToSlice([]int{1, 2, 3})

	data[0] = 'h'
	copyOfText[0] = 'Y'
	fmt.Println(text)
	fmt.Println(string(copyOfText))
	fmt.Println(items)
}
