package util

import (
	"crypto/rand"
	"fmt"
	"math/big"
)

func RandAlpaNumericString(l int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	lettersLen := big.NewInt(int64(len(letters)))
	ret := make([]byte, l)
	for i := 0; i < l; i++ {
		num, err := rand.Int(rand.Reader, lettersLen)
		if err != nil {
			panic(fmt.Sprintf("Failed to random word %+v", err))
		}
		ret[i] = letters[num.Int64()]
	}

	return string(ret)
}
