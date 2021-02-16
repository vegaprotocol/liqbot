package proto_test

import (
	"strings"
	"testing"

	restproto "code.vegaprotocol.io/liqbot/proto"

	"github.com/stretchr/testify/assert"
)

func TestConvertPriceLevel(t *testing.T) {
	const (
		p uint64 = 1 << 63
		n uint64 = 1<<63 + 1
		v uint64 = 1<<63 + 2

		pStr = "9223372036854775808" // 1<<63
		nStr = "9223372036854775809" // 1<<63 + 1
		vStr = "9223372036854775810" // 1<<63 + 2

		tooLarge = "18446744073709551616" // 1<<64
	)
	rpl := restproto.RESTPriceLevel{
		Price:          pStr,
		NumberOfOrders: nStr,
		Volume:         vStr,
	}

	pl, err := restproto.ConvertPriceLevel(nil)
	assert.NoError(t, err)
	assert.Nil(t, pl)

	pl, err = restproto.ConvertPriceLevel(&rpl)
	assert.NoError(t, err)
	assert.Equal(t, p, pl.Price)
	assert.Equal(t, n, pl.NumberOfOrders)
	assert.Equal(t, v, pl.Volume)

	_, err = restproto.ConvertPriceLevel(&restproto.RESTPriceLevel{
		Price: tooLarge,
	})
	assert.True(t, strings.Contains(err.Error(), "value out of range"))

	_, err = restproto.ConvertPriceLevel(&restproto.RESTPriceLevel{
		Price:          "0",
		NumberOfOrders: tooLarge,
	})
	assert.True(t, strings.Contains(err.Error(), "value out of range"))

	_, err = restproto.ConvertPriceLevel(&restproto.RESTPriceLevel{
		Price:          "0",
		NumberOfOrders: "0",
		Volume:         tooLarge,
	})
	assert.True(t, strings.Contains(err.Error(), "value out of range"))
}
