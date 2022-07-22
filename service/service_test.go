package service

import (
	"encoding/json"
	"testing"

	"github.com/golang/mock/gomock"

	"code.vegaprotocol.io/liqbot/service/mocks"
)

func TestService_getBotsTraderDetails(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBot := mocks.NewMockBot(ctrl)
	details := getDummyBotTraderDetails()
	want := `[{
  "name": "test_bot1",
  "pubKey": "somerandompublickey",
  "settlementEthereumContractAddress": "somerandomethereumaddress",
  "settlementVegaAssetID": "somerandomexternalassetid"
}]`

	mockBot.EXPECT().GetTraderDetails().Return(details)

	s := &Service{
		bots: map[string]Bot{
			"bot1": mockBot,
		},
	}

	if got := s.getBotsTraderDetails(); got != want {
		t.Errorf("getBotsTraderDetails() = %v, want %v", got, want)
	}
}

func getDummyBotTraderDetails() string {
	jsn, _ := json.MarshalIndent(map[string]string{
		"name":                              "test_bot1",
		"pubKey":                            "somerandompublickey",
		"settlementVegaAssetID":             "somerandomexternalassetid",
		"settlementEthereumContractAddress": "somerandomethereumaddress",
	}, "", "  ")
	return string(jsn)
}
